package client

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// roundTripperFunc lets a test inject precise transport-level failures that a
// real httptest server cannot reproduce (connection refused, DNS failure, TLS
// handshake failure, connection reset).
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// fastClient returns a client with retry waits short enough for unit tests.
func fastClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	return NewClient(ClientConfig{
		APIToken:     "test-token",
		BaseURL:      baseURL,
		MaxRetries:   3,
		RetryMinWait: time.Millisecond,
		RetryMaxWait: 5 * time.Millisecond,
		RateLimit:    1000,
	})
}

func connRefusedErr() error {
	return &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: os.NewSyscallError("connect", syscall.ECONNREFUSED),
	}
}

func connResetErr() error {
	return &net.OpError{
		Op:  "read",
		Net: "tcp",
		Err: os.NewSyscallError("read", syscall.ECONNRESET),
	}
}

func dnsErr() error {
	return &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: &net.DNSError{Err: "no such host", Name: "api.nodeping.invalid", IsNotFound: true},
	}
}

// ---------------------------------------------------------------------------
// Finding 2: POST must never be replayed after a response-bearing failure.
// ---------------------------------------------------------------------------

func TestCreateCheckNotRetriedOn500(t *testing.T) {
	var posts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&posts, 1)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "boom"})
	}))
	defer server.Close()

	c := fastClient(t, server.URL)

	_, err := c.CreateCheck(context.Background(), CheckCreateRequest{Type: "HTTP"})
	if err == nil {
		t.Fatal("expected error from a 500 on create")
	}

	if got := atomic.LoadInt32(&posts); got != 1 {
		t.Fatalf("POST must not be replayed after a response-bearing 5xx: got %d requests, want 1", got)
	}
}

// A retried create must never leave two resources behind.
func TestCreateCheckCreatesExactlyOneResource(t *testing.T) {
	var created int32
	var requests int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		// The server acts first, then the response is "lost".
		id := atomic.AddInt32(&created, 1)
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("test server does not support hijacking")
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		_ = id
		_ = conn.Close() // drop the connection without writing a response
	}))
	defer server.Close()

	c := fastClient(t, server.URL)

	_, err := c.CreateCheck(context.Background(), CheckCreateRequest{Type: "HTTP"})
	if err == nil {
		t.Fatal("expected error when the response is lost")
	}

	if got := atomic.LoadInt32(&created); got != 1 {
		t.Fatalf("create must run exactly once even when the response is lost: server created %d resources", got)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("expected 1 POST, got %d", got)
	}
}

func TestPostRetriedOnlyWhenServerProvablyNeverActed(t *testing.T) {
	tests := []struct {
		name        string
		transport   error
		wantRetries bool
	}{
		{"connection refused", connRefusedErr(), true},
		{"dns failure", dnsErr(), true},
		{"tls handshake failure", errors.New("remote error: tls: handshake failure"), true},
		{"connection reset", connResetErr(), false},
		{"unexpected eof", io.ErrUnexpectedEOF, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var attempts int32
			c := fastClient(t, "http://example.invalid")
			c.httpClient.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				atomic.AddInt32(&attempts, 1)
				return nil, tt.transport
			})

			_, err := c.CreateCheck(context.Background(), CheckCreateRequest{Type: "HTTP"})
			if err == nil {
				t.Fatal("expected error")
			}

			got := atomic.LoadInt32(&attempts)
			if tt.wantRetries && int(got) != c.maxRetries+1 {
				t.Errorf("expected POST to be retried %d times, got %d attempts", c.maxRetries+1, got)
			}
			if !tt.wantRetries && got != 1 {
				t.Errorf("POST must not be replayed for %s: got %d attempts, want 1", tt.name, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Finding 3: network-level failures must be retried for idempotent methods.
// ---------------------------------------------------------------------------

func TestNetworkFailuresRetriedForIdempotentMethods(t *testing.T) {
	tests := []struct {
		name      string
		transport error
	}{
		{"connection reset", connResetErr()},
		{"connection refused", connRefusedErr()},
		{"unexpected eof", io.ErrUnexpectedEOF},
		{"dns failure", dnsErr()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var attempts int32
			c := fastClient(t, "http://example.invalid")
			c.httpClient.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				atomic.AddInt32(&attempts, 1)
				return nil, tt.transport
			})

			_, err := c.GetCheck(context.Background(), "abc")
			if err == nil {
				t.Fatal("expected error")
			}
			if got := atomic.LoadInt32(&attempts); int(got) != c.maxRetries+1 {
				t.Errorf("expected %d attempts, got %d", c.maxRetries+1, got)
			}
		})
	}
}

