package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func newProfileTestClient(t *testing.T, handler http.HandlerFunc) *Client {
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

func TestListNotificationProfilesDecodesObjectResponse(t *testing.T) {
	c := newProfileTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/notificationprofiles" {
			t.Errorf("unexpected path %s", req.URL.Path)
		}
		_, _ = w.Write([]byte(`{"ACC-P-1":{"_id":"ACC-P-1","name":"first"},"ACC-P-2":{"name":"second"}}`))
	})

	got, err := c.ListNotificationProfiles(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(got))
	}
	// A payload that omits _id must still carry a usable ID.
	if got["ACC-P-2"].ID != "ACC-P-2" {
		t.Fatalf("expected the map key to backfill the ID, got %q", got["ACC-P-2"].ID)
	}
}

func TestListNotificationProfilesDecodesArrayResponse(t *testing.T) {
	c := newProfileTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`[{"_id":"ACC-P-1","name":"first"},{"name":"unusable"}]`))
	})

	got, err := c.ListNotificationProfiles(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got["ACC-P-1"].Name != "first" {
		t.Fatalf("unexpected result: %#v", got)
	}
}

func TestDecodeNotificationProfileCollectionHandlesEmptyAndInvalidInput(t *testing.T) {
	got, err := decodeNotificationProfileCollection(nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("expected an empty map, got %#v (err=%v)", got, err)
	}

	got, err = decodeNotificationProfileCollection(json.RawMessage(`null`))
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("expected an empty map for null, got %#v (err=%v)", got, err)
	}

	if _, err := decodeNotificationProfileCollection(json.RawMessage(`"a string"`)); err == nil {
		t.Fatal("expected an error for an undecodable collection")
	}
}

func TestListNotificationProfilesReportsAPIError(t *testing.T) {
	c := newProfileTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"denied"}`))
	})

	if _, err := c.ListNotificationProfiles(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
}

func TestGetNotificationProfile(t *testing.T) {
	c := newProfileTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/notificationprofiles/ACC-P-1" {
			t.Errorf("unexpected path %s", req.URL.Path)
		}
		_, _ = w.Write([]byte(`{"_id":"ACC-P-1","type":"notification profile","customer_id":"SUB1","name":"Escalation","notifications":[{"A-1":{"delay":0,"schedule":"All"}}]}`))
	})

	got, err := c.GetNotificationProfile(context.Background(), "ACC-P-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "Escalation" || got.CustomerID != "SUB1" || len(got.Notifications) != 1 {
		t.Fatalf("unexpected profile: %#v", got)
	}
}

func TestGetNotificationProfileReportsNotFound(t *testing.T) {
	c := newProfileTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	})

	_, err := c.GetNotificationProfile(context.Background(), "ACC-P-MISSING")
	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected a *NotFoundError, got %v", err)
	}
}

func TestGetNotificationProfileTreatsEmptyBodyAsNotFound(t *testing.T) {
	c := newProfileTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})

	_, err := c.GetNotificationProfile(context.Background(), "ACC-P-1")
	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected a *NotFoundError, got %v", err)
	}
}

func TestGetNotificationProfileRejectsEmptyID(t *testing.T) {
	c := newProfileTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		t.Error("no request should be issued for an empty ID")
	})

	if _, err := c.GetNotificationProfile(context.Background(), "  "); err == nil {
		t.Fatal("expected a validation error")
	}
}

func TestCreateNotificationProfileAlwaysSendsAnArray(t *testing.T) {
	var body map[string]interface{}
	c := newProfileTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", req.Method)
		}
		raw, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}
		_, _ = w.Write([]byte(`{"_id":"ACC-P-1","name":"Escalation"}`))
	})

	got, err := c.CreateNotificationProfile(context.Background(), NotificationProfileCreateRequest{Name: "Escalation"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "ACC-P-1" {
		t.Fatalf("unexpected ID %q", got.ID)
	}
	if _, ok := body["notifications"].([]interface{}); !ok {
		t.Fatalf("notifications must be sent as an array, got %#v", body["notifications"])
	}
}

func TestCreateNotificationProfileReportsAPIError(t *testing.T) {
	c := newProfileTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"name is required"}`))
	})

	if _, err := c.CreateNotificationProfile(context.Background(), NotificationProfileCreateRequest{}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestUpdateNotificationProfileSendsIDAndFullArray(t *testing.T) {
	var body map[string]interface{}
	c := newProfileTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", req.Method)
		}
		raw, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}
		_, _ = w.Write([]byte(`{"_id":"ACC-P-1","name":"Escalation"}`))
	})

	notifications := []map[string]interface{}{
		{"A-1": map[string]interface{}{"delay": 0, "schedule": "All"}},
	}
	if _, err := c.UpdateNotificationProfile(context.Background(), "ACC-P-1", NotificationProfileUpdateRequest{
		Name:          "Escalation",
		Notifications: notifications,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if body["id"] != "ACC-P-1" {
		t.Fatalf("expected the id in the PUT body, got %v", body["id"])
	}
	got, ok := body["notifications"].([]interface{})
	if !ok || len(got) != 1 {
		t.Fatalf("expected the complete notifications array, got %#v", body["notifications"])
	}
}

func TestUpdateNotificationProfileReportsNotFound(t *testing.T) {
	c := newProfileTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	})

	_, err := c.UpdateNotificationProfile(context.Background(), "ACC-P-1", NotificationProfileUpdateRequest{Name: "x"})
	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected a *NotFoundError, got %v", err)
	}
}

