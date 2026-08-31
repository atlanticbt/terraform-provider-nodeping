package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newScheduleTestClient(t *testing.T, handler http.HandlerFunc) *Client {
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

func businessHours() map[string]ScheduleDay {
	return map[string]ScheduleDay{
		"monday": {Time1: "8:00", Time2: "18:00"},
		"friday": {AllDay: true},
	}
}

// ---------------------------------------------------------------------------
// ScheduleFlexBool
// ---------------------------------------------------------------------------

func TestScheduleFlexBool_DecodesEveryEncodingNodePingUses(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"json true", `true`, true},
		{"json false", `false`, false},
		{"number one", `1`, true},
		{"number zero", `0`, false},
		{"string one", `"1"`, true},
		{"string zero", `"0"`, false},
		{"string false", `"false"`, false},
		{"string empty", `""`, false},
		{"string yes", `"yes"`, true},
		{"null", `null`, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got ScheduleFlexBool
			if err := json.Unmarshal([]byte(tc.input), &got); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Bool() != tc.want {
				t.Fatalf("input %s: got %v, want %v", tc.input, got.Bool(), tc.want)
			}
		})
	}
}

func TestScheduleFlexBool_RejectsUnsupportedShapes(t *testing.T) {
	var got ScheduleFlexBool
	if err := json.Unmarshal([]byte(`{"nested": true}`), &got); err == nil {
		t.Fatal("expected an error for an object value")
	}
	if err := json.Unmarshal([]byte(`not json`), &got); err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}

