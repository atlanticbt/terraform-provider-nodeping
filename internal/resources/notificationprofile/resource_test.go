package notificationprofile

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/atlanticbt/terraform-provider-nodeping/internal/client"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newTestResource(t *testing.T, handler http.HandlerFunc) (*NotificationProfileResource, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c := client.NewClient(client.ClientConfig{
		APIToken:     "test-token",
		BaseURL:      srv.URL,
		RateLimit:    1000,
		MaxRetries:   1,
		RetryMinWait: time.Millisecond,
		RetryMaxWait: time.Millisecond,
		Timeout:      5 * time.Second,
	})

	return &NotificationProfileResource{client: c}, srv
}

func testPlan(t *testing.T, ctx context.Context, model NotificationProfileResourceModel) tfsdk.Plan {
	t.Helper()
	s := NotificationProfileSchema()
	p := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if diags := p.Set(ctx, &model); diags.HasError() {
		t.Fatalf("failed to build plan: %v", diags)
	}
	return p
}

func testState(t *testing.T, ctx context.Context, model *NotificationProfileResourceModel) tfsdk.State {
	t.Helper()
	s := NotificationProfileSchema()
	st := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if model != nil {
		if diags := st.Set(ctx, model); diags.HasError() {
			t.Fatalf("failed to build state: %v", diags)
		}
	}
	return st
}

func decodeState(t *testing.T, ctx context.Context, st tfsdk.State) NotificationProfileResourceModel {
	t.Helper()
	var out NotificationProfileResourceModel
	if diags := st.Get(ctx, &out); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	return out
}

func notif(contactID string, delay int64, schedule string) NotificationModel {
	return NotificationModel{
		ContactID: types.StringValue(contactID),
		Delay:     types.Int64Value(delay),
		Schedule:  types.StringValue(schedule),
	}
}

func apiNotif(contactID string, delay int64, schedule string) map[string]interface{} {
	return map[string]interface{}{
		contactID: map[string]interface{}{
			"delay":    float64(delay),
			"schedule": schedule,
		},
	}
}

func profileJSON(t *testing.T, id, customerID, name string, notifications []map[string]interface{}) []byte {
	t.Helper()
	body, err := json.Marshal(client.NotificationProfile{
		ID:            id,
		Type:          "notification profile",
		CustomerID:    customerID,
		Name:          name,
		Notifications: notifications,
	})
	if err != nil {
		t.Fatalf("failed to marshal profile: %v", err)
	}
	return body
}

func contactIDs(notifications []NotificationModel) []string {
	out := make([]string, 0, len(notifications))
	for _, n := range notifications {
		out = append(out, n.ContactID.ValueString())
	}
	return out
}

// ---------------------------------------------------------------------------
// metadata / schema / configure
// ---------------------------------------------------------------------------

func TestMetadataUsesProviderTypeName(t *testing.T) {
	r := NewNotificationProfileResource()
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "nodeping"}, resp)

	if resp.TypeName != "nodeping_notificationprofile" {
		t.Fatalf("expected nodeping_notificationprofile, got %s", resp.TypeName)
	}
}

func TestSchemaExposesNotificationsBlock(t *testing.T) {
	r := NewNotificationProfileResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	if _, ok := resp.Schema.Blocks["notifications"]; !ok {
		t.Fatal("expected a notifications block")
	}
	if !resp.Schema.Attributes["name"].IsRequired() {
		t.Fatal("expected name to be required")
	}
	customerID := resp.Schema.Attributes["customer_id"]
	if !customerID.IsOptional() || !customerID.IsComputed() {
		t.Fatal("expected customer_id to be optional and computed")
	}
}

