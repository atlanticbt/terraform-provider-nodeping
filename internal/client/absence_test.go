package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newAbsenceTestClient returns a client pointed at a server that answers every
// request with the supplied status and body.
func newAbsenceTestClient(t *testing.T, status int, body string) *Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	return NewClient(ClientConfig{APIToken: "test-token", BaseURL: server.URL})
}

// singleResourceGetter adapts every Get that returns one resource to a common
// shape so the absence table can drive all of them.
var singleResourceGetters = map[string]func(context.Context, *Client, string) error{
	"check": func(ctx context.Context, c *Client, id string) error {
		_, err := c.GetCheck(ctx, id)
		return err
	},
	"contact": func(ctx context.Context, c *Client, id string) error {
		_, err := c.GetContact(ctx, id)
		return err
	},
	"contact group": func(ctx context.Context, c *Client, id string) error {
		_, err := c.GetContactGroup(ctx, id)
		return err
	},
	"schedule": func(ctx context.Context, c *Client, id string) error {
		_, err := c.GetSchedule(ctx, id)
		return err
	},
	"notification profile": func(ctx context.Context, c *Client, id string) error {
		_, err := c.GetNotificationProfile(ctx, id)
		return err
	},
	"maintenance": func(ctx context.Context, c *Client, id string) error {
		_, err := c.GetMaintenance(ctx, id)
		return err
	},
	"account": func(ctx context.Context, c *Client, id string) error {
		_, err := c.GetAccount(ctx, id)
		return err
	},
}

func TestGetSingleResource_ReturnsNotFoundForEveryAbsenceSignal(t *testing.T) {
	signals := []struct {
		name   string
		status int
		body   string
	}{
		{"http 404", http.StatusNotFound, `{"error":"not found"}`},
		{"http 404 with no body", http.StatusNotFound, ``},
		{"empty object with http 200", http.StatusOK, `{}`},
		{"error body with http 200", http.StatusOK, `{"error":"check not found"}`},
		{"does-not-exist body with http 200", http.StatusOK, `{"error":"That id does not exist"}`},
		{"no-such body with http 200", http.StatusOK, `{"error":"no such object"}`},
		{"unknown-id body with http 200", http.StatusOK, `{"error":"Unknown ID supplied"}`},
		{"not-found body with http 400", http.StatusBadRequest, `{"error":"contact not found"}`},
	}

	for resourceType, get := range singleResourceGetters {
		for _, signal := range signals {
			t.Run(resourceType+"/"+signal.name, func(t *testing.T) {
				c := newAbsenceTestClient(t, signal.status, signal.body)

				err := get(context.Background(), c, "abc123")

				var notFound *NotFoundError
				if !errors.As(err, &notFound) {
					t.Fatalf("expected *NotFoundError, got %v", err)
				}
				if notFound.ResourceType != resourceType {
					t.Errorf("ResourceType = %q, want %q", notFound.ResourceType, resourceType)
				}
				if notFound.ResourceID != "abc123" {
					t.Errorf("ResourceID = %q, want %q", notFound.ResourceID, "abc123")
				}
			})
		}
	}
}

// An error message the client does not recognise must never be mistaken for a
// missing resource. Misclassifying it would make Terraform drop the resource
// from state and recreate live infrastructure on the next apply.
func TestGetSingleResource_UnrecognisedErrorBodyIsNotNotFound(t *testing.T) {
	messages := []string{
		"rate limit exceeded",
		"account suspended",
		"permission denied",
		"internal server error",
	}

	for resourceType, get := range singleResourceGetters {
		for _, message := range messages {
			t.Run(resourceType+"/"+message, func(t *testing.T) {
				c := newAbsenceTestClient(t, http.StatusOK, `{"error":"`+message+`"}`)

				err := get(context.Background(), c, "abc123")
				if err == nil {
					t.Fatal("expected an error")
				}

				var notFound *NotFoundError
				if errors.As(err, &notFound) {
					t.Fatalf("%q was misclassified as not-found", message)
				}

				var apiErr *APIError
				if !errors.As(err, &apiErr) {
					t.Fatalf("expected *APIError, got %T: %v", err, err)
				}
				if apiErr.Message != message {
					t.Errorf("Message = %q, want %q", apiErr.Message, message)
				}
				if apiErr.StatusCode != http.StatusOK {
					t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusOK)
				}
			})
		}
	}
}

// A retryable status must keep its meaning even when the body happens to say
// "not found" — a transient upstream failure is not evidence of absence.
func TestAPIError_RetryableStatusIsNeverNotFound(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway} {
		err := &APIError{StatusCode: status, Message: "upstream service not found"}
		if err.IsNotFound() {
			t.Errorf("status %d with a not-found message must not classify as not-found", status)
		}
	}
}