func TestScheduleFlexBool_EncodesAsZeroOrOne(t *testing.T) {
	on, err := json.Marshal(ScheduleFlexBool(true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(on) != "1" {
		t.Fatalf("true encoded as %s, want 1", on)
	}

	off, err := json.Marshal(ScheduleFlexBool(false))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(off) != "0" {
		t.Fatalf("false encoded as %s, want 0", off)
	}
}

// ---------------------------------------------------------------------------
// Schedule decoding
// ---------------------------------------------------------------------------

func TestScheduleUnmarshal_AcceptsDocumentedShape(t *testing.T) {
	var s Schedule
	body := `{"id":"Days","customer_id":"CUST1","data":{"monday":{"time1":"6:00","time2":"18:00","exclude":1}}}`
	if err := json.Unmarshal([]byte(body), &s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if s.ID != "Days" || s.CustomerID != "CUST1" {
		t.Fatalf("got id=%q customer=%q", s.ID, s.CustomerID)
	}
	monday := s.Data["monday"]
	if monday.Time1 != "6:00" || monday.Time2 != "18:00" || !monday.Exclude.Bool() {
		t.Fatalf("monday decoded as %+v", monday)
	}
}

func TestScheduleUnmarshal_AcceptsMongoStyleIDAndBareDayMap(t *testing.T) {
	var s Schedule
	body := `{"_id":"Days","customerid":"CUST2","Monday":{"allday":true}}`
	if err := json.Unmarshal([]byte(body), &s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if s.ID != "Days" || s.CustomerID != "CUST2" {
		t.Fatalf("got id=%q customer=%q", s.ID, s.CustomerID)
	}
	if !s.Data["monday"].AllDay.Bool() {
		t.Fatalf("expected monday allday, got %+v", s.Data)
	}
}

func TestScheduleUnmarshal_DropsKeysThatAreNotDayNames(t *testing.T) {
	var s Schedule
	body := `{"id":"Days","data":{"monday":{"allday":true},"monady":{"allday":true}}}`
	if err := json.Unmarshal([]byte(body), &s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s.Data) != 1 {
		t.Fatalf("expected only monday to survive, got %+v", s.Data)
	}
}

func TestScheduleUnmarshal_ReportsMalformedPayloads(t *testing.T) {
	var s Schedule
	if err := json.Unmarshal([]byte(`["not","an","object"]`), &s); err == nil {
		t.Fatal("expected an error for a non-object payload")
	}
	if err := json.Unmarshal([]byte(`{"id":"Days","data":"nope"}`), &s); err == nil {
		t.Fatal("expected an error for a non-object data field")
	}
	if err := json.Unmarshal([]byte(`{"monday":"nope"}`), &s); err == nil {
		t.Fatal("expected an error for a non-object day")
	}
}

// ---------------------------------------------------------------------------
// validation
// ---------------------------------------------------------------------------

func TestValidateScheduleID_RejectsUnsafeIdentifiers(t *testing.T) {
	cases := map[string]string{
		"empty":         "",
		"whitespace":    "   ",
		"dot":           ".",
		"dotdot":        "..",
		"newline":       "Days\nEvil",
		"slash":         "Days/Evil",
		"question mark": "Days?x=1",
		"fragment":      "Days#frag",
		"too long":      strings.Repeat("a", maxScheduleIDLength+1),
	}

	for name, id := range cases {
		t.Run(name, func(t *testing.T) {
			err := validateScheduleID(id)
			if err == nil {
				t.Fatalf("expected a validation error for %q", id)
			}
			var validationErr *ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("expected *ValidationError, got %T", err)
			}
		})
	}
}

func TestValidateScheduleID_AcceptsOrdinaryNames(t *testing.T) {
	for _, id := range []string{"Days", "BusinessHours", "on call - weekend", strings.Repeat("a", maxScheduleIDLength)} {
		if err := validateScheduleID(id); err != nil {
			t.Fatalf("id %q rejected: %v", id, err)
		}
	}
}

func TestValidateScheduleData_RejectsEmptyUnknownDaysAndBadTimes(t *testing.T) {
	if err := validateScheduleData(nil); err == nil {
		t.Fatal("expected an error for an empty schedule")
	}

	err := validateScheduleData(map[string]ScheduleDay{"monady": {AllDay: true}})
	if err == nil || !strings.Contains(err.Error(), "monady") {
		t.Fatalf("expected an unknown-day error, got %v", err)
	}

	for _, bad := range []string{"25:00", "6:0", "0600", "6:60", "-1:00", "six"} {
		err := validateScheduleData(map[string]ScheduleDay{"monday": {Time1: bad}})
		if err == nil {
			t.Fatalf("expected %q to be rejected", bad)
		}
	}

	err = validateScheduleData(map[string]ScheduleDay{"monday": {Time1: "8:00", Time2: "99:99"}})
	if err == nil || !strings.Contains(err.Error(), "time2") {
		t.Fatalf("expected a time2 error, got %v", err)
	}
}

func TestValidateScheduleData_AcceptsValidSpans(t *testing.T) {
	data := map[string]ScheduleDay{
		"monday": {Time1: "0:00", Time2: "23:59"},
		"sunday": {Time1: "6:00", Time2: "18:30", Exclude: true},
	}
	if err := validateScheduleData(data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ListSchedules
// ---------------------------------------------------------------------------

func TestListSchedules_FillsIDFromMapKey(t *testing.T) {
	c := newScheduleTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/schedules" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if _, err := io.WriteString(w, `{"Days":{"data":{"monday":{"allday":true}}},"Nights":{"id":"Nights","data":{"sunday":{"disabled":1}}}}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})

	got, err := c.ListSchedules(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["Days"].ID != "Days" {
		t.Fatalf("expected the map key to become the ID, got %q", got["Days"].ID)
	}
	if !got["Nights"].Data["sunday"].Disabled.Bool() {
		t.Fatalf("sunday should be disabled, got %+v", got["Nights"].Data)
	}
}

func TestListSchedules_ReturnsEmptyMapForEmptyAccount(t *testing.T) {
	c := newScheduleTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	got, err := c.ListSchedules(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("expected an empty map, got %v", got)
	}
}

func TestListSchedules_WrapsAPIErrors(t *testing.T) {
	c := newScheduleTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		if _, err := io.WriteString(w, `{"error":"denied"}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})

	if _, err := c.ListSchedules(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
}

// ---------------------------------------------------------------------------
// GetSchedule
// ---------------------------------------------------------------------------

func TestGetSchedule_ReturnsSchedule(t *testing.T) {
	c := newScheduleTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/schedules/Business%20Hours" {
			t.Errorf("unexpected path %q", r.URL.EscapedPath())
		}
		if _, err := io.WriteString(w, `{"data":{"monday":{"time1":"8:00","time2":"18:00"}}}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})

	got, err := c.GetSchedule(context.Background(), "Business Hours")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "Business Hours" {
		t.Fatalf("expected the requested ID to be filled in, got %q", got.ID)
	}
	if got.Data["monday"].Time2 != "18:00" {
		t.Fatalf("unexpected data %+v", got.Data)
	}
}

func TestGetSchedule_ValidatesIDBeforeCallingTheAPI(t *testing.T) {
	c := newScheduleTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("the API must not be called with an invalid ID")
	})

	if _, err := c.GetSchedule(context.Background(), ""); err == nil {
		t.Fatal("expected a validation error")
	}
}

func TestGetSchedule_ReportsNotFoundForEveryAbsentSignal(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"http 404", http.StatusNotFound, `{"error":"not found"}`},
		{"empty body", http.StatusOK, ``},
		{"error body with 200", http.StatusOK, `{"error":"Schedule not found"}`},
		{"no days", http.StatusOK, `{"id":"Days"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newScheduleTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				if tc.body != "" {
					if _, err := io.WriteString(w, tc.body); err != nil {
						t.Errorf("write failed: %v", err)
					}
				}
			})

			_, err := c.GetSchedule(context.Background(), "Days")
			var notFound *NotFoundError
			if !errors.As(err, &notFound) {
				t.Fatalf("expected *NotFoundError, got %v", err)
			}
		})
	}
}

func TestGetSchedule_SurfacesNonNotFoundErrorBodies(t *testing.T) {
	c := newScheduleTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.WriteString(w, `{"error":"account suspended"}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})

	_, err := c.GetSchedule(context.Background(), "Days")
	if err == nil || !strings.Contains(err.Error(), "account suspended") {
		t.Fatalf("expected the API message to surface, got %v", err)
	}
	var notFound *NotFoundError
	if errors.As(err, &notFound) {
		t.Fatal("an operational error must not be reported as not-found")
	}
}

func TestGetSchedule_ReportsMalformedResponses(t *testing.T) {
	c := newScheduleTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.WriteString(w, `["unexpected"]`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})

	if _, err := c.GetSchedule(context.Background(), "Days"); err == nil {
		t.Fatal("expected a decode error")
	}
}