func TestConfigureAcceptsClientAndRejectsOtherTypes(t *testing.T) {
	ctx := context.Background()

	r := &NotificationProfileResource{}
	resp := &resource.ConfigureResponse{}
	r.Configure(ctx, resource.ConfigureRequest{}, resp)
	if r.client != nil {
		t.Fatal("expected no client when ProviderData is nil")
	}

	c := client.NewClient(client.ClientConfig{APIToken: "t"})
	resp = &resource.ConfigureResponse{}
	r.Configure(ctx, resource.ConfigureRequest{ProviderData: c}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if r.client != c {
		t.Fatal("expected the provider client to be stored")
	}

	resp = &resource.ConfigureResponse{}
	r.Configure(ctx, resource.ConfigureRequest{ProviderData: "not a client"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for an unexpected ProviderData type")
	}
}

func TestClientForScopesToCustomer(t *testing.T) {
	c := client.NewClient(client.ClientConfig{APIToken: "t"})
	r := &NotificationProfileResource{client: c}

	if r.clientFor(types.StringNull()) != c {
		t.Fatal("expected the provider client when customer_id is null")
	}
	if r.clientFor(types.StringUnknown()) != c {
		t.Fatal("expected the provider client when customer_id is unknown")
	}
	if r.clientFor(types.StringValue("")) != c {
		t.Fatal("expected the provider client when customer_id is empty")
	}
	if r.clientFor(types.StringValue("SUB1")) == c {
		t.Fatal("expected a scoped client when customer_id is set")
	}
}

// ---------------------------------------------------------------------------
// notification mapping
// ---------------------------------------------------------------------------

func TestMapNotificationsIsDeterministicAcrossRepeatedRuns(t *testing.T) {
	// A single element carrying several keys is the pathological case: ranging
	// the map would produce a different order on every run.
	api := []map[string]interface{}{
		{
			"C-3": map[string]interface{}{"delay": float64(30), "schedule": "Nights"},
			"A-1": map[string]interface{}{"delay": float64(0), "schedule": "All"},
			"B-2": map[string]interface{}{"delay": float64(15), "schedule": "Days"},
		},
		apiNotif("Z-9", 5, "Weekends"),
	}

	want := []string{"A-1", "B-2", "C-3", "Z-9"}
	for i := 0; i < 50; i++ {
		got := contactIDs(mapNotifications(api))
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d: got %v, want %v", i, got, want)
		}
	}
}

func TestMapNotificationsPreservesRepeatedContactIDs(t *testing.T) {
	api := []map[string]interface{}{
		apiNotif("A-1", 0, "All"),
		apiNotif("A-1", 30, "Days"),
		apiNotif("A-1", 60, "Nights"),
	}

	got := mapNotifications(api)
	if len(got) != 3 {
		t.Fatalf("expected 3 notifications, got %d", len(got))
	}
	for i, wantDelay := range []int64{0, 30, 60} {
		if got[i].Delay.ValueInt64() != wantDelay {
			t.Fatalf("element %d: got delay %d, want %d", i, got[i].Delay.ValueInt64(), wantDelay)
		}
	}
}

func TestMapNotificationsHandlesEmptyAndMalformedInput(t *testing.T) {
	if got := mapNotifications(nil); got != nil {
		t.Fatalf("expected nil for no notifications, got %v", got)
	}
	// A non-object value for a contact key is skipped rather than panicking.
	if got := mapNotifications([]map[string]interface{}{{"A-1": "nonsense"}}); got != nil {
		t.Fatalf("expected nil for malformed notifications, got %v", got)
	}
}

func TestMapNotificationsAppliesDefaults(t *testing.T) {
	got := mapNotifications([]map[string]interface{}{{"A-1": map[string]interface{}{}}})
	if len(got) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(got))
	}
	if got[0].Delay.ValueInt64() != 0 {
		t.Fatalf("expected delay 0, got %d", got[0].Delay.ValueInt64())
	}
	if got[0].Schedule.ValueString() != defaultSchedule {
		t.Fatalf("expected schedule %q, got %q", defaultSchedule, got[0].Schedule.ValueString())
	}
}

func TestMapNotificationsDecodesNumericVariants(t *testing.T) {
	cases := []interface{}{float64(45), 45, int64(45), "45", json.Number("45"), float32(45)}
	for _, delay := range cases {
		got := mapNotifications([]map[string]interface{}{
			{"A-1": map[string]interface{}{"delay": delay, "schedule": "All"}},
		})
		if len(got) != 1 || got[0].Delay.ValueInt64() != 45 {
			t.Fatalf("delay %#v: got %v", delay, got)
		}
	}

	// Values that cannot be decoded fall back to zero rather than erroring out.
	got := mapNotifications([]map[string]interface{}{
		{"A-1": map[string]interface{}{"delay": "not-a-number", "schedule": "All"}},
	})
	if len(got) != 1 || got[0].Delay.ValueInt64() != 0 {
		t.Fatalf("expected an undecodable delay to become 0, got %v", got)
	}
}

