package schedule

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func newTestResource(t *testing.T, handler http.HandlerFunc) *ScheduleResource {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &ScheduleResource{client: client.NewClient(client.ClientConfig{
		APIToken:     "test-token",
		BaseURL:      srv.URL,
		RateLimit:    1000,
		MaxRetries:   1,
		RetryMinWait: time.Millisecond,
		RetryMaxWait: time.Millisecond,
		Timeout:      5 * time.Second,
	})}
}

func newPlan(t *testing.T, ctx context.Context, model ScheduleResourceModel) tfsdk.Plan {
	t.Helper()
	s := ScheduleSchema()
	p := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if diags := p.Set(ctx, &model); diags.HasError() {
		t.Fatalf("failed to build plan: %v", diags)
	}
	return p
}

func newState(t *testing.T, ctx context.Context, model *ScheduleResourceModel) tfsdk.State {
	t.Helper()
	s := ScheduleSchema()
	st := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if model != nil {
		if diags := st.Set(ctx, model); diags.HasError() {
			t.Fatalf("failed to build state: %v", diags)
		}
	}
	return st
}

func decodeState(t *testing.T, ctx context.Context, st tfsdk.State) ScheduleResourceModel {
	t.Helper()
	var out ScheduleResourceModel
	if diags := st.Get(ctx, &out); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	return out
}

func span(time1, time2 string) *DayModel {
	return &DayModel{
		Time1:    types.StringValue(time1),
		Time2:    types.StringValue(time2),
		Exclude:  types.BoolValue(false),
		Disabled: types.BoolValue(false),
		AllDay:   types.BoolValue(false),
	}
}

func offDay() *DayModel {
	return &DayModel{
		Time1:    types.StringNull(),
		Time2:    types.StringNull(),
		Exclude:  types.BoolValue(false),
		Disabled: types.BoolValue(true),
		AllDay:   types.BoolValue(false),
	}
}

func businessHoursPlan() ScheduleResourceModel {
	return ScheduleResourceModel{
		ID:         types.StringValue("BusinessHours"),
		CustomerID: types.StringNull(),
		Monday:     span("8:00", "18:00"),
		Friday:     span("8:00", "17:00"),
		Saturday:   offDay(),
	}
}

// ---------------------------------------------------------------------------
// day mapping
// ---------------------------------------------------------------------------

func TestDaysFromModel_OmitsUnconfiguredDays(t *testing.T) {
	plan := businessHoursPlan()

	data := daysFromModel(&plan)
	if len(data) != 3 {
		t.Fatalf("expected 3 days, got %d: %v", len(data), data)
	}
	if data["monday"].Time1 != "8:00" || data["monday"].Time2 != "18:00" {
		t.Fatalf("monday mapped as %+v", data["monday"])
	}
	if !data["saturday"].Disabled.Bool() {
		t.Fatalf("saturday should be disabled, got %+v", data["saturday"])
	}
	if _, ok := data["sunday"]; ok {
		t.Fatal("sunday was not configured and must not be sent")
	}
}

