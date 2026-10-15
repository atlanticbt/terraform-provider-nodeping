package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	cfg := ClientConfig{
		APIToken:   "test-token",
		CustomerID: "test-customer",
		BaseURL:    "https://api.example.com",
		RateLimit:  5,
		MaxRetries: 2,
	}

	c := NewClient(cfg)

	if c.apiToken != cfg.APIToken {
		t.Errorf("expected apiToken %q, got %q", cfg.APIToken, c.apiToken)
	}
	if c.customerID != cfg.CustomerID {
		t.Errorf("expected customerID %q, got %q", cfg.CustomerID, c.customerID)
	}
	if c.baseURL != cfg.BaseURL {
		t.Errorf("expected baseURL %q, got %q", cfg.BaseURL, c.baseURL)
	}
	if c.maxRetries != cfg.MaxRetries {
		t.Errorf("expected maxRetries %d, got %d", cfg.MaxRetries, c.maxRetries)
	}
}

func TestNewClientDefaults(t *testing.T) {
	cfg := ClientConfig{
		APIToken: "test-token",
	}

	c := NewClient(cfg)

	if c.baseURL != DefaultBaseURL {
		t.Errorf("expected default baseURL %q, got %q", DefaultBaseURL, c.baseURL)
	}
	if c.maxRetries != DefaultMaxRetries {
		t.Errorf("expected default maxRetries %d, got %d", DefaultMaxRetries, c.maxRetries)
	}
}

func TestWithCustomerID(t *testing.T) {
	c := NewClient(ClientConfig{
		APIToken:   "test-token",
		CustomerID: "original",
	})

	newClient := c.WithCustomerID("new-customer")

	if newClient.customerID != "new-customer" {
		t.Errorf("expected customerID %q, got %q", "new-customer", newClient.customerID)
	}
	if c.customerID != "original" {
		t.Errorf("original client customerID should not change, got %q", c.customerID)
	}
}

// TestDoRequestBasicAuth asserts the credential placement itself, not merely
// that some Authorization header exists. See also TestBasicAuthTokenPlacement.
func TestDoRequestBasicAuth(t *testing.T) {
	const token = "my-secret-token"

	var gotUser, gotPass string
	var gotOK bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass, gotOK = r.BasicAuth()
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	c := NewClient(ClientConfig{
		APIToken: token,
		BaseURL:  server.URL,
	})

	var result map[string]string
	err := c.doRequest(context.Background(), requestOptions{
		method: http.MethodGet,
		path:   "/test",
	}, &result)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !gotOK {
		t.Fatal("expected a parseable Basic auth header")
	}
	if gotUser != token {
		t.Errorf("token must be the basic-auth username: got %q, want %q", gotUser, token)
	}
	if gotPass != "" {
		t.Errorf("expected empty password, got %q", gotPass)
	}
}

func TestDoRequestCustomerID(t *testing.T) {
	var receivedCustomerID string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedCustomerID = r.URL.Query().Get("customerid")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	c := NewClient(ClientConfig{
		APIToken:   "test-token",
		CustomerID: "test-customer-123",
		BaseURL:    server.URL,
	})

	var result map[string]string
	err := c.doRequest(context.Background(), requestOptions{
		method: http.MethodGet,
		path:   "/test",
	}, &result)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedCustomerID != "test-customer-123" {
		t.Errorf("expected customerid %q, got %q", "test-customer-123", receivedCustomerID)
	}
}

func TestDoRequestRetry(t *testing.T) {
	attempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "temporary error"})
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	c := NewClient(ClientConfig{
		APIToken:     "test-token",
		BaseURL:      server.URL,
		MaxRetries:   3,
		RetryMinWait: 10 * time.Millisecond,
		RetryMaxWait: 50 * time.Millisecond,
	})

	var result map[string]string
	err := c.doRequest(context.Background(), requestOptions{
		method: http.MethodGet,
		path:   "/test",
	}, &result)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestDoRequestNoRetryOn400(t *testing.T) {
	attempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "bad request"})
	}))
	defer server.Close()

	c := NewClient(ClientConfig{
		APIToken:   "test-token",
		BaseURL:    server.URL,
		MaxRetries: 3,
	})

	var result map[string]string
	err := c.doRequest(context.Background(), requestOptions{
		method: http.MethodGet,
		path:   "/test",
	}, &result)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if attempts != 1 {
		t.Errorf("expected 1 attempt (no retry on 400), got %d", attempts)
	}

	var apiErr *APIError
	ok := errors.As(err, &apiErr)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("expected status code 400, got %d", apiErr.StatusCode)
	}
}

func TestAPIErrorMethods(t *testing.T) {
	tests := []struct {
		name        string
		err         *APIError
		isNotFound  bool
		isUnauth    bool
		isRetryable bool
	}{
		{
			name:        "404 Not Found",
			err:         &APIError{StatusCode: 404, Message: "not found"},
			isNotFound:  true,
			isUnauth:    false,
			isRetryable: false,
		},
		{
			name:        "401 Unauthorized",
			err:         &APIError{StatusCode: 401, Message: "unauthorized"},
			isNotFound:  false,
			isUnauth:    true,
			isRetryable: false,
		},
		{
			name:        "403 Forbidden",
			err:         &APIError{StatusCode: 403, Message: "forbidden"},
			isNotFound:  false,
			isUnauth:    true,
			isRetryable: false,
		},
		{
			name:        "429 Rate Limit",
			err:         &APIError{StatusCode: 429, Message: "rate limit"},
			isNotFound:  false,
			isUnauth:    false,
			isRetryable: true,
		},
		{
			name:        "500 Internal Error",
			err:         &APIError{StatusCode: 500, Message: "internal error"},
			isNotFound:  false,
			isUnauth:    false,
			isRetryable: true,
		},
		{
			name:        "503 Service Unavailable",
			err:         &APIError{StatusCode: 503, Message: "service unavailable"},
			isNotFound:  false,
			isUnauth:    false,
			isRetryable: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.IsNotFound(); got != tt.isNotFound {
				t.Errorf("IsNotFound() = %v, want %v", got, tt.isNotFound)
			}
			if got := tt.err.IsUnauthorized(); got != tt.isUnauth {
				t.Errorf("IsUnauthorized() = %v, want %v", got, tt.isUnauth)
			}
			if got := tt.err.IsRetryable(); got != tt.isRetryable {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.isRetryable)
			}
		})
	}
}

// Backoff behaviour is covered by TestCalculateBackoffNeverExceedsMax and
// TestCalculateBackoffGrowsExponentially in retry_test.go. The former replaces
// an earlier test that asserted a cap of 40s for a 30s retry_wait_max, which
// codified the bug of applying jitter after the cap.