func TestNetworkFailureRecovers(t *testing.T) {
	var attempts int32
	c := fastClient(t, "http://example.invalid")
	c.httpClient.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if atomic.AddInt32(&attempts, 1) < 3 {
			return nil, connResetErr()
		}
		body := `{"_id":"abc","label":"ok"}`
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     http.Header{},
			Request:    r,
		}, nil
	})

	check, err := c.GetCheck(context.Background(), "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if check.Label != "ok" {
		t.Errorf("expected label 'ok', got %q", check.Label)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Errorf("expected 3 attempts, got %d", got)
	}
}

func TestUnmarshalErrorNotRetried(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"_id": "abc", "label":`) // truncated JSON
	}))
	defer server.Close()

	c := fastClient(t, server.URL)

	_, err := c.GetCheck(context.Background(), "abc")
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("expected an unmarshal error, got %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Errorf("malformed JSON must not be retried: got %d attempts", got)
	}
}

// ---------------------------------------------------------------------------
// Finding 4: Retry-After handling.
// ---------------------------------------------------------------------------

func TestRateLimitErrorCarriesRetryAfterSeconds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "slow down"})
	}))
	defer server.Close()

	c := NewClient(ClientConfig{
		APIToken:     "test-token",
		BaseURL:      server.URL,
		MaxRetries:   1,
		RetryMinWait: time.Millisecond,
		RetryMaxWait: time.Millisecond, // cap the sleep so the test stays fast
		RateLimit:    1000,
	})

	var result map[string]string
	err := c.doRequest(context.Background(), requestOptions{method: http.MethodGet, path: "/x"}, &result)

	var rateErr *RateLimitError
	if !errors.As(err, &rateErr) {
		t.Fatalf("expected *RateLimitError, got %T: %v", err, err)
	}
	if rateErr.RetryAfter != 7 {
		t.Errorf("expected RetryAfter 7, got %d", rateErr.RetryAfter)
	}

	// A 429 must still be reachable through the *APIError contract.
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatal("expected RateLimitError to unwrap to *APIError")
	}
	if apiErr.StatusCode != 429 {
		t.Errorf("expected status 429, got %d", apiErr.StatusCode)
	}
	if !apiErr.IsRetryable() {
		t.Error("429 should be retryable")
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		value string
		want  time.Duration
		ok    bool
	}{
		{"delta seconds", "30", 30 * time.Second, true},
		{"zero", "0", 0, true},
		{"http date in future", now.Add(45 * time.Second).Format(http.TimeFormat), 45 * time.Second, true},
		{"http date in past", now.Add(-time.Hour).Format(http.TimeFormat), 0, true},
		{"garbage", "soon please", 0, false},
		{"negative", "-5", 0, false},
		{"empty", "", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.Header{}
			if tt.value != "" {
				h.Set("Retry-After", tt.value)
			}
			got, ok := parseRetryAfter(h, now)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Errorf("duration = %v, want %v", got, tt.want)
			}
		})
	}

	if _, ok := parseRetryAfter(nil, now); ok {
		t.Error("nil header should not yield a Retry-After")
	}
}

func TestRetryDelayPrefersServerValue(t *testing.T) {
	c := NewClient(ClientConfig{
		APIToken:     "t",
		RetryMinWait: 1 * time.Second,
		RetryMaxWait: 30 * time.Second,
	})

	if got := c.retryDelay(1, 12*time.Second); got != 12*time.Second {
		t.Errorf("expected the server value 12s to win, got %v", got)
	}
	if got := c.retryDelay(1, 5*time.Minute); got != 30*time.Second {
		t.Errorf("expected the server value to be capped at retry_wait_max, got %v", got)
	}
	if got := c.retryDelay(1, 0); got < 1*time.Second {
		t.Errorf("expected local backoff when no server value, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// Finding 5: retries must take a rate-limiter token each.
// ---------------------------------------------------------------------------

// The pacing itself is golang.org/x/time/rate's job; what matters here is that
// the client builds the limiter from the configured rate with a burst of one.
func TestRateLimiterIsConfiguredFromRateLimit(t *testing.T) {
	c := NewClient(ClientConfig{APIToken: "t", RateLimit: 20})
	limiter, ok := c.rateLimiter.(*rate.Limiter)
	if !ok {
		t.Fatalf("expected a *rate.Limiter, got %T", c.rateLimiter)
	}
	if limiter.Limit() != 20 || limiter.Burst() != 1 {
		t.Errorf("limiter = %v/s burst %d, want 20/s burst 1", limiter.Limit(), limiter.Burst())
	}
}

// ---------------------------------------------------------------------------
// Finding 6: bounded body reads and truncated error messages.
// ---------------------------------------------------------------------------

func TestErrorMessageTruncated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, strings.Repeat("x", 10000)) // not JSON
	}))
	defer server.Close()

	c := fastClient(t, server.URL)

	var result map[string]string
	err := c.doRequest(context.Background(), requestOptions{method: http.MethodGet, path: "/x"}, &result)

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if len(apiErr.Message) > maxErrorMessageLength+len("... (truncated)") {
		t.Errorf("error message not truncated: %d bytes", len(apiErr.Message))
	}
	if !strings.HasSuffix(apiErr.Message, "(truncated)") {
		t.Error("expected the truncated marker on an oversized error body")
	}
}

func TestTruncateMessage(t *testing.T) {
	if got := truncateMessage("  short  "); got != "short" {
		t.Errorf("expected trimmed short message, got %q", got)
	}
	long := strings.Repeat("y", 600)
	got := truncateMessage(long)
	if len(got) != 512+len("... (truncated)") {
		t.Errorf("unexpected truncated length %d", len(got))
	}
}

// ---------------------------------------------------------------------------
// Finding 7: only 2xx counts as success.
// ---------------------------------------------------------------------------

func TestNon2xxStatusHandling(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		wantErr   bool
		wantCode  int
		attemptsN int32
	}{
		{name: "200 ok", status: 200, body: `{"status":"ok"}`, wantErr: false, attemptsN: 1},
		{name: "204 no content", status: 204, body: "", wantErr: false, attemptsN: 1},
		{name: "301 moved", status: 301, body: "", wantErr: true, wantCode: 301, attemptsN: 1},
		{name: "302 found", status: 302, body: "", wantErr: true, wantCode: 302, attemptsN: 1},
		{name: "304 not modified", status: 304, body: "", wantErr: true, wantCode: 304, attemptsN: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var attempts int32
			c := fastClient(t, "http://example.invalid")
			c.httpClient.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				atomic.AddInt32(&attempts, 1)
				return &http.Response{
					StatusCode: tt.status,
					Body:       io.NopCloser(strings.NewReader(tt.body)),
					Header:     http.Header{},
					Request:    r,
				}, nil
			})

			var result map[string]string
			err := c.doRequest(context.Background(), requestOptions{method: http.MethodGet, path: "/x"}, &result)

			if tt.wantErr {
				var apiErr *APIError
				if !errors.As(err, &apiErr) {
					t.Fatalf("expected *APIError for status %d, got %T (%v)", tt.status, err, err)
				}
				if apiErr.StatusCode != tt.wantCode {
					t.Errorf("expected status %d, got %d", tt.wantCode, apiErr.StatusCode)
				}
			} else if err != nil {
				t.Fatalf("unexpected error for status %d: %v", tt.status, err)
			}

			if got := atomic.LoadInt32(&attempts); got != tt.attemptsN {
				t.Errorf("expected %d attempts, got %d", tt.attemptsN, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Finding 8: jitter must not push the delay past retry_wait_max.
// ---------------------------------------------------------------------------

func TestCalculateBackoffNeverExceedsMax(t *testing.T) {
	c := NewClient(ClientConfig{
		APIToken:     "t",
		RetryMinWait: 1 * time.Second,
		RetryMaxWait: 30 * time.Second,
	})

	for attempt := 1; attempt <= 20; attempt++ {
		for i := 0; i < 200; i++ {
			got := c.calculateBackoff(attempt)
			if got > c.retryMaxWait {
				t.Fatalf("attempt %d: backoff %v exceeds retry_wait_max %v", attempt, got, c.retryMaxWait)
			}
			if got <= 0 {
				t.Fatalf("attempt %d: non-positive backoff %v", attempt, got)
			}
		}
	}
}

func TestCalculateBackoffGrowsExponentially(t *testing.T) {
	c := NewClient(ClientConfig{
		APIToken:     "t",
		RetryMinWait: 1 * time.Second,
		RetryMaxWait: 30 * time.Second,
	})

	if b := c.calculateBackoff(1); b < 1*time.Second || b > 1300*time.Millisecond {
		t.Errorf("attempt 1 backoff should be 1s..1.3s, got %v", b)
	}
	if b := c.calculateBackoff(2); b < 2*time.Second || b > 2600*time.Millisecond {
		t.Errorf("attempt 2 backoff should be 2s..2.6s, got %v", b)
	}
	if b := c.calculateBackoff(5); b < 16*time.Second || b > 20800*time.Millisecond {
		t.Errorf("attempt 5 backoff should be 16s..20.8s, got %v", b)
	}
	if b := c.calculateBackoff(10); b != 30*time.Second {
		t.Errorf("attempt 10 backoff should saturate at 30s, got %v", b)
	}
}

// ---------------------------------------------------------------------------
// Finding 9 + path escaping.
// ---------------------------------------------------------------------------

func TestEmptyIDRejected(t *testing.T) {
	var reached int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&reached, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()

	c := fastClient(t, server.URL)
	ctx := context.Background()

	for _, id := range []string{"", "   ", ".", ".."} {
		t.Run(fmt.Sprintf("id=%q", id), func(t *testing.T) {
			var valErr *ValidationError

			if _, err := c.GetCheck(ctx, id); !errors.As(err, &valErr) {
				t.Errorf("GetCheck(%q): expected *ValidationError, got %T (%v)", id, err, err)
			}
			if err := c.DeleteCheck(ctx, id); !errors.As(err, &valErr) {
				t.Errorf("DeleteCheck(%q): expected *ValidationError, got %T (%v)", id, err, err)
			}
			if _, err := c.UpdateCheck(ctx, id, CheckUpdateRequest{}); !errors.As(err, &valErr) {
				t.Errorf("UpdateCheck(%q): expected *ValidationError, got %T (%v)", id, err, err)
			}
			if _, err := c.GetContact(ctx, id); !errors.As(err, &valErr) {
				t.Errorf("GetContact(%q): expected *ValidationError, got %T (%v)", id, err, err)
			}
			if err := c.DeleteContact(ctx, id); !errors.As(err, &valErr) {
				t.Errorf("DeleteContact(%q): expected *ValidationError, got %T (%v)", id, err, err)
			}
			if _, err := c.UpdateContact(ctx, id, ContactUpdateRequest{}); !errors.As(err, &valErr) {
				t.Errorf("UpdateContact(%q): expected *ValidationError, got %T (%v)", id, err, err)
			}
		})
	}

	if got := atomic.LoadInt32(&reached); got != 0 {
		t.Errorf("an invalid ID must never reach the network, but %d requests were sent", got)
	}
}

func TestIDPathEscaping(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		wantPath string
	}{
		{"slash", "a/b", "/checks/a%2Fb"},
		{"question mark", "a?b=c", "/checks/a%3Fb=c"},
		{"dot dot segment", "x/../../etc", "/checks/x%2F..%2F..%2Fetc"},
		{"space", "a b", "/checks/a%20b"},
		{"hash", "a#b", "/checks/a%23b"},
		{"percent", "a%2Fb", "/checks/a%252Fb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rawPath string
			var gotQuery string
			c := fastClient(t, "http://example.invalid")
			c.httpClient.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				rawPath = r.URL.EscapedPath()
				gotQuery = r.URL.RawQuery
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader(`{"_id":"x"}`)),
					Header:     http.Header{},
					Request:    r,
				}, nil
			})

			if _, err := c.GetCheck(context.Background(), tt.id); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if rawPath != tt.wantPath {
				t.Errorf("path = %q, want %q", rawPath, tt.wantPath)
			}
			if strings.Contains(gotQuery, "b=c") {
				t.Errorf("an ID containing '?' leaked into the query string: %q", gotQuery)
			}
		})
	}
}

func TestQueryOptionsAreNotMutatedAcrossRetries(t *testing.T) {
	var seen []string
	c := fastClient(t, "http://example.invalid")
	c.customerID = "CUST1"
	c.httpClient.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		seen = append(seen, r.URL.RawQuery)
		return nil, connResetErr()
	})

	opts := requestOptions{method: http.MethodGet, path: "/checks"}
	_ = c.doRequest(context.Background(), opts, nil)

	if opts.query != nil {
		t.Error("doRequest must not mutate the caller's requestOptions.query")
	}
	for _, q := range seen {
		if q != "customerid=CUST1" {
			t.Errorf("unexpected query %q", q)
		}
	}
}

// ---------------------------------------------------------------------------
// Context cancellation.
// ---------------------------------------------------------------------------

func TestContextCancellationDuringRequest(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	defer close(release)

	c := fastClient(t, server.URL)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	var result map[string]string
	err := c.doRequest(ctx, requestOptions{method: http.MethodGet, path: "/x"}, &result)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %T: %v", err, err)
	}
}

func TestContextCancellationDuringRetryBackoff(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewClient(ClientConfig{
		APIToken:     "t",
		BaseURL:      server.URL,
		MaxRetries:   5,
		RetryMinWait: 2 * time.Second,
		RetryMaxWait: 5 * time.Second,
		RateLimit:    1000,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	var result map[string]string
	start := time.Now()
	err := c.doRequest(ctx, requestOptions{method: http.MethodGet, path: "/x"}, &result)
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %T: %v", err, err)
	}
	if elapsed > time.Second {
		t.Errorf("backoff did not abort on context expiry: took %v", elapsed)
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Errorf("expected 1 attempt before the deadline, got %d", got)
	}
}

func TestContextAlreadyCancelled(t *testing.T) {
	var attempts int32
	c := fastClient(t, "http://example.invalid")
	c.httpClient.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		return nil, errors.New("should not be called")
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := c.doRequest(ctx, requestOptions{method: http.MethodGet, path: "/x"}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 0 {
		t.Errorf("no request should be issued with a cancelled context, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// Auth.
// ---------------------------------------------------------------------------

func TestBasicAuthTokenPlacement(t *testing.T) {
	const token = "my-secret-token"

	var authHeader, rawURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		rawURL = r.URL.String()
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	}))
	defer server.Close()

	c := NewClient(ClientConfig{APIToken: token, CustomerID: "CUST", BaseURL: server.URL})

	var result map[string]string
	if err := c.doRequest(context.Background(), requestOptions{method: http.MethodGet, path: "/test"}, &result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	const prefix = "Basic "
	if !strings.HasPrefix(authHeader, prefix) {
		t.Fatalf("expected a Basic auth header, got %q", authHeader)
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(authHeader, prefix))
	if err != nil {
		t.Fatalf("Authorization header is not valid base64: %v", err)
	}

	user, pass, found := strings.Cut(string(decoded), ":")
	if !found {
		t.Fatalf("malformed basic auth payload %q", decoded)
	}
	if user != token {
		t.Errorf("token must be in the username position: got username %q, want %q", user, token)
	}
	if pass != "" {
		t.Errorf("expected an empty password, got %q", pass)
	}

	if strings.Contains(rawURL, token) {
		t.Errorf("API token leaked into the request URL: %q", rawURL)
	}
}

// ---------------------------------------------------------------------------
// Classification unit tests.
// ---------------------------------------------------------------------------

func TestIsIdempotentMethod(t *testing.T) {
	for _, m := range []string{http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete, http.MethodOptions} {
		if !isIdempotentMethod(m) {
			t.Errorf("%s should be idempotent", m)
		}
	}
	for _, m := range []string{http.MethodPost, http.MethodPatch} {
		if isIdempotentMethod(m) {
			t.Errorf("%s must not be treated as idempotent", m)
		}
	}
}

func TestShouldRetry(t *testing.T) {
	tests := []struct {
		name   string
		method string
		err    error
		want   bool
	}{
		{"GET 500", http.MethodGet, &APIError{StatusCode: 500}, true},
		{"POST 500", http.MethodPost, &APIError{StatusCode: 500}, false},
		{"PUT 503", http.MethodPut, &APIError{StatusCode: 503}, true},
		{"DELETE 502", http.MethodDelete, &APIError{StatusCode: 502}, true},
		{"GET 400", http.MethodGet, &APIError{StatusCode: 400}, false},
		{"GET 404", http.MethodGet, &APIError{StatusCode: 404}, false},
		{"POST 429", http.MethodPost, &RateLimitError{StatusCode: 429}, true},
		{"GET 429", http.MethodGet, &RateLimitError{StatusCode: 429}, true},
		{"GET conn reset", http.MethodGet, classifyTransportError(connResetErr()), true},
		{"POST conn reset", http.MethodPost, classifyTransportError(connResetErr()), false},
		{"POST conn refused", http.MethodPost, classifyTransportError(connRefusedErr()), true},
		{"POST dns", http.MethodPost, classifyTransportError(dnsErr()), true},
		{"GET context canceled", http.MethodGet, classifyTransportError(context.Canceled), false},
		{"GET unmarshal error", http.MethodGet, errors.New("failed to unmarshal response"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRetry(tt.method, tt.err); got != tt.want {
				t.Errorf("shouldRetry(%s, %v) = %v, want %v", tt.method, tt.err, got, tt.want)
			}
		})
	}
}

func TestTransportErrorMessage(t *testing.T) {
	te := classifyTransportError(connRefusedErr())
	if !strings.Contains(te.Error(), "request failed") {
		t.Errorf("unexpected message %q", te.Error())
	}
	if !errors.Is(te, syscall.ECONNREFUSED) {
		t.Error("expected the transport error to unwrap to ECONNREFUSED")
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestClassifyTransportError(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		wantRetryable bool
		wantPreSend   bool
	}{
		{"context canceled", context.Canceled, false, false},
		{"context deadline", context.DeadlineExceeded, false, false},
		{"dns", dnsErr(), true, true},
		{"conn refused", connRefusedErr(), true, true},
		{"host unreachable", &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.EHOSTUNREACH)}, true, true},
		{"net unreachable", &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ENETUNREACH)}, true, true},
		{"tls handshake", errors.New("remote error: tls: handshake failure"), true, true},
		{"x509 unknown authority", &url.Error{Op: "Get", URL: "https://x", Err: x509.UnknownAuthorityError{}}, true, true},
		{"x509 hostname", &url.Error{Op: "Get", URL: "https://x", Err: x509.HostnameError{Host: "x"}}, true, true},
		{"conn reset", connResetErr(), true, false},
		{"broken pipe", &net.OpError{Op: "write", Err: os.NewSyscallError("write", syscall.EPIPE)}, true, false},
		{"unexpected eof", io.ErrUnexpectedEOF, true, false},
		{"eof", io.EOF, true, false},
		{"timeout", &net.OpError{Op: "read", Err: timeoutErr{}}, true, false},
		{"url error wrapping timeout", &url.Error{Op: "Get", URL: "http://x", Err: timeoutErr{}}, true, false},
		{"plain error", errors.New("something odd"), false, false},
		{"url error wrapping plain", &url.Error{Op: "Get", URL: "http://x", Err: errors.New("bad scheme")}, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := classifyTransportError(tt.err)
			if te.Retryable != tt.wantRetryable {
				t.Errorf("Retryable = %v, want %v", te.Retryable, tt.wantRetryable)
			}
			if te.PreSend != tt.wantPreSend {
				t.Errorf("PreSend = %v, want %v", te.PreSend, tt.wantPreSend)
			}
			if te.PreSend && !te.Retryable {
				t.Error("PreSend implies Retryable")
			}
		})
	}
}

func TestValidateResourceIDRejectsControlCharacters(t *testing.T) {
	var valErr *ValidationError
	for _, id := range []string{"a\x00b", "a\nb", "a\rb"} {
		if err := validateResourceID("check", id); !errors.As(err, &valErr) {
			t.Errorf("validateResourceID(%q) = %v, want a *ValidationError", id, err)
		}
	}
	if err := validateResourceID("check", "201205050153W2Q4C-0J2HSIRF"); err != nil {
		t.Errorf("a normal ID must be accepted, got %v", err)
	}
}

func TestInvalidBaseURLReported(t *testing.T) {
	c := NewClient(ClientConfig{APIToken: "t", BaseURL: "http://[::1]:namedport"})
	err := c.doRequest(context.Background(), requestOptions{method: http.MethodGet, path: "/x"}, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid URL") {
		t.Fatalf("expected an invalid URL error, got %v", err)
	}
}

func TestUnmarshalableRequestBodyReported(t *testing.T) {
	c := fastClient(t, "http://example.invalid")
	c.httpClient.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		t.Error("no request should be issued for an unmarshalable body")
		return nil, errors.New("unreachable")
	})

	err := c.doRequest(context.Background(), requestOptions{
		method: http.MethodPost,
		path:   "/checks",
		body:   map[string]interface{}{"bad": make(chan int)},
	}, nil)

	if err == nil || !strings.Contains(err.Error(), "marshal") {
		t.Fatalf("expected a marshal error, got %v", err)
	}
}

func TestListChecksErrorPropagates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "bad token"})
	}))
	defer server.Close()

	c := fastClient(t, server.URL)

	if _, err := c.ListChecks(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
	if _, err := c.ListContacts(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
}

func TestUpdateErrorPropagates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "nope"})
	}))
	defer server.Close()

	c := fastClient(t, server.URL)

	if _, err := c.UpdateCheck(context.Background(), "abc", CheckUpdateRequest{}); err == nil {
		t.Fatal("expected an error")
	}
	if _, err := c.UpdateContact(context.Background(), "abc", ContactUpdateRequest{}); err == nil {
		t.Fatal("expected an error")
	}
}