func TestErrorMessageFromBody(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    string
		wantOK  bool
		wantLen int
	}{
		{name: "empty body", body: ``},
		{name: "not json", body: `<html>oops</html>`},
		{name: "json array", body: `[1,2,3]`},
		{name: "object without error key", body: `{"_id":"abc"}`},
		{name: "error key is not a string", body: `{"error":{"code":7}}`},
		{name: "empty error string", body: `{"error":""}`},
		{name: "error string", body: `{"error":"nope"}`, want: "nope", wantOK: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := errorMessageFromBody([]byte(tt.body))
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("message = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestErrorMessageFromBody_TruncatesOversizedMessages(t *testing.T) {
	long := strings.Repeat("a", maxErrorMessageLength+50)

	got, ok := errorMessageFromBody([]byte(`{"error":"` + long + `"}`))
	if !ok {
		t.Fatal("expected an error message")
	}
	if !strings.HasSuffix(got, "... (truncated)") {
		t.Errorf("expected a truncated message, got %d characters", len(got))
	}
}

// An error carried by an HTTP 200 must not be replayed. It is a decision the
// server already made, not a transient failure.
func TestErrorInSuccessfulResponse_IsNotRetried(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"error":"account suspended"}`)); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	c := NewClient(ClientConfig{APIToken: "test-token", BaseURL: server.URL})

	if _, err := c.GetCheck(context.Background(), "abc123"); err == nil {
		t.Fatal("expected an error")
	}
	if attempts != 1 {
		t.Errorf("made %d attempts, want 1", attempts)
	}
}

// The 200-with-an-error-body case reaches the collection endpoints too, which
// would otherwise decode into an empty map and look like an empty account.
func TestListEndpoints_SurfaceErrorsCarriedByHTTP200(t *testing.T) {
	c := newAbsenceTestClient(t, http.StatusOK, `{"error":"account suspended"}`)
	ctx := context.Background()

	lists := map[string]func() error{
		"checks":               func() error { _, err := c.ListChecks(ctx); return err },
		"contacts":             func() error { _, err := c.ListContacts(ctx); return err },
		"contactgroups":        func() error { _, err := c.ListContactGroups(ctx); return err },
		"schedules":            func() error { _, err := c.ListSchedules(ctx); return err },
		"notificationprofiles": func() error { _, err := c.ListNotificationProfiles(ctx); return err },
		"maintenance":          func() error { _, err := c.ListMaintenance(ctx); return err },
		"accounts":             func() error { _, err := c.ListAccounts(ctx); return err },
	}

	for name, list := range lists {
		t.Run(name, func(t *testing.T) {
			err := list()
			if err == nil {
				t.Fatal("expected an error")
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("expected *APIError, got %T: %v", err, err)
			}
			if apiErr.Message != "account suspended" {
				t.Errorf("Message = %q, want %q", apiErr.Message, "account suspended")
			}
		})
	}
}

// Deletes stay idempotent: an already-absent resource is reported as a
// *NotFoundError, which callers treat as success.
func TestDelete_ReportsNotFoundForEveryAbsenceSignal(t *testing.T) {
	deleters := map[string]func(*Client, context.Context, string) error{
		"check":                (*Client).DeleteCheck,
		"contact":              (*Client).DeleteContact,
		"contact group":        (*Client).DeleteContactGroup,
		"schedule":             (*Client).DeleteSchedule,
		"notification profile": (*Client).DeleteNotificationProfile,
		"maintenance":          (*Client).DeleteMaintenance,
		"account":              (*Client).DeleteAccount,
	}

	signals := []struct {
		name   string
		status int
		body   string
	}{
		{"http 404", http.StatusNotFound, `{"error":"not found"}`},
		{"error body with http 200", http.StatusOK, `{"error":"check not found"}`},
	}

	for resourceType, del := range deleters {
		for _, signal := range signals {
			t.Run(resourceType+"/"+signal.name, func(t *testing.T) {
				c := newAbsenceTestClient(t, signal.status, signal.body)

				err := del(c, context.Background(), "abc123")

				var notFound *NotFoundError
				if !errors.As(err, &notFound) {
					t.Fatalf("expected *NotFoundError, got %v", err)
				}
				if notFound.ResourceType != resourceType {
					t.Errorf("ResourceType = %q, want %q", notFound.ResourceType, resourceType)
				}
			})
		}
	}
}

// A delete that fails for a reason other than absence must stay a real error.
func TestDelete_UnrecognisedErrorBodyIsNotNotFound(t *testing.T) {
	c := newAbsenceTestClient(t, http.StatusOK, `{"error":"rate limit exceeded"}`)

	err := c.DeleteCheck(context.Background(), "abc123")
	if err == nil {
		t.Fatal("expected an error")
	}

	var notFound *NotFoundError
	if errors.As(err, &notFound) {
		t.Fatal("a rate limit must not be reported as a missing check")
	}
	if !strings.Contains(err.Error(), "failed to delete check") {
		t.Errorf("error lost its context: %v", err)
	}
}

// A sparse-but-real document must survive the emptiness predicates. A check
// identified only by its target, or a maintenance window identified only by its
// name, is present and must not be deleted from state.
func TestGet_SparseButRealDocumentsAreNotTreatedAsAbsent(t *testing.T) {
	t.Run("check with only a target", func(t *testing.T) {
		c := newAbsenceTestClient(t, http.StatusOK, `{"parameters":{"target":"https://example.com"}}`)

		check, err := c.GetCheck(context.Background(), "abc123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if check.Parameters.Target != "https://example.com" {
			t.Errorf("Target = %q", check.Parameters.Target)
		}
	})

	t.Run("maintenance with only a name", func(t *testing.T) {
		c := newAbsenceTestClient(t, http.StatusOK, `{"name":"Patch window"}`)

		window, err := c.GetMaintenance(context.Background(), "abc123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if window.Identifier() != "abc123" {
			t.Errorf("Identifier() = %q, want the requested id", window.Identifier())
		}
	})
}