// ---------------------------------------------------------------------------
// PutSchedule
// ---------------------------------------------------------------------------

func TestPutSchedule_SendsPutWithBooleansAsZeroOrOne(t *testing.T) {
	var body map[string]interface{}
	var method, path, query string

	c := newScheduleTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		query = r.URL.RawQuery
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if _, err := io.WriteString(w, `{"id":"Days","data":{"monday":{"time1":"8:00","time2":"18:00","exclude":1}}}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})

	got, err := c.WithCustomerID("CUST1").PutSchedule(context.Background(), "Days", SchedulePutRequest{
		Data: map[string]ScheduleDay{"monday": {Time1: "8:00", Time2: "18:00", Exclude: true}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if method != http.MethodPut {
		t.Fatalf("expected PUT, got %s", method)
	}
	if path != "/schedules/Days" {
		t.Fatalf("unexpected path %q", path)
	}
	if !strings.Contains(query, "customerid=CUST1") {
		t.Fatalf("expected the customer ID in the query, got %q", query)
	}
	if body["id"] != "Days" {
		t.Fatalf("expected the path ID in the body, got %v", body["id"])
	}

	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data missing from body: %v", body)
	}
	monday, ok := data["monday"].(map[string]interface{})
	if !ok {
		t.Fatalf("monday missing from data: %v", data)
	}
	if monday["exclude"] != float64(1) || monday["disabled"] != float64(0) || monday["allday"] != float64(0) {
		t.Fatalf("booleans must be sent as 0/1, got %v", monday)
	}
	if !got.Data["monday"].Exclude.Bool() {
		t.Fatalf("expected the response to be decoded, got %+v", got.Data)
	}
}

func TestPutSchedule_OverridesAMismatchedBodyID(t *testing.T) {
	var body map[string]interface{}
	c := newScheduleTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if _, err := io.WriteString(w, `{"ok":true}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})

	got, err := c.PutSchedule(context.Background(), "Days", SchedulePutRequest{
		ID:   "SomethingElse",
		Data: businessHours(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body["id"] != "Days" {
		t.Fatalf("the path ID must win, got %v", body["id"])
	}
	// A bare acknowledgement carries no data; the caller keeps its own values.
	if got.ID != "Days" || len(got.Data) != 0 {
		t.Fatalf("unexpected schedule %+v", got)
	}
}

func TestPutSchedule_RejectsInvalidInputBeforeCallingTheAPI(t *testing.T) {
	c := newScheduleTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("the API must not be called with invalid input")
	})

	if _, err := c.PutSchedule(context.Background(), "", SchedulePutRequest{Data: businessHours()}); err == nil {
		t.Fatal("expected an ID validation error")
	}
	if _, err := c.PutSchedule(context.Background(), "Days", SchedulePutRequest{}); err == nil {
		t.Fatal("expected a data validation error")
	}
	if _, err := c.PutSchedule(context.Background(), "Days", SchedulePutRequest{
		Data: map[string]ScheduleDay{"monday": {Time1: "99:99"}},
	}); err == nil {
		t.Fatal("expected a time validation error")
	}
}

