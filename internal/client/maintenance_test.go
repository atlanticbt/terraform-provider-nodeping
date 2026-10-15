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

func newMaintenanceTestClient(t *testing.T, handler http.HandlerFunc) *Client {
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

func maintenanceJSON(t *testing.T, m Maintenance) []byte {
	t.Helper()
	body, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal maintenance: %v", err)
	}
	return body
}

func TestMaintenanceIdentifierPrefersIDThenUnderscoreID(t *testing.T) {
	m := Maintenance{ID: "9VKOB"}
	if m.Identifier() != "9VKOB" {
		t.Fatalf("identifier = %q", m.Identifier())
	}

	m = Maintenance{UnderscoreID: "ABCDE"}
	if m.Identifier() != "ABCDE" {
		t.Fatalf("identifier = %q", m.Identifier())
	}

	m = Maintenance{}
	if m.Identifier() != "" {
		t.Fatalf("identifier = %q", m.Identifier())
	}
}

func TestMaintenanceIsAdHocIsCaseInsensitive(t *testing.T) {
	for _, id := range []string{"ad-hoc", "AD-HOC", "Ad-Hoc"} {
		m := Maintenance{ID: id}
		if !m.IsAdHoc() {
			t.Fatalf("%q should be ad-hoc", id)
		}
	}
	m := Maintenance{ID: "9VKOB"}
	if m.IsAdHoc() {
		t.Fatal("9VKOB should not be ad-hoc")
	}
}

func TestSortedHistoryIsDeterministicAndOldestFirst(t *testing.T) {
	m := Maintenance{History: map[string]MaintenanceRun{
		"1700000300000": {DisabledChecklist: []string{"C3"}},
		"1700000100000": {DisabledChecklist: []string{"C1"}, SkippedChecklist: []string{"S1"}},
		"1700000200000": {DisabledChecklist: []string{"C2"}},
		"not-a-number":  {DisabledChecklist: []string{"C0"}},
	}}

	for i := 0; i < 200; i++ {
		got := m.SortedHistory()
		if len(got) != 4 {
			t.Fatalf("got %d entries", len(got))
		}
		if got[0].Key != "not-a-number" {
			t.Fatalf("iteration %d: unparseable key should sort first, got %q", i, got[0].Key)
		}
		if got[1].Timestamp != 1700000100000 || got[2].Timestamp != 1700000200000 || got[3].Timestamp != 1700000300000 {
			t.Fatalf("iteration %d: unsorted history: %+v", i, got)
		}
	}

	if got := got1Skipped(m); got != "S1" {
		t.Fatalf("skipped checklist = %q", got)
	}

	if (&Maintenance{}).SortedHistory() != nil {
		t.Fatal("expected nil history for an empty map")
	}
}

func got1Skipped(m Maintenance) string {
	entries := m.SortedHistory()
	if len(entries) < 2 || len(entries[1].SkippedChecklist) == 0 {
		return ""
	}
	return entries[1].SkippedChecklist[0]
}

func TestNormalizeChecklistSortsAndDeduplicates(t *testing.T) {
	got := normalizeChecklist([]string{"C3", "C1", "C3", "C2"})
	if len(got) != 3 || got[0] != "C1" || got[1] != "C2" || got[2] != "C3" {
		t.Fatalf("normalizeChecklist = %v", got)
	}

	if got := normalizeChecklist(nil); got == nil || len(got) != 0 {
		t.Fatalf("expected a non-nil empty slice, got %#v", got)
	}
}

