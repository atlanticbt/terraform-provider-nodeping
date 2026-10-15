package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

// recordingSleep replaces the client's retry sleep and records each requested
// delay, so retry tests assert the delay without waiting for it.
func recordingSleep(c *Client) *[]time.Duration {
	var waits []time.Duration
	c.sleep = func(_ context.Context, d time.Duration) error {
		waits = append(waits, d)
		return nil
	}
	return &waits
}

type countingLimiter struct{ calls int32 }

func (l *countingLimiter) Wait(context.Context) error {
	atomic.AddInt32(&l.calls, 1)
	return nil
}

func failingServer(t *testing.T, attempts *int32) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestRetryPolicy_ShouldMakeExactlyOneAttemptWhenMaxRetriesIsZero(t *testing.T) {
	var attempts int32
	server := failingServer(t, &attempts)

	c := NewClient(ClientConfig{
		APIToken:  "t",
		BaseURL:   server.URL,
		RateLimit: 1000,
		Retry:     &RetryPolicy{MaxRetries: 0, MinWait: time.Second, MaxWait: time.Second},
	})
	waits := recordingSleep(c)

	var result map[string]string
	if err := c.doRequest(context.Background(), requestOptions{method: http.MethodGet, path: "/x"}, &result); err == nil {
		t.Fatal("expected the 500 to be returned")
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Errorf("max_retries = 0 made %d HTTP attempts, want exactly 1", got)
	}
	if len(*waits) != 0 {
		t.Errorf("max_retries = 0 slept %v, want no retry delay", *waits)
	}
}

func TestRetryPolicy_ShouldRetryWithoutWaitingWhenWaitsAreZero(t *testing.T) {
	var attempts int32
	server := failingServer(t, &attempts)

	c := NewClient(ClientConfig{
		APIToken:  "t",
		BaseURL:   server.URL,
		RateLimit: 1000,
		Retry:     &RetryPolicy{MaxRetries: 2, MinWait: 0, MaxWait: 0},
	})
	waits := recordingSleep(c)

	var result map[string]string
	_ = c.doRequest(context.Background(), requestOptions{method: http.MethodGet, path: "/x"}, &result)

	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Errorf("max_retries = 2 made %d attempts, want 3", got)
	}
	for _, w := range *waits {
		if w != 0 {
			t.Errorf("retry_wait_min = retry_wait_max = 0 requested a %v delay, want 0", w)
		}
	}
}

func TestRetryPolicy_ShouldUseDefaultsWhenNotConfigured(t *testing.T) {
	got := NewClient(ClientConfig{APIToken: "t"}).RetryPolicy()
	want := RetryPolicy{MaxRetries: DefaultMaxRetries, MinWait: DefaultRetryMinWait, MaxWait: DefaultRetryMaxWait}
	if got != want {
		t.Errorf("unconfigured retry policy = %+v, want the defaults %+v", got, want)
	}
}

func TestRetryPolicy_ShouldTreatNegativeValuesAsUnset(t *testing.T) {
	got := NewClient(ClientConfig{
		APIToken: "t",
		Retry:    &RetryPolicy{MaxRetries: -1, MinWait: -time.Second, MaxWait: -time.Second},
	}).RetryPolicy()
	want := RetryPolicy{MaxRetries: DefaultMaxRetries, MinWait: DefaultRetryMinWait, MaxWait: DefaultRetryMaxWait}
	if got != want {
		t.Errorf("negative retry policy = %+v, want the defaults %+v", got, want)
	}
}

func TestRetryAfter_ShouldBeRequestedBetweenAttemptsWithoutRealWaiting(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	}))
	defer server.Close()

	c := NewClient(ClientConfig{
		APIToken:     "t",
		BaseURL:      server.URL,
		MaxRetries:   2,
		RetryMinWait: time.Microsecond, // far below the server's 1s
		RetryMaxWait: 2 * time.Second,
		RateLimit:    1000,
	})
	waits := recordingSleep(c)

	var result map[string]string
	if err := c.doRequest(context.Background(), requestOptions{method: http.MethodGet, path: "/x"}, &result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(*waits) != 1 || (*waits)[0] != time.Second {
		t.Errorf("expected one wait of the server-requested 1s, got %v", *waits)
	}
}

func TestRateLimiter_ShouldBeConsultedOnEveryAttemptIncludingRetries(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	}))
	defer server.Close()

	c := NewClient(ClientConfig{APIToken: "t", BaseURL: server.URL, MaxRetries: 3})
	recordingSleep(c)
	limiter := &countingLimiter{}
	c.rateLimiter = limiter

	var result map[string]string
	if err := c.doRequest(context.Background(), requestOptions{method: http.MethodGet, path: "/x"}, &result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := atomic.LoadInt32(&limiter.calls); got != 3 {
		t.Errorf("3 attempts took %d rate-limiter tokens, want 3", got)
	}
}

func TestResponseBody_ShouldBeRejectedWhenLargerThanTheCap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"label":"`)
		_, _ = io.WriteString(w, strings.Repeat("A", maxResponseBodySize))
		_, _ = io.WriteString(w, `"}`)
	}))
	defer server.Close()

	c := fastClient(t, server.URL)
	var result map[string]string
	err := c.doRequest(context.Background(), requestOptions{method: http.MethodGet, path: "/checks"}, &result)
	if err == nil {
		t.Fatal("expected an error for a response larger than the cap")
	}
	if !strings.Contains(err.Error(), "/checks") || !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error should name the path and say the limit was exceeded, got %q", err)
	}
}

func TestResponseBody_ShouldParseLargeListsUnderTheCap(t *testing.T) {
	big := strings.Repeat("B", 4<<20) // 4 MiB, above the old 1 MiB cap
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"label":"`+big+`"}`)
	}))
	defer server.Close()

	c := fastClient(t, server.URL)
	var result map[string]string
	if err := c.doRequest(context.Background(), requestOptions{method: http.MethodGet, path: "/checks"}, &result); err != nil {
		t.Fatalf("a 4 MiB response should parse: %v", err)
	}
	if len(result["label"]) != len(big) {
		t.Errorf("label decoded to %d bytes, want %d", len(result["label"]), len(big))
	}
}

func TestTruncateMessage_ShouldNotSplitAMultiByteCharacter(t *testing.T) {
	// "é" is two bytes; placing it across the 512-byte limit would split it.
	msg := strings.Repeat("a", 511) + "é" + strings.Repeat("b", 100)
	got := truncateMessage(msg)
	if !utf8.ValidString(got) {
		t.Fatalf("truncated message is not valid UTF-8: %q", got[505:])
	}
	if !strings.HasPrefix(got, strings.Repeat("a", 511)+"...") {
		t.Errorf("expected truncation before the multi-byte character, got suffix %q", got[505:])
	}
}
