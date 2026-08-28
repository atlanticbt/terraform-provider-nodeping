package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newContactGroupTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return NewClient(ClientConfig{
		APIToken:     "test-token",
		BaseURL:      srv.URL,
		RateLimit:    1000,
		MaxRetries:   1,
		RetryMinWait: time.Millisecond,
		RetryMaxWait: time.Millisecond,
		Timeout:      5 * time.Second,
	})
}

func TestListContactGroups_ReturnsGroupsKeyedByID(t *testing.T) {
	c := newContactGroupTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/contactgroups" {
			t.Errorf("expected path /contactgroups, got %s", r.URL.Path)
		}

		if err := json.NewEncoder(w).Encode(map[string]ContactGroup{
			"201205050153W2Q4C-G-ISYB7": {
				ID:         "201205050153W2Q4C-G-ISYB7",
				Type:       "group",
				CustomerID: "201205050153W2Q4C",
				Name:       "Production Alerts",
				Members:    []string{"1664V", "K5SP9CQP"},
			},
		}); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	})

	groups, err := c.ListContactGroups(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}

	group, ok := groups["201205050153W2Q4C-G-ISYB7"]
	if !ok {
		t.Fatal("expected group not present in result")
	}
	if group.Name != "Production Alerts" {
		t.Errorf("expected name %q, got %q", "Production Alerts", group.Name)
	}
	if len(group.Members) != 2 || group.Members[0] != "1664V" {
		t.Errorf("unexpected members: %v", group.Members)
	}
}

func TestListContactGroups_WrapsAPIError(t *testing.T) {
	c := newContactGroupTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		if _, err := io.WriteString(w, `{"error":"bad request"}`); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	})

	if _, err := c.ListContactGroups(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
}

func TestListContactGroups_ScopesToCustomerID(t *testing.T) {
	var seen string
	c := newContactGroupTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.Query().Get("customerid")
		if _, err := io.WriteString(w, `{}`); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	})

	if _, err := c.WithCustomerID("SUB123").ListContactGroups(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seen != "SUB123" {
		t.Errorf("expected customerid=SUB123, got %q", seen)
	}
}

func TestGetContactGroup_ReturnsGroup(t *testing.T) {
	c := newContactGroupTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/contactgroups/201205050153W2Q4C-G-ISYB7" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if err := json.NewEncoder(w).Encode(ContactGroup{
			ID:         "201205050153W2Q4C-G-ISYB7",
			Type:       "group",
			CustomerID: "201205050153W2Q4C",
			Name:       "Production Alerts",
			Members:    []string{"K5SP9CQP"},
		}); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	})

	group, err := c.GetContactGroup(context.Background(), "201205050153W2Q4C-G-ISYB7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if group.ID != "201205050153W2Q4C-G-ISYB7" {
		t.Errorf("unexpected id %q", group.ID)
	}
}

func TestGetContactGroup_EscapesIDInPath(t *testing.T) {
	var path string
	c := newContactGroupTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.EscapedPath()
		w.WriteHeader(http.StatusNotFound)
	})

	_, err := c.GetContactGroup(context.Background(), "a b/../c")
	if err == nil {
		t.Fatal("expected an error")
	}
	if path != "/contactgroups/a%20b%2F..%2Fc" {
		t.Errorf("expected the ID to be path-escaped, got %q", path)
	}
}

func TestGetContactGroup_NotFoundReturnsNotFoundError(t *testing.T) {
	c := newContactGroupTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		if _, err := io.WriteString(w, `{"error":"not found"}`); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	})

	_, err := c.GetContactGroup(context.Background(), "MISSING-G-1")

	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected *NotFoundError, got %v", err)
	}
	if notFound.ResourceType != "contact group" {
		t.Errorf("unexpected resource type %q", notFound.ResourceType)
	}
}

func TestGetContactGroup_EmptyBodyIsTreatedAsNotFound(t *testing.T) {
	c := newContactGroupTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.WriteString(w, `{}`); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	})

	_, err := c.GetContactGroup(context.Background(), "GONE-G-1")

	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected *NotFoundError for an empty 200 body, got %v", err)
	}
}

func TestGetContactGroup_ServerErrorIsWrapped(t *testing.T) {
	c := newContactGroupTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := c.GetContactGroup(context.Background(), "G1")
	if err == nil {
		t.Fatal("expected an error")
	}
	var notFound *NotFoundError
	if errors.As(err, &notFound) {
		t.Fatal("a 500 must not be reported as not-found")
	}
}

func TestGetContactGroup_RejectsEmptyID(t *testing.T) {
	c := newContactGroupTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request should be sent for an empty ID")
	})

	_, err := c.GetContactGroup(context.Background(), "   ")

	var valErr *ValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("expected *ValidationError, got %v", err)
	}
}