func TestToInt64RejectsUnusableValues(t *testing.T) {
	for _, v := range []interface{}{nil, "", "   ", true, []int{1}, json.Number("abc")} {
		if got, ok := toInt64(v); ok {
			t.Fatalf("expected %#v to be undecodable, got %d", v, got)
		}
	}
	if got, ok := toInt64("1.9"); !ok || got != 1 {
		t.Fatalf("expected decimal string to truncate to 1, got %d (ok=%v)", got, ok)
	}
	if got, ok := toInt64(json.Number("1.9")); !ok || got != 1 {
		t.Fatalf("expected json.Number float to truncate to 1, got %d (ok=%v)", got, ok)
	}
}

func TestNotificationsToAPIPreservesOrderAndDuplicates(t *testing.T) {
	planned := []NotificationModel{
		notif("B-2", 15, "Days"),
		notif("A-1", 0, "All"),
		notif("A-1", 30, "Nights"),
	}

	got := notificationsToAPI(planned)
	if len(got) != 3 {
		t.Fatalf("expected 3 wire elements, got %d", len(got))
	}
	wantKeys := []string{"B-2", "A-1", "A-1"}
	for i, want := range wantKeys {
		if len(got[i]) != 1 {
			t.Fatalf("element %d: expected a single key, got %d", i, len(got[i]))
		}
		if _, ok := got[i][want]; !ok {
			t.Fatalf("element %d: expected key %q, got %v", i, want, got[i])
		}
	}
}

func TestNotificationsToAPIDefaultsUnknownSchedule(t *testing.T) {
	planned := []NotificationModel{{
		ContactID: types.StringValue("A-1"),
		Delay:     types.Int64Value(0),
		Schedule:  types.StringUnknown(),
	}}

	got := notificationsToAPI(planned)
	entry, ok := got[0]["A-1"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected a nested object, got %#v", got[0]["A-1"])
	}
	if entry["schedule"] != defaultSchedule {
		t.Fatalf("expected schedule %q, got %v", defaultSchedule, entry["schedule"])
	}
}

func TestNotificationsToAPIEmitsEmptySliceNotNil(t *testing.T) {
	got := notificationsToAPI(nil)
	if got == nil {
		t.Fatal("expected an empty slice so the full-replace update clears the profile")
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 elements, got %d", len(got))
	}
}

func TestReconcileNotificationsKeepsPlanCountAndOrder(t *testing.T) {
	planned := []NotificationModel{
		notif("A-1", 0, "All"),
		notif("A-1", 30, "Days"),
		notif("B-2", 15, "Nights"),
	}
	// The API returns the same set in a different order.
	api := []NotificationModel{
		notif("B-2", 15, "Nights"),
		notif("A-1", 30, "Days"),
		notif("A-1", 0, "All"),
	}

	got := reconcileNotifications(planned, api)
	if len(got) != len(planned) {
		t.Fatalf("expected %d elements, got %d", len(planned), len(got))
	}
	if !reflect.DeepEqual(contactIDs(got), []string{"A-1", "A-1", "B-2"}) {
		t.Fatalf("unexpected order: %v", contactIDs(got))
	}
	if got[0].Delay.ValueInt64() != 0 || got[1].Delay.ValueInt64() != 30 {
		t.Fatalf("repeated blocks were paired incorrectly: %v", got)
	}
}

func TestReconcileNotificationsFillsUnknownsFromAPI(t *testing.T) {
	planned := []NotificationModel{{
		ContactID: types.StringValue("A-1"),
		Delay:     types.Int64Unknown(),
		Schedule:  types.StringUnknown(),
	}}
	api := []NotificationModel{notif("A-1", 45, "Weekends")}

	got := reconcileNotifications(planned, api)
	if got[0].Delay.ValueInt64() != 45 || got[0].Schedule.ValueString() != "Weekends" {
		t.Fatalf("unknowns were not filled from the API response: %v", got[0])
	}
}