func TestUpdateNotificationProfileRejectsEmptyID(t *testing.T) {
	c := newProfileTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		t.Error("no request should be issued for an empty ID")
	})

	if _, err := c.UpdateNotificationProfile(context.Background(), "", NotificationProfileUpdateRequest{}); err == nil {
		t.Fatal("expected a validation error")
	}
}

func TestDeleteNotificationProfile(t *testing.T) {
	c := newProfileTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", req.Method)
		}
		_, _ = w.Write([]byte(`{"ok":true,"id":"ACC-P-1"}`))
	})

	if err := c.DeleteNotificationProfile(context.Background(), "ACC-P-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteNotificationProfileReportsNotFound(t *testing.T) {
	c := newProfileTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	})

	err := c.DeleteNotificationProfile(context.Background(), "ACC-P-1")
	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected a *NotFoundError, got %v", err)
	}
}

func TestDeleteNotificationProfileReportsOtherErrors(t *testing.T) {
	c := newProfileTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"denied"}`))
	})

	err := c.DeleteNotificationProfile(context.Background(), "ACC-P-1")
	var notFound *NotFoundError
	if err == nil || errors.As(err, &notFound) {
		t.Fatalf("expected a plain error, got %v", err)
	}
}

func TestDeleteNotificationProfileRejectsEmptyID(t *testing.T) {
	c := newProfileTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		t.Error("no request should be issued for an empty ID")
	})

	if err := c.DeleteNotificationProfile(context.Background(), ""); err == nil {
		t.Fatal("expected a validation error")
	}
}

func TestNotificationProfileRequestsCarryTheCustomerScope(t *testing.T) {
	var query string
	c := newProfileTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		query = req.URL.Query().Get("customerid")
		_, _ = w.Write([]byte(`{"_id":"ACC-P-1","name":"Escalation"}`))
	})

	if _, err := c.WithCustomerID("SUB1").GetNotificationProfile(context.Background(), "ACC-P-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if query != "SUB1" {
		t.Fatalf("expected customerid=SUB1, got %q", query)
	}
}

func TestNotificationsOrEmpty(t *testing.T) {
	if got := notificationsOrEmpty(nil); got == nil || len(got) != 0 {
		t.Fatalf("expected an empty non-nil slice, got %#v", got)
	}
	in := []map[string]interface{}{{"A-1": map[string]interface{}{}}}
	if got := notificationsOrEmpty(in); !reflect.DeepEqual(got, in) {
		t.Fatalf("expected the input unchanged, got %#v", got)
	}
}