func TestPutSchedule_SurfacesErrorBodiesAndTransportFailures(t *testing.T) {
	errBody := newScheduleTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.WriteString(w, `{"error":"quota exceeded"}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})
	_, err := errBody.PutSchedule(context.Background(), "Days", SchedulePutRequest{Data: businessHours()})
	if err == nil || !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("expected the API message to surface, got %v", err)
	}

	badStatus := newScheduleTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	if _, err := badStatus.PutSchedule(context.Background(), "Days", SchedulePutRequest{Data: businessHours()}); err == nil {
		t.Fatal("expected an error for HTTP 400")
	}

	badJSON := newScheduleTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.WriteString(w, `["nope"]`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})
	if _, err := badJSON.PutSchedule(context.Background(), "Days", SchedulePutRequest{Data: businessHours()}); err == nil {
		t.Fatal("expected a decode error")
	}
}

func TestPutSchedule_HonorsContextCancellation(t *testing.T) {
	c := newScheduleTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("the API must not be called with a cancelled context")
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := c.PutSchedule(ctx, "Days", SchedulePutRequest{Data: businessHours()}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// DeleteSchedule
// ---------------------------------------------------------------------------

func TestDeleteSchedule_Succeeds(t *testing.T) {
	var method, path string
	c := newScheduleTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		if _, err := io.WriteString(w, `{"ok":true,"id":"Days"}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})

	if err := c.DeleteSchedule(context.Background(), "Days"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if method != http.MethodDelete || path != "/schedules/Days" {
		t.Fatalf("unexpected request %s %s", method, path)
	}
}

func TestDeleteSchedule_IsIdempotent(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"http 404", http.StatusNotFound, `{"error":"not found"}`},
		{"error body with 200", http.StatusOK, `{"error":"schedule does not exist"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newScheduleTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				if _, err := io.WriteString(w, tc.body); err != nil {
					t.Errorf("write failed: %v", err)
				}
			})

			err := c.DeleteSchedule(context.Background(), "Days")
			var notFound *NotFoundError
			if !errors.As(err, &notFound) {
				t.Fatalf("expected *NotFoundError, got %v", err)
			}
		})
	}
}

func TestDeleteSchedule_SurfacesRealFailures(t *testing.T) {
	c := newScheduleTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.WriteString(w, `{"error":"schedule in use"}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})

	err := c.DeleteSchedule(context.Background(), "Days")
	if err == nil || !strings.Contains(err.Error(), "schedule in use") {
		t.Fatalf("expected the API message to surface, got %v", err)
	}
	var notFound *NotFoundError
	if errors.As(err, &notFound) {
		t.Fatal("an operational error must not be reported as not-found")
	}

	server := newScheduleTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	if err := server.DeleteSchedule(context.Background(), "Days"); err == nil {
		t.Fatal("expected an error for HTTP 500")
	}
}

func TestDeleteSchedule_ValidatesIDBeforeCallingTheAPI(t *testing.T) {
	c := newScheduleTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("the API must not be called with an invalid ID")
	})

	if err := c.DeleteSchedule(context.Background(), "  "); err == nil {
		t.Fatal("expected a validation error")
	}
}