func TestReconcileNotificationsResolvesUnmatchedUnknowns(t *testing.T) {
	planned := []NotificationModel{{
		ContactID: types.StringValue("MISSING"),
		Delay:     types.Int64Unknown(),
		Schedule:  types.StringUnknown(),
	}}

	got := reconcileNotifications(planned, nil)
	if len(got) != 1 {
		t.Fatalf("expected the planned block to survive, got %d", len(got))
	}
	if got[0].Delay.IsUnknown() || got[0].Schedule.IsUnknown() {
		t.Fatalf("unknown values must be resolved before they reach state: %v", got[0])
	}
	if got[0].Schedule.ValueString() != defaultSchedule {
		t.Fatalf("expected schedule %q, got %q", defaultSchedule, got[0].Schedule.ValueString())
	}
}

func TestReconcileNotificationsNilPlanStaysNil(t *testing.T) {
	if got := reconcileNotifications(nil, []NotificationModel{notif("A-1", 0, "All")}); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestAlignNotificationsFollowsPriorOrder(t *testing.T) {
	prior := []NotificationModel{notif("B-2", 15, "Days"), notif("A-1", 0, "All")}
	api := []NotificationModel{notif("A-1", 0, "All"), notif("B-2", 15, "Days"), notif("C-3", 5, "All")}

	got := alignNotifications(prior, api)
	if !reflect.DeepEqual(contactIDs(got), []string{"B-2", "A-1", "C-3"}) {
		t.Fatalf("unexpected order: %v", contactIDs(got))
	}
}

func TestAlignNotificationsDropsRemovedEntries(t *testing.T) {
	prior := []NotificationModel{notif("A-1", 0, "All"), notif("GONE", 0, "All")}
	api := []NotificationModel{notif("A-1", 0, "All")}

	got := alignNotifications(prior, api)
	if !reflect.DeepEqual(contactIDs(got), []string{"A-1"}) {
		t.Fatalf("expected the removed entry to be dropped, got %v", contactIDs(got))
	}
}

func TestAlignNotificationsPassesThroughEmptyInput(t *testing.T) {
	api := []NotificationModel{notif("A-1", 0, "All")}
	if got := alignNotifications(nil, api); !reflect.DeepEqual(got, api) {
		t.Fatalf("expected the API list unchanged, got %v", got)
	}
	if got := alignNotifications(api, nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// CRUD
// ---------------------------------------------------------------------------

func TestCreateSendsFullNotificationArrayAndStoresID(t *testing.T) {
	ctx := context.Background()

	var body map[string]interface{}
	var query string
	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", req.Method)
		}
		query = req.URL.Query().Get("customerid")
		raw, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(profileJSON(t, "ACC-P-3QJWG", "SUB1", "Escalation", []map[string]interface{}{
			apiNotif("A-1", 0, "All"),
			apiNotif("G-1", 15, "Days"),
		}))
	})

	plan := NotificationProfileResourceModel{
		ID:         types.StringUnknown(),
		CustomerID: types.StringValue("SUB1"),
		Name:       types.StringValue("Escalation"),
		Notifications: []NotificationModel{
			notif("A-1", 0, "All"),
			notif("G-1", 15, "Days"),
		},
	}

	resp := &resource.CreateResponse{State: testState(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{Plan: testPlan(t, ctx, plan)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if query != "SUB1" {
		t.Fatalf("expected the request to be scoped to SUB1, got %q", query)
	}
	notifications, ok := body["notifications"].([]interface{})
	if !ok || len(notifications) != 2 {
		t.Fatalf("expected 2 notifications on the wire, got %#v", body["notifications"])
	}

	state := decodeState(t, ctx, resp.State)
	if state.ID.ValueString() != "ACC-P-3QJWG" {
		t.Fatalf("expected the API ID in state, got %q", state.ID.ValueString())
	}
	if !reflect.DeepEqual(contactIDs(state.Notifications), []string{"A-1", "G-1"}) {
		t.Fatalf("unexpected notifications in state: %v", contactIDs(state.Notifications))
	}
}

func TestCreateFillsUnknownCustomerIDFromResponse(t *testing.T) {
	ctx := context.Background()

	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write(profileJSON(t, "ACC-P-1", "ROOT", "Basic", nil))
	})

	plan := NotificationProfileResourceModel{
		ID:         types.StringUnknown(),
		CustomerID: types.StringUnknown(),
		Name:       types.StringValue("Basic"),
	}

	resp := &resource.CreateResponse{State: testState(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{Plan: testPlan(t, ctx, plan)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	state := decodeState(t, ctx, resp.State)
	if state.CustomerID.ValueString() != "ROOT" {
		t.Fatalf("expected customer_id ROOT, got %q", state.CustomerID.ValueString())
	}
}

func TestCreateReportsAPIError(t *testing.T) {
	ctx := context.Background()

	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"name is required"}`))
	})

	plan := NotificationProfileResourceModel{
		ID:         types.StringUnknown(),
		CustomerID: types.StringUnknown(),
		Name:       types.StringValue("Broken"),
	}

	resp := &resource.CreateResponse{State: testState(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{Plan: testPlan(t, ctx, plan)}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic")
	}
}

func TestReadRefreshesStateAndKeepsNotificationOrder(t *testing.T) {
	ctx := context.Background()

	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		// The API reports the notifications in a different order than state.
		_, _ = w.Write(profileJSON(t, "ACC-P-1", "SUB1", "Renamed", []map[string]interface{}{
			apiNotif("B-2", 15, "Days"),
			apiNotif("A-1", 0, "All"),
		}))
	})

	prior := NotificationProfileResourceModel{
		ID:         types.StringValue("ACC-P-1"),
		CustomerID: types.StringValue("SUB1"),
		Name:       types.StringValue("Escalation"),
		Notifications: []NotificationModel{
			notif("A-1", 0, "All"),
			notif("B-2", 15, "Days"),
		},
	}

	resp := &resource.ReadResponse{State: testState(t, ctx, &prior)}
	r.Read(ctx, resource.ReadRequest{State: testState(t, ctx, &prior)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	state := decodeState(t, ctx, resp.State)
	if state.Name.ValueString() != "Renamed" {
		t.Fatalf("expected the API name, got %q", state.Name.ValueString())
	}
	if !reflect.DeepEqual(contactIDs(state.Notifications), []string{"A-1", "B-2"}) {
		t.Fatalf("expected the prior ordering to be preserved, got %v", contactIDs(state.Notifications))
	}
}

func TestReadRemovesResourceOnNotFound(t *testing.T) {
	ctx := context.Background()

	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	})

	prior := NotificationProfileResourceModel{
		ID:         types.StringValue("ACC-P-1"),
		CustomerID: types.StringValue("SUB1"),
		Name:       types.StringValue("Escalation"),
	}

	resp := &resource.ReadResponse{State: testState(t, ctx, &prior)}
	r.Read(ctx, resource.ReadRequest{State: testState(t, ctx, &prior)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected the resource to be removed from state")
	}
}

func TestReadReportsAPIError(t *testing.T) {
	ctx := context.Background()

	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"denied"}`))
	})

	prior := NotificationProfileResourceModel{
		ID:         types.StringValue("ACC-P-1"),
		CustomerID: types.StringNull(),
		Name:       types.StringValue("Escalation"),
	}

	resp := &resource.ReadResponse{State: testState(t, ctx, &prior)}
	r.Read(ctx, resource.ReadRequest{State: testState(t, ctx, &prior)}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic")
	}
}

