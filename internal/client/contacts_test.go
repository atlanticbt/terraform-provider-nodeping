package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestListContacts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/contacts" {
			t.Errorf("expected path /contacts, got %s", r.URL.Path)
		}

		contacts := map[string]Contact{
			"201205050153W2Q4C-BKPGH": {
				ID:         "201205050153W2Q4C-BKPGH",
				CustomerID: "201205050153W2Q4C",
				Name:       "Test Contact",
				CustRole:   "notify",
				Addresses: map[string]ContactAddress{
					"K5SP9CQP": {
						Address: "test@example.com",
						Type:    "email",
					},
				},
			},
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(contacts)
	}))
	defer server.Close()

	c := NewClient(ClientConfig{
		APIToken: "test-token",
		BaseURL:  server.URL,
	})

	contacts, err := c.ListContacts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(contacts) != 1 {
		t.Errorf("expected 1 contact, got %d", len(contacts))
	}

	contact, ok := contacts["201205050153W2Q4C-BKPGH"]
	if !ok {
		t.Fatal("expected contact not found")
	}

	if contact.Name != "Test Contact" {
		t.Errorf("expected name 'Test Contact', got %q", contact.Name)
	}
}

func TestGetContact(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}

		contact := Contact{
			ID:         "201205050153W2Q4C-BKPGH",
			CustomerID: "201205050153W2Q4C",
			Name:       "Test Contact",
			CustRole:   "notify",
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(contact)
	}))
	defer server.Close()

	c := NewClient(ClientConfig{
		APIToken: "test-token",
		BaseURL:  server.URL,
	})

	contact, err := c.GetContact(context.Background(), "201205050153W2Q4C-BKPGH")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if contact.ID != "201205050153W2Q4C-BKPGH" {
		t.Errorf("expected ID '201205050153W2Q4C-BKPGH', got %q", contact.ID)
	}
}

func TestGetContactNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "Contact not found"})
	}))
	defer server.Close()

	c := NewClient(ClientConfig{
		APIToken: "test-token",
		BaseURL:  server.URL,
	})

	_, err := c.GetContact(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	_, ok := err.(*NotFoundError)
	if !ok {
		t.Errorf("expected *NotFoundError, got %T", err)
	}
}

func TestCreateContact(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var req ContactCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if req.Name != "New Contact" {
			t.Errorf("expected name 'New Contact', got %q", req.Name)
		}

		contact := Contact{
			ID:         "201205050153W2Q4C-NEWID",
			CustomerID: "201205050153W2Q4C",
			Name:       req.Name,
			CustRole:   req.CustRole,
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(contact)
	}))
	defer server.Close()

	c := NewClient(ClientConfig{
		APIToken: "test-token",
		BaseURL:  server.URL,
	})

	contact, err := c.CreateContact(context.Background(), ContactCreateRequest{
		Name:     "New Contact",
		CustRole: "notify",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if contact.ID != "201205050153W2Q4C-NEWID" {
		t.Errorf("expected ID '201205050153W2Q4C-NEWID', got %q", contact.ID)
	}
}

func TestUpdateContact(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}

		contact := Contact{
			ID:         "201205050153W2Q4C-BKPGH",
			CustomerID: "201205050153W2Q4C",
			Name:       "Updated Contact",
			CustRole:   "edit",
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(contact)
	}))
	defer server.Close()

	c := NewClient(ClientConfig{
		APIToken: "test-token",
		BaseURL:  server.URL,
	})

	contact, err := c.UpdateContact(context.Background(), "201205050153W2Q4C-BKPGH", ContactUpdateRequest{
		Name:     "Updated Contact",
		CustRole: "edit",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if contact.Name != "Updated Contact" {
		t.Errorf("expected name 'Updated Contact', got %q", contact.Name)
	}
}

func TestDeleteContact(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(DeleteResponse{OK: true, ID: "201205050153W2Q4C-BKPGH"})
	}))
	defer server.Close()

	c := NewClient(ClientConfig{
		APIToken: "test-token",
		BaseURL:  server.URL,
	})

	err := c.DeleteContact(context.Background(), "201205050153W2Q4C-BKPGH")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Finding 1: delete must be idempotent.
// ---------------------------------------------------------------------------