func TestDaysFromModel_ReturnsNilWhenNothingIsConfigured(t *testing.T) {
	empty := ScheduleResourceModel{ID: types.StringValue("Empty")}
	if got := daysFromModel(&empty); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestMapDaysForRead_ReplacesStateWithWhatTheAPIReports(t *testing.T) {
	state := businessHoursPlan()

	// The API no longer knows about Friday or Saturday, and Monday moved.
	mapDaysForRead(&state, map[string]client.ScheduleDay{
		"monday": {Time1: "9:00", Time2: "17:00", Exclude: true},
	})

	if state.Monday.Time1.ValueString() != "9:00" || !state.Monday.Exclude.ValueBool() {
		t.Fatalf("monday not refreshed: %+v", state.Monday)
	}
	if state.Friday != nil || state.Saturday != nil {
		t.Fatal("refresh must drop days the API no longer reports")
	}
}

func TestMapDaysForApply_KeepsPlannedValuesWhenTheAPIEchoesNothing(t *testing.T) {
	plan := businessHoursPlan()
	plan.Monday.Time2 = types.StringUnknown()

	mapDaysForApply(&plan, nil)

	if plan.Monday.Time1.ValueString() != "8:00" {
		t.Fatalf("planned time1 lost: %+v", plan.Monday)
	}
	if plan.Monday.Time2.IsUnknown() {
		t.Fatal("an unknown value must never reach state")
	}
	if plan.Friday == nil || plan.Saturday == nil {
		t.Fatal("planned days must survive a bare acknowledgement")
	}
}

func TestMapDaysForApply_PrefersTheAPIValueButFallsBackToThePlan(t *testing.T) {
	plan := businessHoursPlan()

	mapDaysForApply(&plan, map[string]client.ScheduleDay{
		// The API normalised the span away and answered with allday instead.
		"monday": {AllDay: true},
		"friday": {Time1: "8:00", Time2: "17:00"},
	})

	if !plan.Monday.AllDay.ValueBool() {
		t.Fatalf("the API value must win for booleans: %+v", plan.Monday)
	}
	if plan.Monday.Time1.ValueString() != "8:00" {
		t.Fatalf("an omitted time must fall back to the plan: %+v", plan.Monday)
	}
	if plan.Friday.Time2.ValueString() != "17:00" {
		t.Fatalf("friday mapped as %+v", plan.Friday)
	}
	// Saturday was planned but not echoed; the plan is preserved so the apply
	// stays consistent.
	if plan.Saturday == nil || !plan.Saturday.Disabled.ValueBool() {
		t.Fatalf("saturday should have kept the planned value: %+v", plan.Saturday)
	}
	if plan.Sunday != nil {
		t.Fatal("sunday was never configured")
	}
}

func TestResolveCustomerID_PrefersConfigurationThenAPIThenNull(t *testing.T) {
	if got := resolveCustomerID(types.StringValue("CFG"), "API"); got.ValueString() != "CFG" {
		t.Fatalf("configuration must win, got %v", got)
	}
	if got := resolveCustomerID(types.StringUnknown(), "API"); got.ValueString() != "API" {
		t.Fatalf("expected the API value, got %v", got)
	}
	if got := resolveCustomerID(types.StringNull(), ""); !got.IsNull() {
		t.Fatalf("expected null, got %v", got)
	}
	if got := resolveCustomerID(types.StringValue(""), ""); !got.IsNull() {
		t.Fatalf("an empty string must not be treated as configured, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// Create / Update
// ---------------------------------------------------------------------------

func TestCreate_IssuesAPutAndWritesState(t *testing.T) {
	ctx := context.Background()

	var method, path string
	var body map[string]interface{}

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		method = req.Method
		path = req.URL.Path
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode body: %v", err)
		}
		if _, err := io.WriteString(w, `{"id":"BusinessHours","data":{"monday":{"time1":"8:00","time2":"18:00"},"friday":{"time1":"8:00","time2":"17:00"},"saturday":{"disabled":1}}}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})

	resp := &resource.CreateResponse{State: newState(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{Plan: newPlan(t, ctx, businessHoursPlan())}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}

	// The schedules endpoint has no POST; create must be a PUT.
	if method != http.MethodPut {
		t.Fatalf("expected PUT, got %s", method)
	}
	if path != "/schedules/BusinessHours" {
		t.Fatalf("unexpected path %q", path)
	}
	if body["id"] != "BusinessHours" {
		t.Fatalf("the caller-chosen ID must be sent, got %v", body["id"])
	}

	got := decodeState(t, ctx, resp.State)
	if got.ID.ValueString() != "BusinessHours" {
		t.Fatalf("unexpected id %v", got.ID)
	}
	if !got.CustomerID.IsNull() {
		t.Fatalf("customer_id should be null, got %v", got.CustomerID)
	}
	if got.Monday.Time1.ValueString() != "8:00" || !got.Saturday.Disabled.ValueBool() {
		t.Fatalf("unexpected state: monday=%+v saturday=%+v", got.Monday, got.Saturday)
	}
	if got.Sunday != nil {
		t.Fatal("sunday must stay unset")
	}
}

func TestCreate_ScopesTheRequestToTheSubAccount(t *testing.T) {
	ctx := context.Background()

	var query string
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		query = req.URL.RawQuery
		if _, err := io.WriteString(w, `{"id":"BusinessHours","data":{"monday":{"time1":"8:00","time2":"18:00"}}}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})

	plan := businessHoursPlan()
	plan.CustomerID = types.StringValue("SUB1")

	resp := &resource.CreateResponse{State: newState(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{Plan: newPlan(t, ctx, plan)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}

	if !strings.Contains(query, "customerid=SUB1") {
		t.Fatalf("expected the SubAccount in the query, got %q", query)
	}
	if got := decodeState(t, ctx, resp.State); got.CustomerID.ValueString() != "SUB1" {
		t.Fatalf("customer_id must be persisted, got %v", got.CustomerID)
	}
}

func TestCreate_RejectsAScheduleWithNoDays(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		t.Error("the API must not be called for an empty schedule")
	})

	plan := ScheduleResourceModel{ID: types.StringValue("Empty"), CustomerID: types.StringNull()}
	resp := &resource.CreateResponse{State: newState(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{Plan: newPlan(t, ctx, plan)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic")
	}
	// Nothing was sent, so nothing exists in NodePing and nothing belongs in
	// state -- the partial-state write must not fire for a pre-flight rejection.
	if !resp.State.Raw.IsNull() {
		t.Fatalf("state must stay empty when no schedule was created, got %v", resp.State.Raw)
	}
}

func TestCreate_ReportsAPIFailures(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		if _, err := io.WriteString(w, `{"error":"bad schedule"}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})

	resp := &resource.CreateResponse{State: newState(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{Plan: newPlan(t, ctx, businessHoursPlan())}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "bad schedule") {
		t.Fatalf("expected the API message to surface: %v", resp.Diagnostics)
	}
}

func TestCreate_RecordsTheScheduleIDWhenTheResponseCannotBeDecoded(t *testing.T) {
	ctx := context.Background()

	// NodePing accepted the PUT, so the schedule exists, but the body it sent
	// back is not one the client can decode. This is how three schedules were
	// orphaned: the provider returned the error without writing state, and
	// OpenTofu could not destroy an object it had never recorded.
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		if _, err := io.WriteString(w, `{"data":"not-an-object"}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})

	resp := &resource.CreateResponse{State: newState(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{Plan: newPlan(t, ctx, businessHoursPlan())}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected the undecodable response to be reported")
	}

	got := decodeState(t, ctx, resp.State)
	if got.ID.ValueString() != "BusinessHours" {
		t.Fatalf("state must carry the schedule ID so the next plan can reconcile it, got %q", got.ID.ValueString())
	}
}

func TestCreate_RecordsTheConfiguredCustomerIDAlongsideTheScheduleID(t *testing.T) {
	ctx := context.Background()

	// Read has to scope its lookup to the same subaccount the create used, so
	// an explicitly configured customer_id belongs in the partial state too.
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		if _, err := io.WriteString(w, `{"data":"not-an-object"}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})

	plan := businessHoursPlan()
	plan.CustomerID = types.StringValue("SUBACC1")

	resp := &resource.CreateResponse{State: newState(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{Plan: newPlan(t, ctx, plan)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected the undecodable response to be reported")
	}

	got := decodeState(t, ctx, resp.State)
	if got.CustomerID.ValueString() != "SUBACC1" {
		t.Fatalf("state must carry the configured customer_id, got %q", got.CustomerID.ValueString())
	}
}

func TestCreate_IdentityStaysNullWhenCreateFails(t *testing.T) {
	ctx := context.Background()

	// The partial state deliberately leaves identity alone. customer_id is
	// often still unknown here, and an identity that disagrees with the one the
	// next Read derives is rejected by the framework as "Unexpected Identity
	// Change", which would block the recovery the partial state exists for.
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		if _, err := io.WriteString(w, `{"data":"not-an-object"}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})

	resp := &resource.CreateResponse{State: newState(t, ctx, nil), Identity: newIdentity(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{Plan: newPlan(t, ctx, businessHoursPlan())}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected the undecodable response to be reported")
	}
	if !resp.Identity.Raw.IsFullyNull() {
		t.Fatalf("identity must stay null on a failed create, got %v", resp.Identity.Raw)
	}
}

func TestUpdate_IssuesTheSamePutAndKeepsTheCustomerID(t *testing.T) {
	ctx := context.Background()

	var method string
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		method = req.Method
		if _, err := io.WriteString(w, `{"id":"BusinessHours","data":{"monday":{"time1":"9:00","time2":"17:00"}}}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})

	state := businessHoursPlan()
	state.CustomerID = types.StringValue("SUB1")

	plan := businessHoursPlan()
	plan.CustomerID = types.StringNull()
	plan.Monday = span("9:00", "17:00")
	plan.Friday = nil
	plan.Saturday = nil

	resp := &resource.UpdateResponse{State: newState(t, ctx, &state)}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  newPlan(t, ctx, plan),
		State: newState(t, ctx, &state),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", resp.Diagnostics)
	}

	if method != http.MethodPut {
		t.Fatalf("expected PUT, got %s", method)
	}

	got := decodeState(t, ctx, resp.State)
	if got.CustomerID.ValueString() != "SUB1" {
		t.Fatalf("customer_id must survive an update, got %v", got.CustomerID)
	}
	if got.Monday.Time1.ValueString() != "9:00" {
		t.Fatalf("monday not updated: %+v", got.Monday)
	}
	if got.Friday != nil {
		t.Fatal("friday was removed from the plan and must be gone from state")
	}
}

func TestUpdate_ReportsAPIFailures(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	state := businessHoursPlan()
	resp := &resource.UpdateResponse{State: newState(t, ctx, &state)}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  newPlan(t, ctx, businessHoursPlan()),
		State: newState(t, ctx, &state),
	}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic")
	}
}

// ---------------------------------------------------------------------------
// Read
// ---------------------------------------------------------------------------

func TestRead_RefreshesFromTheAPI(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		if _, err := io.WriteString(w, `{"id":"BusinessHours","data":{"monday":{"time1":"7:00","time2":"19:00","exclude":true}}}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})

	state := businessHoursPlan()
	resp := &resource.ReadResponse{State: newState(t, ctx, &state)}
	r.Read(ctx, resource.ReadRequest{State: newState(t, ctx, &state)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	got := decodeState(t, ctx, resp.State)
	if got.Monday.Time1.ValueString() != "7:00" || !got.Monday.Exclude.ValueBool() {
		t.Fatalf("monday not refreshed: %+v", got.Monday)
	}
	if got.Friday != nil || got.Saturday != nil {
		t.Fatal("days the API dropped must disappear from state")
	}
}

func TestRead_RemovesTheResourceWhenTheScheduleIsGone(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		if _, err := io.WriteString(w, `{"error":"not found"}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})

	state := businessHoursPlan()
	resp := &resource.ReadResponse{State: newState(t, ctx, &state)}
	r.Read(ctx, resource.ReadRequest{State: newState(t, ctx, &state)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("a missing schedule is not an error: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected the resource to be removed from state")
	}
}

func TestRead_ReportsRealFailures(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	state := businessHoursPlan()
	resp := &resource.ReadResponse{State: newState(t, ctx, &state)}
	r.Read(ctx, resource.ReadRequest{State: newState(t, ctx, &state)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic")
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("a server error must not silently destroy state")
	}
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func TestDelete_Succeeds(t *testing.T) {
	ctx := context.Background()

	var method, path, query string
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		method = req.Method
		path = req.URL.Path
		query = req.URL.RawQuery
		if _, err := io.WriteString(w, `{"ok":true}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})

	state := businessHoursPlan()
	state.CustomerID = types.StringValue("SUB1")

	resp := &resource.DeleteResponse{State: newState(t, ctx, &state)}
	r.Delete(ctx, resource.DeleteRequest{State: newState(t, ctx, &state)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("delete diagnostics: %v", resp.Diagnostics)
	}

	if method != http.MethodDelete || path != "/schedules/BusinessHours" {
		t.Fatalf("unexpected request %s %s", method, path)
	}
	if !strings.Contains(query, "customerid=SUB1") {
		t.Fatalf("delete must stay scoped to the SubAccount, got %q", query)
	}
}

func TestDelete_IsIdempotent(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		if _, err := io.WriteString(w, `{"error":"not found"}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})

	state := businessHoursPlan()
	resp := &resource.DeleteResponse{State: newState(t, ctx, &state)}
	r.Delete(ctx, resource.DeleteRequest{State: newState(t, ctx, &state)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("deleting an absent schedule must succeed: %v", resp.Diagnostics)
	}
}

func TestDelete_ReportsRealFailures(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	state := businessHoursPlan()
	resp := &resource.DeleteResponse{State: newState(t, ctx, &state)}
	r.Delete(ctx, resource.DeleteRequest{State: newState(t, ctx, &state)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic")
	}
}

// ---------------------------------------------------------------------------
// ImportState
// ---------------------------------------------------------------------------

func TestImportState_AcceptsBothIDForms(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name       string
		importID   string
		wantQuery  string
		wantCustID string
	}{
		{"schedule id only", "BusinessHours", "", "CUSTFROMAPI"},
		{"customer and schedule id", "SUB1:BusinessHours", "customerid=SUB1", "SUB1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var query string
			r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
				query = req.URL.RawQuery
				if _, err := io.WriteString(w, `{"id":"BusinessHours","customer_id":"CUSTFROMAPI","data":{"monday":{"time1":"8:00","time2":"18:00"}}}`); err != nil {
					t.Errorf("write failed: %v", err)
				}
			})

			resp := &resource.ImportStateResponse{State: newState(t, ctx, nil)}
			r.ImportState(ctx, resource.ImportStateRequest{ID: tc.importID}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("import diagnostics: %v", resp.Diagnostics)
			}

			if tc.wantQuery != "" && !strings.Contains(query, tc.wantQuery) {
				t.Fatalf("expected %q in the query, got %q", tc.wantQuery, query)
			}

			got := decodeState(t, ctx, resp.State)
			if got.ID.ValueString() != "BusinessHours" {
				t.Fatalf("unexpected id %v", got.ID)
			}
			// Without a persisted customer_id every later call targets the
			// root account and the schedule vanishes on the next refresh.
			if got.CustomerID.ValueString() != tc.wantCustID {
				t.Fatalf("expected customer_id %q, got %v", tc.wantCustID, got.CustomerID)
			}
			if got.Monday.Time2.ValueString() != "18:00" {
				t.Fatalf("monday not imported: %+v", got.Monday)
			}
		})
	}
}

func TestImportState_RejectsMalformedIDs(t *testing.T) {
	ctx := context.Background()

	for _, importID := range []string{"", "a:b:c", ":BusinessHours"} {
		r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
			t.Error("the API must not be called for a malformed import ID")
		})

		resp := &resource.ImportStateResponse{State: newState(t, ctx, nil)}
		r.ImportState(ctx, resource.ImportStateRequest{ID: importID}, resp)

		if !resp.Diagnostics.HasError() {
			t.Fatalf("expected a diagnostic for import ID %q", importID)
		}
	}
}

func TestImportState_ReportsAPIFailures(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	resp := &resource.ImportStateResponse{State: newState(t, ctx, nil)}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "Missing"}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic")
	}
}

// ---------------------------------------------------------------------------
// wiring
// ---------------------------------------------------------------------------

func TestMetadataAndSchema(t *testing.T) {
	ctx := context.Background()
	r := &ScheduleResource{}

	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "nodeping"}, metaResp)
	if metaResp.TypeName != "nodeping_schedule" {
		t.Fatalf("unexpected type name %q", metaResp.TypeName)
	}

	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", schemaResp.Diagnostics)
	}
	for _, day := range client.ScheduleDayNames {
		if _, ok := schemaResp.Schema.Attributes[day]; !ok {
			t.Fatalf("schema is missing %q", day)
		}
	}
}

func TestConfigure(t *testing.T) {
	ctx := context.Background()

	r := &ScheduleResource{}
	r.Configure(ctx, resource.ConfigureRequest{}, &resource.ConfigureResponse{})
	if r.client != nil {
		t.Fatal("a nil ProviderData must leave the resource unconfigured")
	}

	badResp := &resource.ConfigureResponse{}
	r.Configure(ctx, resource.ConfigureRequest{ProviderData: "not a client"}, badResp)
	if !badResp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic for the wrong ProviderData type")
	}

	c := client.NewClient(client.ClientConfig{APIToken: "t"})
	okResp := &resource.ConfigureResponse{}
	r.Configure(ctx, resource.ConfigureRequest{ProviderData: c}, okResp)
	if okResp.Diagnostics.HasError() || r.client != c {
		t.Fatalf("expected the client to be stored: %v", okResp.Diagnostics)
	}
}

func TestClientFor_OnlyScopesWhenACustomerIDIsSet(t *testing.T) {
	c := client.NewClient(client.ClientConfig{APIToken: "t"})
	r := &ScheduleResource{client: c}

	for _, value := range []types.String{types.StringNull(), types.StringUnknown(), types.StringValue("")} {
		if r.clientFor(value) != c {
			t.Fatalf("expected the provider client for %v", value)
		}
	}
	if r.clientFor(types.StringValue("SUB1")) == c {
		t.Fatal("expected a scoped client")
	}
}

func TestTimePattern(t *testing.T) {
	for _, good := range []string{"0:00", "00:00", "6:00", "18:30", "23:59"} {
		if !timePattern.MatchString(good) {
			t.Fatalf("%q should be valid", good)
		}
	}
	for _, bad := range []string{"", "6", "6:0", "24:00", "6:60", "-1:00", "06:00pm"} {
		if timePattern.MatchString(bad) {
			t.Fatalf("%q should be invalid", bad)
		}
	}
}
