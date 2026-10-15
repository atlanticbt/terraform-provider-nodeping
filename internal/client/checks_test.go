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

func TestListChecks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/checks" {
			t.Errorf("expected path /checks, got %s", r.URL.Path)
		}

		checks := map[string]Check{
			"201205050153W2Q4C-0J2HSIRF": {
				ID:         "201205050153W2Q4C-0J2HSIRF",
				CustomerID: "201205050153W2Q4C",
				Label:      "Test Check",
				Type:       "HTTP",
				Enabled:    "active",
				Parameters: CheckParameters{
					Target:    "https://example.com",
					Threshold: 5,
				},
			},
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(checks)
	}))
	defer server.Close()

	c := NewClient(ClientConfig{
		APIToken: "test-token",
		BaseURL:  server.URL,
	})

	checks, err := c.ListChecks(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(checks) != 1 {
		t.Errorf("expected 1 check, got %d", len(checks))
	}

	check, ok := checks["201205050153W2Q4C-0J2HSIRF"]
	if !ok {
		t.Fatal("expected check not found")
	}

	if check.Label != "Test Check" {
		t.Errorf("expected label 'Test Check', got %q", check.Label)
	}
}

func TestGetCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}

		check := Check{
			ID:         "201205050153W2Q4C-0J2HSIRF",
			CustomerID: "201205050153W2Q4C",
			Label:      "Test Check",
			Type:       "HTTP",
			Enabled:    "active",
			State:      1,
			Parameters: CheckParameters{
				Target:    "https://example.com",
				Threshold: 5,
			},
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(check)
	}))
	defer server.Close()

	c := NewClient(ClientConfig{
		APIToken: "test-token",
		BaseURL:  server.URL,
	})

	check, err := c.GetCheck(context.Background(), "201205050153W2Q4C-0J2HSIRF")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if check.ID != "201205050153W2Q4C-0J2HSIRF" {
		t.Errorf("expected ID '201205050153W2Q4C-0J2HSIRF', got %q", check.ID)
	}

	if check.State != 1 {
		t.Errorf("expected state 1, got %d", check.State)
	}
}

func TestGetCheckNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "Check not found"})
	}))
	defer server.Close()

	c := NewClient(ClientConfig{
		APIToken: "test-token",
		BaseURL:  server.URL,
	})

	_, err := c.GetCheck(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var notFound *NotFoundError
	ok := errors.As(err, &notFound)
	if !ok {
		t.Errorf("expected *NotFoundError, got %T", err)
	}
}

func TestCreateCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var req CheckCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if req.Type != "HTTP" {
			t.Errorf("expected type 'HTTP', got %q", req.Type)
		}

		check := Check{
			ID:         "201205050153W2Q4C-NEWCHK",
			CustomerID: "201205050153W2Q4C",
			Label:      req.Label,
			Type:       req.Type,
			Enabled:    req.Enabled,
			Parameters: CheckParameters{
				Target: req.Target,
			},
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(check)
	}))
	defer server.Close()

	c := NewClient(ClientConfig{
		APIToken: "test-token",
		BaseURL:  server.URL,
	})

	check, err := c.CreateCheck(context.Background(), CheckCreateRequest{
		Type:    "HTTP",
		Target:  "https://example.com",
		Label:   "New Check",
		Enabled: "active",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if check.ID != "201205050153W2Q4C-NEWCHK" {
		t.Errorf("expected ID '201205050153W2Q4C-NEWCHK', got %q", check.ID)
	}
}

func TestUpdateCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}

		check := Check{
			ID:         "201205050153W2Q4C-0J2HSIRF",
			CustomerID: "201205050153W2Q4C",
			Label:      "Updated Check",
			Type:       "HTTP",
			Enabled:    "active",
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(check)
	}))
	defer server.Close()

	c := NewClient(ClientConfig{
		APIToken: "test-token",
		BaseURL:  server.URL,
	})

	check, err := c.UpdateCheck(context.Background(), "201205050153W2Q4C-0J2HSIRF", CheckUpdateRequest{
		CheckCreateRequest: CheckCreateRequest{
			Label: "Updated Check",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if check.Label != "Updated Check" {
		t.Errorf("expected label 'Updated Check', got %q", check.Label)
	}
}

func TestDeleteCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(DeleteResponse{OK: true, ID: "201205050153W2Q4C-0J2HSIRF"})
	}))
	defer server.Close()

	c := NewClient(ClientConfig{
		APIToken: "test-token",
		BaseURL:  server.URL,
	})

	err := c.DeleteCheck(context.Background(), "201205050153W2Q4C-0J2HSIRF")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Finding 1: delete must be idempotent.