func TestUpdateSendsCompleteNotificationArray(t *testing.T) {
	ctx := context.Background()

	var body map[string]interface{}
	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
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
		_, _ = w.Write(profileJSON(t, "ACC-P-1", "SUB1", "Escalation", []map[string]interface{}{
			apiNotif("A-1", 0, "All"),
		}))
	})

	state := NotificationProfileResourceModel{
		ID:         types.StringValue("ACC-P-1"),
		CustomerID: types.StringValue("SUB1"),
		Name:       types.StringValue("Escalation"),
		Notifications: []NotificationModel{
			notif("A-1", 0, "All"),
			notif("B-2", 15, "Days"),
		},
	}
	plan := state
	plan.Notifications = []NotificationModel{notif("A-1", 0, "All")}

	resp := &resource.UpdateResponse{State: testState(t, ctx, &state)}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  testPlan(t, ctx, plan),
		State: testState(t, ctx, &state),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if body["id"] != "ACC-P-1" {
		t.Fatalf("expected the profile ID in the PUT body, got %v", body["id"])
	}
	notifications, ok := body["notifications"].([]interface{})
	if !ok || len(notifications) != 1 {
		t.Fatalf("expected the full replacement array, got %#v", body["notifications"])
	}

	got := decodeState(t, ctx, resp.State)
	if len(got.Notifications) != 1 {
		t.Fatalf("expected 1 notification in state, got %d", len(got.Notifications))
	}
}

