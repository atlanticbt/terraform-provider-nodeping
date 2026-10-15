package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"golang.org/x/time/rate"
)

const (
	DefaultBaseURL      = "https://api.nodeping.com/api/1"
	DefaultRateLimit    = 10
	DefaultMaxRetries   = 3
	DefaultRetryMinWait = 1 * time.Second
	DefaultRetryMaxWait = 30 * time.Second
	DefaultTimeout      = 30 * time.Second

	// maxResponseBodySize caps how much of a response body we are willing to
	// buffer. List endpoints return every object in an account in one
	// document, so the cap leaves room for large accounts while still stopping
	// a hostile or misconfigured endpoint from exhausting provider memory. A
	// response over the cap is an error, never silently truncated.
	maxResponseBodySize = 16 << 20 // 16 MiB

	// maxErrorMessageLength caps how much of an unparseable response body gets
	// echoed back into an APIError message.
	maxErrorMessageLength = 512
)

// tokenWaiter is the part of *rate.Limiter the client uses. Tests substitute a
// counting implementation so rate-limit behaviour is asserted without timing.
type tokenWaiter interface {
	Wait(ctx context.Context) error
}

// RetryPolicy is the retry behaviour applied to failed requests. Unlike the
// legacy ClientConfig fields, a zero value here is honoured: MaxRetries 0 means
// exactly one attempt and a zero wait means no delay between attempts.
type RetryPolicy struct {
	MaxRetries int
	MinWait    time.Duration
	MaxWait    time.Duration
}

type Client struct {
	httpClient   *http.Client
	baseURL      string
	apiToken     string
	customerID   string
	rateLimiter  tokenWaiter
	maxRetries   int
	retryMinWait time.Duration
	retryMaxWait time.Duration
	userAgent    string
	defaultTags  []string
	// sleep waits between retry attempts. It is a field so tests can record
	// the requested delay instead of waiting for it.
	sleep func(ctx context.Context, d time.Duration) error
}

type ClientConfig struct {
	APIToken   string
	CustomerID string
	BaseURL    string
	RateLimit  float64
	// MaxRetries, RetryMinWait and RetryMaxWait select the defaults when zero
	// or negative. Set Retry instead to request zero retries or zero waits.
	MaxRetries   int
	RetryMinWait time.Duration
	RetryMaxWait time.Duration
	// Retry, when set, overrides the three fields above and is applied as
	// given: zero values are honoured, negative values select the default.
	Retry       *RetryPolicy
	Timeout     time.Duration
	UserAgent   string
	DefaultTags []string
}

func NewClient(cfg ClientConfig) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.RateLimit <= 0 {
		cfg.RateLimit = DefaultRateLimit
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = DefaultMaxRetries
	}
	if cfg.RetryMinWait <= 0 {
		cfg.RetryMinWait = DefaultRetryMinWait
	}
	if cfg.RetryMaxWait <= 0 {
		cfg.RetryMaxWait = DefaultRetryMaxWait
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "terraform-provider-nodeping"
	}
	if r := cfg.Retry; r != nil {
		cfg.MaxRetries, cfg.RetryMinWait, cfg.RetryMaxWait = DefaultMaxRetries, DefaultRetryMinWait, DefaultRetryMaxWait
		if r.MaxRetries >= 0 {
			cfg.MaxRetries = r.MaxRetries
		}
		if r.MinWait >= 0 {
			cfg.RetryMinWait = r.MinWait
		}
		if r.MaxWait >= 0 {
			cfg.RetryMaxWait = r.MaxWait
		}
	}

	return &Client{
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		baseURL:      cfg.BaseURL,
		apiToken:     cfg.APIToken,
		customerID:   cfg.CustomerID,
		rateLimiter:  rate.NewLimiter(rate.Limit(cfg.RateLimit), 1),
		maxRetries:   cfg.MaxRetries,
		retryMinWait: cfg.RetryMinWait,
		retryMaxWait: cfg.RetryMaxWait,
		userAgent:    cfg.UserAgent,
		defaultTags:  cfg.DefaultTags,
		sleep:        sleepContext,
	}
}