func TestDeleteContactNotFoundReturnsNotFoundError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "Contact not found"})
	}))
	defer server.Close()

	c := NewClient(ClientConfig{APIToken: "test-token", BaseURL: server.URL})

	err := c.DeleteContact(context.Background(), "gone")
	if err == nil {
		t.Fatal("expected an error")
	}
	if _, ok := err.(*NotFoundError); !ok {
		t.Fatalf("expected *NotFoundError, got %T (%v)", err, err)
	}
}

func TestDeleteContactOKFalseIsNotAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(DeleteResponse{OK: false})
	}))
	defer server.Close()

	c := NewClient(ClientConfig{APIToken: "test-token", BaseURL: server.URL})

	if err := c.DeleteContact(context.Background(), "already-gone"); err != nil {
		t.Fatalf("ok:false on delete should not be an error, got %v", err)
	}
}

func TestGetContactNotFoundUsesErrorsAs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "Contact not found"})
	}))
	defer server.Close()

	c := NewClient(ClientConfig{APIToken: "test-token", BaseURL: server.URL})

	_, err := c.GetContact(context.Background(), "nope")
	var nfErr *NotFoundError
	if !errors.As(err, &nfErr) {
		t.Fatalf("expected *NotFoundError, got %T (%v)", err, err)
	}
}

func TestCreateContactNotRetriedOn500(t *testing.T) {
	var posts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&posts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewClient(ClientConfig{
		APIToken:     "test-token",
		BaseURL:      server.URL,
		MaxRetries:   3,
		RetryMinWait: time.Millisecond,
		RetryMaxWait: time.Millisecond,
		RateLimit:    1000,
	})

	if _, err := c.CreateContact(context.Background(), ContactCreateRequest{Name: "n"}); err == nil {
		t.Fatal("expected an error")
	}
	if got := atomic.LoadInt32(&posts); got != 1 {
		t.Fatalf("contact creation must not be replayed after a 5xx: got %d POSTs", got)
	}
}

func TestValidationErrorMessages(t *testing.T) {
	e := &ValidationError{Field: "id", Message: "must not be empty"}
	if got := e.Error(); got != `validation error for field "id": must not be empty` {
		t.Errorf("unexpected message %q", got)
	}
	e = &ValidationError{Message: "bad"}
	if got := e.Error(); got != "validation error: bad" {
		t.Errorf("unexpected message %q", got)
	}
}

func TestRateLimitErrorMessages(t *testing.T) {
	if got := (&RateLimitError{RetryAfter: 5}).Error(); got != "rate limit exceeded, retry after 5 seconds" {
		t.Errorf("unexpected message %q", got)
	}
	if got := (&RateLimitError{}).Error(); got != "rate limit exceeded" {
		t.Errorf("unexpected message %q", got)
	}
	var apiErr *APIError
	if !errors.As(&RateLimitError{}, &apiErr) || apiErr.StatusCode != 429 {
		t.Error("a bare RateLimitError should still unwrap to a 429 APIError")
	}
}

func TestAPIErrorWithRequestID(t *testing.T) {
	e := &APIError{StatusCode: 500, Message: "boom", RequestID: "req-1"}
	if got := e.Error(); got != "NodePing API error (status 500, request req-1): boom" {
		t.Errorf("unexpected message %q", got)
	}
}

func TestNotFoundErrorMessage(t *testing.T) {
	e := &NotFoundError{ResourceType: "check", ResourceID: "abc"}
	if got := e.Error(); got != `check with ID "abc" not found` {
		t.Errorf("unexpected message %q", got)
	}
}

func TestHelperConverters(t *testing.T) {
	if boolToString(true) != "true" || boolToString(false) != "false" {
		t.Error("boolToString is wrong")
	}
	if intToString(42) != "42" {
		t.Error("intToString is wrong")
	}
}

func TestGetDefaultTags(t *testing.T) {
	c := NewClient(ClientConfig{APIToken: "t", DefaultTags: []string{"a", "b"}})
	tags := c.GetDefaultTags()
	if len(tags) != 2 || tags[0] != "a" || tags[1] != "b" {
		t.Errorf("unexpected tags %v", tags)
	}
	if got := c.WithCustomerID("x").GetDefaultTags(); len(got) != 2 {
		t.Error("WithCustomerID should carry default tags forward")
	}
}