func TestValidateMaintenanceNameRejectsEmptyOversizedAndControlChars(t *testing.T) {
	if err := validateMaintenanceName("", true); err == nil {
		t.Fatal("expected an error for an empty required name")
	}
	if err := validateMaintenanceName("", false); err != nil {
		t.Fatalf("empty optional name should be accepted: %v", err)
	}
	if err := validateMaintenanceName(strings.Repeat("a", maxMaintenanceNameLength+1), true); err == nil {
		t.Fatal("expected an error for an oversized name")
	}
	if err := validateMaintenanceName("bad\nname", true); err == nil {
		t.Fatal("expected an error for a name containing a newline")
	}
	if err := validateMaintenanceName("Weekly patching", true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateMaintenanceDurationBounds(t *testing.T) {
	if err := validateMaintenanceDuration(0); err == nil {
		t.Fatal("expected an error for duration 0")
	}
	if err := validateMaintenanceDuration(-5); err == nil {
		t.Fatal("expected an error for a negative duration")
	}
	if err := validateMaintenanceDuration(maxMaintenanceDuration + 1); err == nil {
		t.Fatal("expected an error for an oversized duration")
	}
	if err := validateMaintenanceDuration(1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateChecklistBounds(t *testing.T) {
	if err := validateChecklist(nil); err == nil {
		t.Fatal("expected an error for an empty checklist")
	}
	if err := validateChecklist([]string{" "}); err == nil {
		t.Fatal("expected an error for a blank check ID")
	}
	if err := validateChecklist([]string{"good", "bad\x00id"}); err == nil {
		t.Fatal("expected an error for a check ID containing a NUL")
	}

	oversized := make([]string, maxChecklistEntries+1)
	for i := range oversized {
		oversized[i] = "C"
	}
	if err := validateChecklist(oversized); err == nil {
		t.Fatal("expected an error for an oversized checklist")
	}

	if err := validateChecklist([]string{"C1", "C2"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateCronStructural(t *testing.T) {
	if err := validateCron(""); err != nil {
		t.Fatalf("empty cron should be accepted: %v", err)
	}
	if err := validateCron("0 3 * *"); err == nil {
		t.Fatal("expected an error for a 4-field cron")
	}
	if err := validateCron("0 3 * * 0\n"); err == nil {
		t.Fatal("expected an error for a cron containing a newline")
	}
	if err := validateCron("0 3 * * 0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListMaintenance(t *testing.T) {
	c := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet || req.URL.Path != "/maintenance" {
			t.Errorf("unexpected request %s %s", req.Method, req.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"9VKOB":{"id":"9VKOB","name":"Weekly","enabled":true,"cron":"0 3 * * 0","duration":60,"checklist":["C1"]}}`))
	})

	got, err := c.ListMaintenance(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got["9VKOB"].Name != "Weekly" {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestListMaintenanceWrapsError(t *testing.T) {
	c := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"nope"}`))
	})

	_, err := c.ListMaintenance(context.Background())
	if err == nil || !strings.Contains(err.Error(), "failed to list maintenance windows") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// GET /maintenance wraps the windows in an envelope with `customer_id`,
// `schedules` and `ad-hoc` keys rather than returning them keyed by ID.
func TestListMaintenanceDecodesTheEnvelope(t *testing.T) {
	c := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"customer_id": "201203232048C76FH",
			"schedules": {
				"9VKOB": {"id":"9VKOB","name":"Weekly","enabled":true,"cron":"0 3 * * 0","duration":60,"checklist":["C1"]}
			},
			"ad-hoc": {}
		}`))
	})

	got, err := c.ListMaintenance(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 window, got %d: %+v", len(got), got)
	}
	window := got["9VKOB"]
	if window.Name != "Weekly" || window.Duration != 60 {
		t.Fatalf("unexpected window: %+v", window)
	}
	if window.IsAdHoc() {
		t.Error("a window under `schedules` must not be flagged as ad-hoc")
	}
}

// An ad-hoc window carries its own generated ID, so the envelope key it arrives
// under is the only signal that it is ad-hoc.
func TestListMaintenanceFlagsWindowsUnderTheAdHocKey(t *testing.T) {
	c := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"customer_id": "201203232048C76FH",
			"schedules": {"9VKOB": {"id":"9VKOB","name":"Weekly","duration":60}},
			"ad-hoc": {"9VKOC": {"id":"9VKOC","name":"One off","duration":30}}
		}`))
	})

	got, err := c.ListMaintenance(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 windows, got %d: %+v", len(got), got)
	}
	adHoc := got["9VKOC"]
	if !adHoc.IsAdHoc() {
		t.Error("a window under `ad-hoc` must be flagged as ad-hoc")
	}
	scheduled := got["9VKOB"]
	if scheduled.IsAdHoc() {
		t.Error("a window under `schedules` must not be flagged as ad-hoc")
	}
}

func TestListMaintenanceReturnsNoWindowsForAnEmptyEnvelope(t *testing.T) {
	c := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"customer_id":"201203232048C76FH","schedules":{},"ad-hoc":{}}`))
	})

	got, err := c.ListMaintenance(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no windows, got %+v", got)
	}
}

func TestListMaintenanceReportsAnUndecodableWindow(t *testing.T) {
	c := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"customer_id":"C1","schedules":{"9VKOB":"not a window"},"ad-hoc":{}}`))
	})

	_, err := c.ListMaintenance(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "failed to list maintenance windows") ||
		!strings.Contains(err.Error(), "schedules") {
		t.Fatalf("expected the error to name the failing section, got %v", err)
	}
}