// sleepContext waits for d, returning early with the context's error if it is
// cancelled first.
func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// RetryPolicy reports the retry behaviour the client applies.
func (c *Client) RetryPolicy() RetryPolicy {
	return RetryPolicy{MaxRetries: c.maxRetries, MinWait: c.retryMinWait, MaxWait: c.retryMaxWait}
}

func (c *Client) WithCustomerID(customerID string) *Client {
	return &Client{
		httpClient:   c.httpClient,
		baseURL:      c.baseURL,
		apiToken:     c.apiToken,
		customerID:   customerID,
		rateLimiter:  c.rateLimiter,
		maxRetries:   c.maxRetries,
		retryMinWait: c.retryMinWait,
		retryMaxWait: c.retryMaxWait,
		userAgent:    c.userAgent,
		defaultTags:  c.defaultTags,
		sleep:        c.sleep,
	}
}

func (c *Client) GetDefaultTags() []string {
	return c.defaultTags
}

type requestOptions struct {
	method     string
	path       string
	query      url.Values
	body       interface{}
	customerID string
}

// TransportError wraps a failure that happened at the network/transport layer,
// i.e. before a complete HTTP response was obtained.
//
// PreSend reports whether the failure provably occurred *before* the server
// could have acted on the request (connection refused, DNS resolution failure,
// TLS handshake failure). Only those failures are safe to retry for
// non-idempotent methods such as POST; anything else may have been received and
// executed by the server with only the response lost, and retrying it would
// create a duplicate resource.
type TransportError struct {
	Err       error
	PreSend   bool
	Retryable bool
}

func (e *TransportError) Error() string {
	return fmt.Sprintf("request failed: %v", e.Err)
}

func (e *TransportError) Unwrap() error { return e.Err }

// isIdempotentMethod reports whether the HTTP method may be replayed safely.
// NodePing's API documents POST as create-only and explicitly non-idempotent.
func isIdempotentMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete, http.MethodOptions:
		return true
	default:
		return false
	}
}

// classifyTransportError decides whether a transport-level failure is worth
// retrying, and whether the retry is safe for a non-idempotent method.
func classifyTransportError(err error) *TransportError {
	te := &TransportError{Err: err}

	// A cancelled or expired context is the caller's decision; never retry.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return te
	}

	// --- Failures that provably happened before the server saw the request ---

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		te.Retryable = true
		te.PreSend = true
		return te
	}

	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.EHOSTUNREACH) ||
		errors.Is(err, syscall.ENETUNREACH) || errors.Is(err, syscall.EHOSTDOWN) {
		te.Retryable = true
		te.PreSend = true
		return te
	}

	var recordHeaderErr tls.RecordHeaderError
	var certErr *tls.CertificateVerificationError
	var unknownAuthorityErr x509.UnknownAuthorityError
	var hostnameErr x509.HostnameError
	var certInvalidErr x509.CertificateInvalidError
	if errors.As(err, &recordHeaderErr) || errors.As(err, &certErr) ||
		errors.As(err, &unknownAuthorityErr) || errors.As(err, &hostnameErr) ||
		errors.As(err, &certInvalidErr) ||
		strings.Contains(err.Error(), "tls: handshake failure") ||
		strings.Contains(err.Error(), "remote error: tls:") {
		te.Retryable = true
		te.PreSend = true
		return te
	}

	// --- Failures that may have happened after the server acted ---

	if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		te.Retryable = true
		return te
	}

	// *url.Error itself satisfies net.Error, so unwrap it before asking whether
	// the underlying failure is genuinely a network condition.
	inner := err
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		inner = urlErr.Err
	}

	var netErr net.Error
	if errors.As(inner, &netErr) {
		// Timeouts and other transient network conditions are worth retrying
		// for idempotent methods, but the server may already have processed the
		// request, so they are never PreSend.
		te.Retryable = true
		return te
	}

	return te
}