func TestCreateContactGroup_SendsNameAndMembers(t *testing.T) {
	var body ContactGroupCreateRequest
	c := newContactGroupTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		if err := json.NewEncoder(w).Encode(ContactGroup{
			ID:         "ACC-G-ISYB7",
			Type:       "group",
			CustomerID: "ACC",
			Name:       body.Name,
			Members:    body.Members,
		}); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	})

	group, err := c.CreateContactGroup(context.Background(), ContactGroupCreateRequest{
		Name:    "Alerts",
		Members: []string{"1664V", "K5SP9CQP"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if group.ID != "ACC-G-ISYB7" {
		t.Errorf("unexpected id %q", group.ID)
	}
	if body.Name != "Alerts" || len(body.Members) != 2 {
		t.Errorf("unexpected request body %+v", body)
	}
}

func TestCreateContactGroup_SendsEmptyArrayForNilMembers(t *testing.T) {
	var raw map[string]json.RawMessage
	c := newContactGroupTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		if err := json.NewEncoder(w).Encode(ContactGroup{ID: "ACC-G-1"}); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	})

	if _, err := c.CreateContactGroup(context.Background(), ContactGroupCreateRequest{Name: "Empty"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := string(raw["members"]); got != "[]" {
		t.Errorf("expected members to be sent as [], got %s", got)
	}
}

func TestCreateContactGroup_WrapsError(t *testing.T) {
	c := newContactGroupTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})

	if _, err := c.CreateContactGroup(context.Background(), ContactGroupCreateRequest{}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestUpdateContactGroup_SendsIDInBodyAndPath(t *testing.T) {
	var body ContactGroupUpdateRequest
	var path string
	c := newContactGroupTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		path = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		if err := json.NewEncoder(w).Encode(ContactGroup{
			ID:      body.ID,
			Name:    body.Name,
			Members: body.Members,
		}); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	})

	group, err := c.UpdateContactGroup(context.Background(), "ACC-G-ISYB7", ContactGroupUpdateRequest{
		Name:    "Renamed",
		Members: []string{"K5SP9CQP"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if path != "/contactgroups/ACC-G-ISYB7" {
		t.Errorf("unexpected path %q", path)
	}
	if body.ID != "ACC-G-ISYB7" {
		t.Errorf("expected the group ID in the body, got %q", body.ID)
	}
	if group.Name != "Renamed" {
		t.Errorf("unexpected name %q", group.Name)
	}
}

func TestUpdateContactGroup_ClearingMembersSendsEmptyArray(t *testing.T) {
	var raw map[string]json.RawMessage
	c := newContactGroupTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		if err := json.NewEncoder(w).Encode(ContactGroup{ID: "ACC-G-1"}); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	})

	if _, err := c.UpdateContactGroup(context.Background(), "ACC-G-1", ContactGroupUpdateRequest{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := string(raw["members"]); got != "[]" {
		t.Errorf("expected members to be sent as [], got %s", got)
	}
}

func TestUpdateContactGroup_NotFoundReturnsNotFoundError(t *testing.T) {
	c := newContactGroupTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_, err := c.UpdateContactGroup(context.Background(), "GONE-G-1", ContactGroupUpdateRequest{})

	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected *NotFoundError, got %v", err)
	}
}

func TestUpdateContactGroup_RejectsEmptyID(t *testing.T) {
	c := newContactGroupTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request should be sent for an empty ID")
	})

	_, err := c.UpdateContactGroup(context.Background(), "", ContactGroupUpdateRequest{})

	var valErr *ValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("expected *ValidationError, got %v", err)
	}
}

func TestDeleteContactGroup_Succeeds(t *testing.T) {
	c := newContactGroupTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if err := json.NewEncoder(w).Encode(DeleteResponse{OK: true, ID: "ACC-G-1"}); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	})

	if err := c.DeleteContactGroup(context.Background(), "ACC-G-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteContactGroup_NotFoundReturnsNotFoundError(t *testing.T) {
	c := newContactGroupTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	err := c.DeleteContactGroup(context.Background(), "GONE-G-1")

	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected *NotFoundError, got %v", err)
	}
}

func TestDeleteContactGroup_ServerErrorIsWrapped(t *testing.T) {
	c := newContactGroupTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	err := c.DeleteContactGroup(context.Background(), "ACC-G-1")
	if err == nil {
		t.Fatal("expected an error")
	}
	var notFound *NotFoundError
	if errors.As(err, &notFound) {
		t.Fatal("a 500 must not be reported as not-found")
	}
}

func TestDeleteContactGroup_RejectsEmptyID(t *testing.T) {
	c := newContactGroupTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request should be sent for an empty ID")
	})

	err := c.DeleteContactGroup(context.Background(), "\n")

	var valErr *ValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("expected *ValidationError, got %v", err)
	}
}

func TestNormalizeMembers(t *testing.T) {
	if got := normalizeMembers(nil); got == nil || len(got) != 0 {
		t.Errorf("expected an empty non-nil slice, got %#v", got)
	}
	if got := normalizeMembers([]string{"A"}); len(got) != 1 || got[0] != "A" {
		t.Errorf("expected the input to be preserved, got %#v", got)
	}
}