// The live listing carries no `id` inside the window documents — the envelope
// key is the only identifier — so the key has to be stamped onto the window.
func TestListMaintenanceStampsIDsFromTheEnvelopeKeys(t *testing.T) {
	c := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"customer_id": "20210101000000EXAMPL",
			"schedules": {"MCARA": {"name":"probe","enabled":false,"cron":"0 4 * * 6","duration":90,"checklist":["C1"]}},
			"ad-hoc": {"MCARB": {"name":"one off","duration":30}}
		}`))
	})

	got, err := c.ListMaintenance(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	scheduled := got["MCARA"]
	if scheduled.Identifier() != "MCARA" {
		t.Fatalf("scheduled id was not stamped from the key: %q", scheduled.Identifier())
	}
	adHoc := got["MCARB"]
	if adHoc.Identifier() != "MCARB" {
		t.Fatalf("ad-hoc id was not stamped from the key: %q", adHoc.Identifier())
	}
	if !adHoc.IsAdHoc() || scheduled.IsAdHoc() {
		t.Fatalf("ad-hoc grouping lost: scheduled=%v adhoc=%v", scheduled.IsAdHoc(), adHoc.IsAdHoc())
	}
}

// A listing with only one of the two groups present must still decode; the
// absent group contributes nothing rather than failing the read.
func TestListMaintenanceAcceptsAnEnvelopeMissingAGroup(t *testing.T) {
	c := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"customer_id":"C1","schedules":{"MCARA":{"name":"probe","duration":90}}}`))
	})

	got, err := c.ListMaintenance(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got["MCARA"].Name != "probe" {
		t.Fatalf("unexpected result: %+v", got)
	}
}

// Every path shares one decoder, so every path must report a body it cannot
// read rather than treating it as an empty window.
func TestMaintenanceReportsAnUndecodableResponseBody(t *testing.T) {
	cases := map[string]struct {
		body string
		call func(*Client) error
		want string
	}{
		"list gets a JSON array": {
			body: `[1,2,3]`,
			call: func(c *Client) error { _, err := c.ListMaintenance(context.Background()); return err },
			want: "failed to list maintenance windows",
		},
		"get gets a JSON array": {
			body: `[1,2,3]`,
			call: func(c *Client) error { _, err := c.GetMaintenance(context.Background(), "MCARA"); return err },
			want: "failed to get maintenance window",
		},
		"list gets a group that is not an object": {
			body: `{"customer_id":"C1","schedules":5,"ad-hoc":{}}`,
			call: func(c *Client) error { _, err := c.ListMaintenance(context.Background()); return err },
			want: "schedules",
		},
		"get gets a window field of the wrong type": {
			body: `{"name":123}`,
			call: func(c *Client) error { _, err := c.GetMaintenance(context.Background(), "MCARA"); return err },
			want: "failed to get maintenance window",
		},
		"create gets a JSON array": {
			body: `[1,2,3]`,
			call: func(c *Client) error {
				_, err := c.CreateMaintenance(context.Background(), MaintenanceCreateRequest{
					Name: "n", Cron: "0 3 * * 0", Duration: 60, Checklist: []string{"C1"},
				})
				return err
			},
			want: "failed to create maintenance window",
		},
		"update gets a JSON array": {
			body: `[1,2,3]`,
			call: func(c *Client) error {
				_, err := c.UpdateMaintenance(context.Background(), "MCARA", MaintenanceUpdateRequest{
					Name: "n", Duration: 30, Checklist: []string{"C1"},
				})
				return err
			},
			want: "failed to update maintenance window",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			})

			err := tc.call(c)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected an error naming %q, got %v", tc.want, err)
			}
		})
	}
}