// shouldRetry combines method idempotency with the error classification.
func shouldRetry(method string, err error) bool {
	idempotent := isIdempotentMethod(method)

	var transportErr *TransportError
	if errors.As(err, &transportErr) {
		if !transportErr.Retryable {
			return false
		}
		if idempotent {
			return true
		}
		// Non-idempotent: only replay when the server provably never acted.
		return transportErr.PreSend
	}

	// A 429 means the request was rejected by the rate limiter before any
	// resource was created, so it is safe to replay even for POST.
	var rateErr *RateLimitError
	if errors.As(err, &rateErr) {
		return true
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) {
		if !apiErr.IsRetryable() {
			return false
		}
		// A response-bearing 5xx means the server received the request. It may
		// have created the resource before failing, so POST must not be
		// replayed.
		return idempotent
	}

	// Marshal/unmarshal and other programming errors are never retryable.
	return false
}

func (c *Client) doRequest(ctx context.Context, opts requestOptions, result interface{}) error {
	var lastErr error
	var retryAfter time.Duration

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			if err := c.sleep(ctx, c.retryDelay(attempt, retryAfter)); err != nil {
				return err
			}
		}

		// Every HTTP attempt, including retries, must take a token so that a
		// storm of retries cannot outrun the configured rate limit.
		if err := c.rateLimiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limiter: %w", err)
		}

		err := c.executeRequest(ctx, opts, result)
		if err == nil {
			return nil
		}

		lastErr = err

		if !shouldRetry(opts.method, err) {
			return err
		}

		retryAfter = 0
		var rateErr *RateLimitError
		if errors.As(err, &rateErr) && rateErr.RetryAfter > 0 {
			retryAfter = time.Duration(rateErr.RetryAfter) * time.Second
		}
	}

	return lastErr
}

func (c *Client) executeRequest(ctx context.Context, opts requestOptions, result interface{}) error {
	reqURL, err := url.Parse(c.baseURL + opts.path)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	query := url.Values{}
	for k, v := range opts.query {
		query[k] = v
	}

	customerID := opts.customerID
	if customerID == "" {
		customerID = c.customerID
	}
	if customerID != "" {
		query.Set("customerid", customerID)
	}

	reqURL.RawQuery = query.Encode()

	var bodyReader io.Reader
	if opts.body != nil {
		jsonData, err := json.Marshal(opts.body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequestWithContext(ctx, opts.method, reqURL.String(), bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// The token goes in the basic-auth username position with an empty
	// password. It must never appear in the URL, where it would leak into
	// server logs and proxy traces.
	req.SetBasicAuth(c.apiToken, "")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if opts.body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// http.Client.Do wraps context errors in *url.Error; surface the
		// context error directly so callers can errors.Is it.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return classifyTransportError(err)
	}
	defer resp.Body.Close()

	// One byte past the cap is read so an oversized body is detected rather
	// than silently truncated into JSON that fails to parse.
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize+1))
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		// The server already acted; a truncated read is never PreSend.
		return classifyTransportError(fmt.Errorf("failed to read response body: %w", err))
	}
	if len(respBody) > maxResponseBodySize {
		return fmt.Errorf("response from %s exceeds %d bytes; refusing to parse a truncated body", opts.path, maxResponseBodySize)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.handleErrorResponse(resp.StatusCode, resp.Header, respBody)
	}

	// NodePing reports failures with `{"error": "..."}` under HTTP 200 as often
	// as it uses a 4xx (docs/assumptions.md A-002). Detecting that here means
	// every endpoint gets the same treatment, and callers only ever have to
	// reason about *APIError.
	if message, ok := errorMessageFromBody(respBody); ok {
		return &APIError{StatusCode: resp.StatusCode, Message: message}
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}

	return nil
}

// errorMessageFromBody extracts NodePing's `{"error": "..."}` payload. It is the
// single detector for an error carried in an otherwise successful response.
func errorMessageFromBody(body []byte) (string, bool) {
	if len(body) == 0 {
		return "", false
	}
	var errResp ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		return "", false
	}
	if errResp.Error == "" {
		return "", false
	}
	return truncateMessage(errResp.Error), true
}