func TestUpdateClearsNotificationsWithAnEmptyArray(t *testing.T) {
	ctx := context.Background()

	var body map[string]interface{}
	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		raw, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}
		_, _ = w.Write(profileJSON(t, "ACC-P-1", "SUB1", "Escalation", nil))
	})

	state := NotificationProfileResourceModel{
		ID:            types.StringValue("ACC-P-1"),
		CustomerID:    types.StringValue("SUB1"),
		Name:          types.StringValue("Escalation"),
		Notifications: []NotificationModel{notif("A-1", 0, "All")},
	}
	plan := state
	plan.Notifications = nil

	resp := &resource.UpdateResponse{State: testState(t, ctx, &state)}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  testPlan(t, ctx, plan),
		State: testState(t, ctx, &state),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	notifications, ok := body["notifications"].([]interface{})
	if !ok {
		t.Fatalf("expected notifications to be an array, got %#v", body["notifications"])
	}
	if len(notifications) != 0 {
		t.Fatalf("expected an empty array, got %#v", notifications)
	}
}

func TestUpdateReportsAPIError(t *testing.T) {
	ctx := context.Background()

	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	})

	state := NotificationProfileResourceModel{
		ID:         types.StringValue("ACC-P-1"),
		CustomerID: types.StringValue("SUB1"),
		Name:       types.StringValue("Escalation"),
	}
	plan := state
	plan.Name = types.StringValue("New Name")

	resp := &resource.UpdateResponse{State: testState(t, ctx, &state)}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  testPlan(t, ctx, plan),
		State: testState(t, ctx, &state),
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic")
	}
}

func TestDeleteSucceeds(t *testing.T) {
	ctx := context.Background()

	called := false
	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		called = true
		if req.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", req.Method)
		}
		_, _ = w.Write([]byte(`{"ok":true,"id":"ACC-P-1"}`))
	})

	state := NotificationProfileResourceModel{
		ID:         types.StringValue("ACC-P-1"),
		CustomerID: types.StringValue("SUB1"),
		Name:       types.StringValue("Escalation"),
	}

	resp := &resource.DeleteResponse{State: testState(t, ctx, &state)}
	r.Delete(ctx, resource.DeleteRequest{State: testState(t, ctx, &state)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if !called {
		t.Fatal("expected the delete request to be issued")
	}
}

func TestDeleteIsIdempotentOnNotFound(t *testing.T) {
	ctx := context.Background()

	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	})

	state := NotificationProfileResourceModel{
		ID:         types.StringValue("ACC-P-1"),
		CustomerID: types.StringValue("SUB1"),
		Name:       types.StringValue("Escalation"),
	}

	resp := &resource.DeleteResponse{State: testState(t, ctx, &state)}
	r.Delete(ctx, resource.DeleteRequest{State: testState(t, ctx, &state)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("deleting an already absent profile must succeed: %v", resp.Diagnostics)
	}
}