func TestGetMaintenance(t *testing.T) {
	nextRun := int64(1700000000000)
	c := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/maintenance/9VKOB" {
			t.Errorf("unexpected path %s", req.URL.Path)
		}
		_, _ = w.Write(maintenanceJSON(t, Maintenance{
			ID: "9VKOB", Name: "Weekly", Enabled: true, Cron: "0 3 * * 0",
			Duration: 60, Checklist: []string{"C1"}, NextRun: &nextRun,
			Update: &MaintenanceUpdateInfo{Timestamp: 1699999999000, User: "ops@example.com"},
		}))
	})

	got, err := c.GetMaintenance(context.Background(), "9VKOB")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Identifier() != "9VKOB" || got.Duration != 60 || got.NextRun == nil || *got.NextRun != nextRun {
		t.Fatalf("unexpected result: %+v", got)
	}
	if got.Update == nil || got.Update.User != "ops@example.com" {
		t.Fatalf("update info not decoded: %+v", got.Update)
	}
}

func TestGetMaintenanceRejectsEmptyID(t *testing.T) {
	c := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		t.Error("no request should be made")
	})

	if _, err := c.GetMaintenance(context.Background(), "  "); err == nil {
		t.Fatal("expected a validation error")
	}
}

func TestGetMaintenanceEscapesIDInPath(t *testing.T) {
	var seen string
	c := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		seen = req.URL.EscapedPath()
		_, _ = w.Write(maintenanceJSON(t, Maintenance{ID: "x", Name: "n", Duration: 1}))
	})

	if _, err := c.GetMaintenance(context.Background(), "a/b"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seen != "/maintenance/a%2Fb" {
		t.Fatalf("path was not escaped: %q", seen)
	}
}

func TestGetMaintenanceNotFound(t *testing.T) {
	c := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	})

	_, err := c.GetMaintenance(context.Background(), "GONE")
	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected *NotFoundError, got %v", err)
	}
}

func TestGetMaintenanceEmptyBodyIsTreatedAsNotFound(t *testing.T) {
	c := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})

	_, err := c.GetMaintenance(context.Background(), "GONE")
	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected *NotFoundError, got %v", err)
	}
}

// Some deployments answer a missing window with a 200 and no body at all,
// which must read as absent rather than as a decode failure.
func TestGetMaintenanceAbsentBodyIsTreatedAsNotFound(t *testing.T) {
	c := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
	})

	_, err := c.GetMaintenance(context.Background(), "GONE")
	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected *NotFoundError, got %v", err)
	}
}

func TestGetMaintenanceBackfillsIDFromRequest(t *testing.T) {
	c := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"name":"Weekly","duration":30}`))
	})

	got, err := c.GetMaintenance(context.Background(), "9VKOB")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Identifier() != "9VKOB" {
		t.Fatalf("id was not backfilled: %q", got.Identifier())
	}
}

// GET /maintenance/:id answers with the window keyed by its ID, not as a bare
// document. Decoded as a bare document it reads as empty, and an empty read is
// reported as not-found — every refresh would have deleted the resource from
// state.
func TestGetMaintenanceDecodesTheIDKeyedEnvelope(t *testing.T) {
	c := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MCARA":{"name":"probe-window","enabled":false,"processing":false,
			"cron":"0 3 * * 6","duration":60,"history":{},"nextrun":1790391600000,
			"checklist":["20210101000000EXAMPL-CHK00001"],
			"update":{"user":"","timestamp":1790091625424},"id":"MCARA"}}`))
	})

	got, err := c.GetMaintenance(context.Background(), "MCARA")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Identifier() != "MCARA" || got.Name != "probe-window" || got.Duration != 60 {
		t.Fatalf("unexpected result: %+v", got)
	}
	if got.NextRun == nil || *got.NextRun != 1790391600000 {
		t.Fatalf("nextrun not decoded: %+v", got.NextRun)
	}
	if len(got.Checklist) != 1 || got.Checklist[0] != "20210101000000EXAMPL-CHK00001" {
		t.Fatalf("checklist not decoded: %+v", got.Checklist)
	}
}