func (c *Client) handleErrorResponse(statusCode int, header http.Header, body []byte) error {
	message, ok := errorMessageFromBody(body)
	if !ok {
		message = truncateMessage(string(body))
	}

	if statusCode == http.StatusTooManyRequests {
		retryAfter := 0
		if d, ok := parseRetryAfter(header, time.Now()); ok {
			retryAfter = int(math.Ceil(d.Seconds()))
		}
		return &RateLimitError{
			RetryAfter: retryAfter,
			StatusCode: statusCode,
			Message:    message,
		}
	}

	return &APIError{
		StatusCode: statusCode,
		Message:    message,
	}
}

func truncateMessage(s string) string {
	const limit = maxErrorMessageLength
	s = strings.TrimSpace(s)
	if len(s) <= limit {
		return s
	}
	// Back up to the start of a character so a multi-byte UTF-8 sequence is
	// never cut in half.
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "... (truncated)"
}

// parseRetryAfter understands both forms permitted by RFC 9110: a
// delta-seconds integer and an HTTP-date.
func parseRetryAfter(header http.Header, now time.Time) (time.Duration, bool) {
	if header == nil {
		return 0, false
	}
	raw := strings.TrimSpace(header.Get("Retry-After"))
	if raw == "" {
		return 0, false
	}

	if secs, err := strconv.Atoi(raw); err == nil {
		if secs < 0 {
			return 0, false
		}
		return time.Duration(secs) * time.Second, true
	}

	if t, err := http.ParseTime(raw); err == nil {
		d := t.Sub(now)
		if d < 0 {
			d = 0
		}
		return d, true
	}

	return 0, false
}

// retryDelay prefers a server-supplied Retry-After over local backoff, capped
// at retryMaxWait so a hostile or buggy header cannot stall Terraform forever.
func (c *Client) retryDelay(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		if retryAfter > c.retryMaxWait {
			return c.retryMaxWait
		}
		return retryAfter
	}
	return c.calculateBackoff(attempt)
}

func (c *Client) calculateBackoff(attempt int) time.Duration {
	backoff := float64(c.retryMinWait) * math.Pow(2, float64(attempt-1))

	// Jitter is applied first, then the cap, so the returned delay can never
	// exceed retryMaxWait.
	backoff += rand.Float64() * 0.3 * backoff //nolint:gosec // backoff jitter, not a security value

	if backoff > float64(c.retryMaxWait) {
		backoff = float64(c.retryMaxWait)
	}

	return time.Duration(backoff)
}

// notFoundOrWrap normalises the error from a single-resource request.
//
// A not-found signal — a 404, or a NodePing error body that says the object is
// gone, whatever status carried it — becomes a *NotFoundError so callers can
// keep matching with errors.As. Everything else is wrapped with operation as
// context and surfaced unchanged.
func notFoundOrWrap(err error, operation, resourceType, id string) error {
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.IsNotFound() {
		return &NotFoundError{ResourceType: resourceType, ResourceID: id}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

// absentAsNotFound turns "the response decoded cleanly but the document is
// empty" into a *NotFoundError.
//
// NodePing answers a GET for a missing resource with HTTP 200 and an empty
// object rather than a 404, so without this the resource would write a hollow
// record to state instead of being removed. What counts as empty is
// resource-specific, so each caller passes its own predicate result.
func absentAsNotFound(absent bool, resourceType, id string) error {
	if absent {
		return &NotFoundError{ResourceType: resourceType, ResourceID: id}
	}
	return nil
}

// validateResourceID rejects IDs that would cause the request to target the
// collection endpoint or escape the intended path.
func validateResourceID(resourceType, id string) error {
	if strings.TrimSpace(id) == "" {
		return &ValidationError{
			Field:   "id",
			Message: fmt.Sprintf("%s ID must not be empty", resourceType),
		}
	}
	if id == "." || id == ".." {
		return &ValidationError{
			Field:   "id",
			Message: fmt.Sprintf("%s ID %q is not a valid identifier", resourceType, id),
		}
	}
	if strings.ContainsAny(id, "\x00\r\n") {
		return &ValidationError{
			Field:   "id",
			Message: fmt.Sprintf("%s ID contains illegal characters", resourceType),
		}
	}
	return nil
}

func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func intToString(i int) string {
	return strconv.Itoa(i)
}