func TestDeleteReportsAPIError(t *testing.T) {
	ctx := context.Background()

	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"denied"}`))
	})

	state := NotificationProfileResourceModel{
		ID:         types.StringValue("ACC-P-1"),
		CustomerID: types.StringValue("SUB1"),
		Name:       types.StringValue("Escalation"),
	}

	resp := &resource.DeleteResponse{State: testState(t, ctx, &state)}
	r.Delete(ctx, resource.DeleteRequest{State: testState(t, ctx, &state)}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic")
	}
}

// ---------------------------------------------------------------------------
// import
// ---------------------------------------------------------------------------

func TestImportStateAcceptsBareProfileID(t *testing.T) {
	ctx := context.Background()

	var query string
	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		query = req.URL.Query().Get("customerid")
		_, _ = w.Write(profileJSON(t, "ACC-P-1", "ROOT", "Escalation", []map[string]interface{}{
			apiNotif("A-1", 0, "All"),
		}))
	})

	resp := &resource.ImportStateResponse{State: testState(t, ctx, nil)}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "ACC-P-1"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if query != "" {
		t.Fatalf("expected an unscoped request, got customerid=%q", query)
	}

	state := decodeState(t, ctx, resp.State)
	if state.ID.ValueString() != "ACC-P-1" {
		t.Fatalf("unexpected ID: %q", state.ID.ValueString())
	}
	// The API's customer_id is persisted so later requests stay on the right
	// account.
	if state.CustomerID.ValueString() != "ROOT" {
		t.Fatalf("expected customer_id ROOT, got %q", state.CustomerID.ValueString())
	}
	if len(state.Notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(state.Notifications))
	}
}

func TestImportStateAcceptsCustomerScopedID(t *testing.T) {
	ctx := context.Background()

	var query string
	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		query = req.URL.Query().Get("customerid")
		_, _ = w.Write(profileJSON(t, "ACC-P-1", "", "Escalation", nil))
	})

	resp := &resource.ImportStateResponse{State: testState(t, ctx, nil)}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "SUB1:ACC-P-1"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if query != "SUB1" {
		t.Fatalf("expected the lookup to be scoped to SUB1, got %q", query)
	}

	state := decodeState(t, ctx, resp.State)
	if state.CustomerID.ValueString() != "SUB1" {
		t.Fatalf("expected the supplied customer_id to be persisted, got %q", state.CustomerID.ValueString())
	}
}

func TestImportStateRejectsMalformedIDs(t *testing.T) {
	ctx := context.Background()

	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		t.Error("no request should be issued for a malformed import ID")
	})

	for _, id := range []string{"", "  ", "a:b:c", "SUB1:"} {
		resp := &resource.ImportStateResponse{State: testState(t, ctx, nil)}
		r.ImportState(ctx, resource.ImportStateRequest{ID: id}, resp)
		if !resp.Diagnostics.HasError() {
			t.Fatalf("expected import ID %q to be rejected", id)
		}
	}
}

func TestImportStateReportsAPIError(t *testing.T) {
	ctx := context.Background()

	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	})

	resp := &resource.ImportStateResponse{State: testState(t, ctx, nil)}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "ACC-P-MISSING"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic")
	}
}

// ---------------------------------------------------------------------------
// mapping entry points
// ---------------------------------------------------------------------------

func TestMapForReadKeepsPriorCustomerIDWhenAPIOmitsIt(t *testing.T) {
	prior := &NotificationProfileResourceModel{
		CustomerID: types.StringValue("SUB1"),
	}
	got := mapForRead(&client.NotificationProfile{ID: "ACC-P-1", Name: "n"}, prior)
	if got.CustomerID.ValueString() != "SUB1" {
		t.Fatalf("expected SUB1, got %q", got.CustomerID.ValueString())
	}
}

func TestMapForApplyPreservesPlannedValues(t *testing.T) {
	plan := &NotificationProfileResourceModel{
		ID:            types.StringUnknown(),
		CustomerID:    types.StringValue("SUB1"),
		Name:          types.StringValue("Planned Name"),
		Notifications: []NotificationModel{notif("A-1", 0, "All")},
	}
	profile := &client.NotificationProfile{
		ID:         "ACC-P-1",
		CustomerID: "OTHER",
		Name:       "Server Name",
		Notifications: []map[string]interface{}{
			apiNotif("A-1", 0, "All"),
		},
	}

	got := mapForApply(profile, plan)
	if got.ID.ValueString() != "ACC-P-1" {
		t.Fatalf("expected the unknown ID to be filled, got %q", got.ID.ValueString())
	}
	if got.CustomerID.ValueString() != "SUB1" {
		t.Fatalf("planned customer_id must win, got %q", got.CustomerID.ValueString())
	}
	if got.Name.ValueString() != "Planned Name" {
		t.Fatalf("planned name must win, got %q", got.Name.ValueString())
	}
}