func TestGetMaintenanceWrapsServerError(t *testing.T) {
	c := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})

	_, err := c.GetMaintenance(context.Background(), "9VKOB")
	if err == nil || !strings.Contains(err.Error(), "failed to get maintenance window") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateMaintenanceSendsSortedChecklistAndEnabled(t *testing.T) {
	var body map[string]interface{}
	c := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost || req.URL.Path != "/maintenance" {
			t.Errorf("unexpected request %s %s", req.Method, req.URL.Path)
		}
		raw, _ := io.ReadAll(req.Body)
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("bad request body: %v", err)
		}
		_, _ = w.Write(maintenanceJSON(t, Maintenance{
			ID: "9VKOB", Name: "Weekly", Cron: "0 3 * * 0", Duration: 60, Checklist: []string{"C1", "C2"},
		}))
	})

	enabled := false
	got, err := c.CreateMaintenance(context.Background(), MaintenanceCreateRequest{
		Name: "Weekly", Enabled: &enabled, Cron: "0 3 * * 0", Duration: 60,
		Checklist: []string{"C2", "C1", "C2"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Identifier() != "9VKOB" {
		t.Fatalf("unexpected id %q", got.Identifier())
	}

	// enabled=false must be transmitted, otherwise disabling a window never
	// reaches the API and drifts forever.
	if v, ok := body["enabled"].(bool); !ok || v {
		t.Fatalf("enabled was not sent as false: %#v", body["enabled"])
	}

	list, ok := body["checklist"].([]interface{})
	if !ok || len(list) != 2 || list[0] != "C1" || list[1] != "C2" {
		t.Fatalf("checklist was not sorted and de-duplicated: %#v", body["checklist"])
	}
}

func TestCreateMaintenanceValidatesInput(t *testing.T) {
	c := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		t.Error("no request should be made for invalid input")
	})

	valid := MaintenanceCreateRequest{Name: "n", Cron: "0 3 * * 0", Duration: 60, Checklist: []string{"C1"}}

	cases := map[string]func(MaintenanceCreateRequest) MaintenanceCreateRequest{
		"empty name":      func(r MaintenanceCreateRequest) MaintenanceCreateRequest { r.Name = ""; return r },
		"zero duration":   func(r MaintenanceCreateRequest) MaintenanceCreateRequest { r.Duration = 0; return r },
		"empty checklist": func(r MaintenanceCreateRequest) MaintenanceCreateRequest { r.Checklist = nil; return r },
		"malformed cron":  func(r MaintenanceCreateRequest) MaintenanceCreateRequest { r.Cron = "0 3 *"; return r },
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := c.CreateMaintenance(context.Background(), mutate(valid)); err == nil {
				t.Fatal("expected a validation error")
			}
		})
	}
}

func TestCreateMaintenanceRejectsMissingAndAdHocIDInResponse(t *testing.T) {
	req := MaintenanceCreateRequest{Name: "n", Cron: "0 3 * * 0", Duration: 60, Checklist: []string{"C1"}}

	noID := newMaintenanceTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"name":"n"}`))
	})
	if _, err := noID.CreateMaintenance(context.Background(), req); err == nil ||
		!strings.Contains(err.Error(), "no id") {
		t.Fatalf("unexpected error: %v", err)
	}

	adHoc := newMaintenanceTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"ad-hoc","name":"n"}`))
	})
	if _, err := adHoc.CreateMaintenance(context.Background(), req); err == nil ||
		!strings.Contains(err.Error(), "ad-hoc") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// POST /maintenance answers with the created window keyed by its generated ID