// ---------------------------------------------------------------------------

func TestDeleteCheckNotFoundReturnsNotFoundError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "Check not found"})
	}))
	defer server.Close()

	c := NewClient(ClientConfig{APIToken: "test-token", BaseURL: server.URL})

	err := c.DeleteCheck(context.Background(), "gone")
	if err == nil {
		t.Fatal("expected an error")
	}

	// The caller's not-found branch is a bare type assertion, so the concrete
	// type has to be *NotFoundError, not a wrapped *APIError.
	if notFound := (*NotFoundError)(nil); !errors.As(err, &notFound) {
		t.Fatalf("expected *NotFoundError, got %T (%v)", err, err)
	}

	var nfErr *NotFoundError
	if !errors.As(err, &nfErr) {
		t.Fatal("expected errors.As to match *NotFoundError")
	}
	if nfErr.ResourceType != "check" || nfErr.ResourceID != "gone" {
		t.Errorf("unexpected NotFoundError %+v", nfErr)
	}
}

func TestDeleteCheckOKFalseIsNotAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(DeleteResponse{OK: false})
	}))
	defer server.Close()

	c := NewClient(ClientConfig{APIToken: "test-token", BaseURL: server.URL})

	// NodePing answers ok:false when the resource was already gone. Deleting an
	// absent resource is a success.
	if err := c.DeleteCheck(context.Background(), "already-gone"); err != nil {
		t.Fatalf("ok:false on delete should not be an error, got %v", err)
	}
}

func TestDeleteCheckServerErrorStillFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "boom"})
	}))
	defer server.Close()

	c := NewClient(ClientConfig{
		APIToken:     "test-token",
		BaseURL:      server.URL,
		MaxRetries:   1,
		RetryMinWait: time.Millisecond,
		RetryMaxWait: time.Millisecond,
		RateLimit:    1000,
	})

	err := c.DeleteCheck(context.Background(), "abc")
	if err == nil {
		t.Fatal("expected an error for a 500 on delete")
	}
	if notFound := (*NotFoundError)(nil); errors.As(err, &notFound) {
		t.Fatal("a 500 must not be reported as not-found")
	}
}

func TestGetCheckNotFoundUsesErrorsAs(t *testing.T) {
	// Guards against a regression to err.(*APIError), which breaks the moment
	// doRequest wraps its error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "Check not found"})
	}))
	defer server.Close()

	c := NewClient(ClientConfig{APIToken: "test-token", BaseURL: server.URL})

	_, err := c.GetCheck(context.Background(), "nope")
	var nfErr *NotFoundError
	if !errors.As(err, &nfErr) {
		t.Fatalf("expected *NotFoundError, got %T (%v)", err, err)
	}
}

// NodePing returns "queue": false on every disabled check, where an enabled one
// carries a queue name. Decoding it as a string made every disabled check
// unreadable, so disabling a check broke the next refresh.
func TestGetCheck_DisabledCheckWithBooleanQueueDecodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"_id":"201205050153W2Q4C-0J2HSIRF","customer_id":"201205050153W2Q4C","label":"Disabled","type":"HTTPADV","enable":"inactive","queue":false,"parameters":{"target":"https://example.com","statuscode":403}}`))
	}))
	defer server.Close()

	c := NewClient(ClientConfig{APIToken: "test-token", BaseURL: server.URL})
	check, err := c.GetCheck(context.Background(), "201205050153W2Q4C-0J2HSIRF")
	if err != nil {
		t.Fatalf("should decode a disabled check when queue is a boolean: %v", err)
	}
	if check.Enabled != "inactive" {
		t.Errorf("enable = %q, want inactive", check.Enabled)
	}
}