// beside `ok` and `message`. There is no top-level `id`, so decoding the body
// as a bare window found no identifier and failed the create — after NodePing
// had already created the window, leaking one on every attempt.
func TestCreateMaintenanceDecodesTheIDKeyedEnvelope(t *testing.T) {
	c := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"HDDK2":{"name":"diag-window","cron":"0 3 * * 6","duration":60,
			"enabled":false,"processing":false,"history":{},"nextrun":1790391600000,
			"checklist":["20210101000000EXAMPL-CHK00001"],
			"update":{"user":"","timestamp":1790090708305}},
			"message":"Maintenance settings were successfully saved.","ok":true}`))
	})

	got, err := c.CreateMaintenance(context.Background(), MaintenanceCreateRequest{
		Name: "diag-window", Cron: "0 3 * * 6", Duration: 60,
		Checklist: []string{"20210101000000EXAMPL-CHK00001"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Identifier() != "HDDK2" {
		t.Fatalf("id was not taken from the envelope key: %q", got.Identifier())
	}
	if got.Name != "diag-window" || got.Duration != 60 || got.Cron != "0 3 * * 6" {
		t.Fatalf("window body was not decoded: %+v", got)
	}
	if got.IsAdHoc() {
		t.Error("a window created with a cron must not be reported as ad-hoc")
	}
}

func TestCreateMaintenanceWrapsError(t *testing.T) {
	c := newMaintenanceTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})

	_, err := c.CreateMaintenance(context.Background(), MaintenanceCreateRequest{
		Name: "n", Cron: "0 3 * * 0", Duration: 60, Checklist: []string{"C1"},
	})
	if err == nil || !strings.Contains(err.Error(), "failed to create maintenance window") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateMaintenanceSendsIDInBody(t *testing.T) {
	var body map[string]interface{}
	c := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPut || req.URL.Path != "/maintenance/9VKOB" {
			t.Errorf("unexpected request %s %s", req.Method, req.URL.Path)
		}
		raw, _ := io.ReadAll(req.Body)
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("bad request body: %v", err)
		}
		_, _ = w.Write(maintenanceJSON(t, Maintenance{ID: "9VKOB", Name: "New", Duration: 30, Checklist: []string{"C1"}}))
	})

	got, err := c.UpdateMaintenance(context.Background(), "9VKOB", MaintenanceUpdateRequest{
		Name: "New", Duration: 30, Checklist: []string{"C1"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "New" {
		t.Fatalf("unexpected result %+v", got)
	}
	if body["id"] != "9VKOB" {
		t.Fatalf("id was not sent in the body: %#v", body["id"])
	}
}

func TestUpdateMaintenanceRejectsAdHocAndInvalidInput(t *testing.T) {
	c := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		t.Error("no request should be made for invalid input")
	})

	valid := MaintenanceUpdateRequest{Name: "n", Duration: 30, Checklist: []string{"C1"}}

	if _, err := c.UpdateMaintenance(context.Background(), "ad-hoc", valid); err == nil {
		t.Fatal("expected ad-hoc updates to be rejected")
	}
	if _, err := c.UpdateMaintenance(context.Background(), "", valid); err == nil {
		t.Fatal("expected an empty ID to be rejected")
	}

	invalid := valid
	invalid.Duration = 0
	if _, err := c.UpdateMaintenance(context.Background(), "9VKOB", invalid); err == nil {
		t.Fatal("expected a duration validation error")
	}

	invalid = valid
	invalid.Checklist = nil
	if _, err := c.UpdateMaintenance(context.Background(), "9VKOB", invalid); err == nil {
		t.Fatal("expected a checklist validation error")
	}

	invalid = valid
	invalid.Cron = "bogus"
	if _, err := c.UpdateMaintenance(context.Background(), "9VKOB", invalid); err == nil {
		t.Fatal("expected a cron validation error")
	}

	invalid = valid
	invalid.Name = strings.Repeat("a", maxMaintenanceNameLength+1)
	if _, err := c.UpdateMaintenance(context.Background(), "9VKOB", invalid); err == nil {
		t.Fatal("expected a name validation error")
	}
}

func TestUpdateMaintenanceNotFoundAndWrappedError(t *testing.T) {
	missing := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	_, err := missing.UpdateMaintenance(context.Background(), "GONE", MaintenanceUpdateRequest{
		Name: "n", Duration: 30, Checklist: []string{"C1"},
	})
	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected *NotFoundError, got %v", err)
	}

	broken := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	_, err = broken.UpdateMaintenance(context.Background(), "9VKOB", MaintenanceUpdateRequest{
		Name: "n", Duration: 30, Checklist: []string{"C1"},
	})
	if err == nil || !strings.Contains(err.Error(), "failed to update maintenance window") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateMaintenanceBackfillsIDInResponse(t *testing.T) {
	c := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"name":"New","duration":30}`))
	})

	got, err := c.UpdateMaintenance(context.Background(), "9VKOB", MaintenanceUpdateRequest{
		Name: "New", Duration: 30, Checklist: []string{"C1"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Identifier() != "9VKOB" {
		t.Fatalf("id was not backfilled: %q", got.Identifier())
	}
}

// PUT /maintenance/:id answers with the same ID-keyed envelope a create does.
// Decoded as a bare window it yielded a record with nothing but the ID stamped
// back on, so an update wrote an empty name, a zero duration and no checklist
// into state without reporting an error.
func TestUpdateMaintenanceDecodesTheIDKeyedEnvelope(t *testing.T) {
	c := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MCARA":{"name":"probe-window-updated","enabled":false,
			"processing":false,"cron":"0 4 * * 6","duration":90,"history":{},
			"nextrun":1790395200000,"checklist":["20210101000000EXAMPL-CHK00001"],
			"update":{"user":"","timestamp":1790091625700}},
			"message":"Maintenance settings were successfully saved.","ok":true}`))
	})

	got, err := c.UpdateMaintenance(context.Background(), "MCARA", MaintenanceUpdateRequest{
		Name: "probe-window-updated", Cron: "0 4 * * 6", Duration: 90,
		Checklist: []string{"20210101000000EXAMPL-CHK00001"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Identifier() != "MCARA" {
		t.Fatalf("unexpected id %q", got.Identifier())
	}
	if got.Name != "probe-window-updated" || got.Duration != 90 || got.Cron != "0 4 * * 6" {
		t.Fatalf("window body was not decoded: %+v", got)
	}
	if len(got.Checklist) != 1 {
		t.Fatalf("checklist was not decoded: %+v", got.Checklist)
	}
}

// An update that returns no window at all must be reported. Stamping the ID
// onto an empty record would persist a hollow window to state and plan the
// real settings away on the next run.
func TestUpdateMaintenanceReportsAResponseWithNoWindow(t *testing.T) {
	c := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"message":"Maintenance settings were successfully saved."}`))
	})

	_, err := c.UpdateMaintenance(context.Background(), "MCARA", MaintenanceUpdateRequest{
		Name: "New", Duration: 30, Checklist: []string{"C1"},
	})
	if err == nil || !strings.Contains(err.Error(), "failed to update maintenance window") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteMaintenance(t *testing.T) {
	c := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodDelete || req.URL.Path != "/maintenance/9VKOB" {
			t.Errorf("unexpected request %s %s", req.Method, req.URL.Path)
		}
		_, _ = w.Write([]byte(`{"ok":true,"id":"9VKOB"}`))
	})

	if err := c.DeleteMaintenance(context.Background(), "9VKOB"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteMaintenanceNotFoundAndValidation(t *testing.T) {
	missing := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	err := missing.DeleteMaintenance(context.Background(), "GONE")
	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected *NotFoundError, got %v", err)
	}

	if err := missing.DeleteMaintenance(context.Background(), ""); err == nil {
		t.Fatal("expected an empty ID to be rejected")
	}

	broken := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	if err := broken.DeleteMaintenance(context.Background(), "9VKOB"); err == nil ||
		!strings.Contains(err.Error(), "failed to delete maintenance window") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMaintenanceRequestsScopedToCustomerID(t *testing.T) {
	var seen string
	c := newMaintenanceTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		seen = req.URL.Query().Get("customerid")
		_, _ = w.Write([]byte(`{}`))
	}).WithCustomerID("SUB1")

	if _, err := c.ListMaintenance(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seen != "SUB1" {
		t.Fatalf("customerid = %q", seen)
	}
}
