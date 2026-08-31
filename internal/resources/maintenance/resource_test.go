package maintenance

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/client"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newTestResource(t *testing.T, handler http.HandlerFunc) *MaintenanceResource {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &MaintenanceResource{client: client.NewClient(client.ClientConfig{
		APIToken:     "test-token",
		BaseURL:      srv.URL,
		RateLimit:    1000,
		MaxRetries:   1,
		RetryMinWait: time.Millisecond,
		RetryMaxWait: time.Millisecond,
		Timeout:      5 * time.Second,
	})}
}

func checklistValue(t *testing.T, ids ...string) types.Set {
	t.Helper()
	set, diags := types.SetValueFrom(context.Background(), types.StringType, ids)
	if diags.HasError() {
		t.Fatalf("building checklist: %v", diags)
	}
	return set
}

func testPlan(t *testing.T, ctx context.Context, model MaintenanceResourceModel) tfsdk.Plan {
	t.Helper()
	s := MaintenanceSchema()
	p := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if diags := p.Set(ctx, &model); diags.HasError() {
		t.Fatalf("failed to build plan: %v", diags)
	}
	return p
}

func testState(t *testing.T, ctx context.Context, model *MaintenanceResourceModel) tfsdk.State {
	t.Helper()
	s := MaintenanceSchema()
	st := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if model != nil {
		if diags := st.Set(ctx, model); diags.HasError() {
			t.Fatalf("failed to build state: %v", diags)
		}
	}
	return st
}

func decodeState(t *testing.T, ctx context.Context, st tfsdk.State) MaintenanceResourceModel {
	t.Helper()
	var out MaintenanceResourceModel
	if diags := st.Get(ctx, &out); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	return out
}

func plannedModel(t *testing.T) MaintenanceResourceModel {
	t.Helper()
	return MaintenanceResourceModel{
		ID:         types.StringUnknown(),
		CustomerID: types.StringNull(),
		Name:       types.StringValue("Weekly patching"),
		Enabled:    types.BoolValue(true),
		Cron:       types.StringValue("0 3 * * 0"),
		Duration:   types.Int64Value(60),
		Checklist:  checklistValue(t, "C2", "C1"),
		Processing: types.BoolUnknown(),
		NextRun:    types.Int64Unknown(),
	}
}

func maintenanceBody(t *testing.T, m client.Maintenance) []byte {
	t.Helper()
	body, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return body
}

func checklistStrings(t *testing.T, set types.Set) []string {
	t.Helper()
	var out []string
	if diags := set.ElementsAs(context.Background(), &out, false); diags.HasError() {
		t.Fatalf("reading checklist: %v", diags)
	}
	return out
}

// ---------------------------------------------------------------------------
// metadata, schema, configure
// ---------------------------------------------------------------------------

func TestResourceMetadataAndSchema(t *testing.T) {
	ctx := context.Background()
	r := NewMaintenanceResource().(*MaintenanceResource)

	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "nodeping"}, metaResp)
	if metaResp.TypeName != "nodeping_maintenance" {
		t.Fatalf("type name = %q", metaResp.TypeName)
	}

	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", schemaResp.Diagnostics)
	}
	for _, name := range []string{"id", "customer_id", "name", "enabled", "cron", "duration", "checklist", "processing", "nextrun"} {
		if _, ok := schemaResp.Schema.Attributes[name]; !ok {
			t.Errorf("schema is missing attribute %q", name)
		}
	}
	if _, ok := schemaResp.Schema.Attributes["history"]; ok {
		t.Error("history must not be a resource attribute: it churns on every run and would produce noise")
	}
}

func TestResourceConfigure(t *testing.T) {
	ctx := context.Background()
	r := NewMaintenanceResource().(*MaintenanceResource)

	resp := &resource.ConfigureResponse{}
	r.Configure(ctx, resource.ConfigureRequest{}, resp)
	if r.client != nil || resp.Diagnostics.HasError() {
		t.Fatal("nil provider data must be a no-op")
	}

	resp = &resource.ConfigureResponse{}
	r.Configure(ctx, resource.ConfigureRequest{ProviderData: "not a client"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for the wrong provider data type")
	}

	c := client.NewClient(client.ClientConfig{APIToken: "t"})
	resp = &resource.ConfigureResponse{}
	r.Configure(ctx, resource.ConfigureRequest{ProviderData: c}, resp)
	if resp.Diagnostics.HasError() || r.client != c {
		t.Fatal("expected the client to be configured")
	}
}

func TestClientForScopesToCustomerID(t *testing.T) {
	base := client.NewClient(client.ClientConfig{APIToken: "t"})
	r := &MaintenanceResource{client: base}

	if r.clientFor(types.StringNull()) != base {
		t.Error("a null customer_id must use the provider-level client")
	}
	if r.clientFor(types.StringUnknown()) != base {
		t.Error("an unknown customer_id must use the provider-level client")
	}
	if r.clientFor(types.StringValue("")) != base {
		t.Error("an empty customer_id must use the provider-level client")
	}
	if r.clientFor(types.StringValue("SUB1")) == base {
		t.Error("a set customer_id must produce a scoped client")
	}
}

// ---------------------------------------------------------------------------
// create
// ---------------------------------------------------------------------------

func TestCreateSendsSortedChecklistAndStoresResult(t *testing.T) {
	ctx := context.Background()

	var body map[string]interface{}
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			t.Errorf("unexpected method %s", req.Method)
		}
		raw, _ := io.ReadAll(req.Body)
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("bad body: %v", err)
		}
		nextRun := int64(1700000000000)
		_, _ = w.Write(maintenanceBody(t, client.Maintenance{
			ID: "9VKOB", Name: "Weekly patching", Enabled: true, Cron: "0 3 * * 0",
			Duration: 60, Checklist: []string{"C2", "C1"}, NextRun: &nextRun,
		}))
	})

	resp := &resource.CreateResponse{State: testState(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{Plan: testPlan(t, ctx, plannedModel(t))}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}

	list, ok := body["checklist"].([]interface{})
	if !ok || len(list) != 2 || list[0] != "C1" || list[1] != "C2" {
		t.Fatalf("checklist was not sorted: %#v", body["checklist"])
	}
	if body["cron"] != "0 3 * * 0" {
		t.Fatalf("cron was not sent: %#v", body["cron"])
	}

	got := decodeState(t, ctx, resp.State)
	if got.ID.ValueString() != "9VKOB" {
		t.Fatalf("id = %q", got.ID.ValueString())
	}
	if got.NextRun.ValueInt64() != 1700000000000 {
		t.Fatalf("nextrun = %v", got.NextRun)
	}
	if got.Processing.ValueBool() {
		t.Fatal("processing should be false")
	}
	if ids := checklistStrings(t, got.Checklist); len(ids) != 2 {
		t.Fatalf("checklist = %v", ids)
	}
}

func TestCreateSendsEnabledFalseExplicitly(t *testing.T) {
	ctx := context.Background()

	var body map[string]interface{}
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		raw, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = w.Write(maintenanceBody(t, client.Maintenance{
			ID: "9VKOB", Name: "Weekly patching", Enabled: false, Cron: "0 3 * * 0",
			Duration: 60, Checklist: []string{"C1", "C2"},
		}))
	})

	plan := plannedModel(t)
	plan.Enabled = types.BoolValue(false)

	resp := &resource.CreateResponse{State: testState(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{Plan: testPlan(t, ctx, plan)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}

	// Omitting `enabled` when it is false would mean a disable never reaches
	// the API and drifts forever.
	if v, ok := body["enabled"].(bool); !ok || v {
		t.Fatalf("enabled was not sent as false: %#v", body["enabled"])
	}
	if decodeState(t, ctx, resp.State).Enabled.ValueBool() {
		t.Fatal("enabled should be false in state")
	}
}

func TestCreateScopesToCustomerIDAndPersistsIt(t *testing.T) {
	ctx := context.Background()

	var seen string
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		seen = req.URL.Query().Get("customerid")
		_, _ = w.Write(maintenanceBody(t, client.Maintenance{
			ID: "9VKOB", CustomerID: "SUB1", Name: "Weekly patching", Enabled: true,
			Cron: "0 3 * * 0", Duration: 60, Checklist: []string{"C1", "C2"},
		}))
	})

	plan := plannedModel(t)
	plan.CustomerID = types.StringValue("SUB1")

	resp := &resource.CreateResponse{State: testState(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{Plan: testPlan(t, ctx, plan)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}
	if seen != "SUB1" {
		t.Fatalf("customerid query = %q", seen)
	}
	if decodeState(t, ctx, resp.State).CustomerID.ValueString() != "SUB1" {
		t.Fatal("customer_id was not persisted")
	}
}

func TestCreateSurfacesAPIError(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad checklist"}`))
	})

	resp := &resource.CreateResponse{State: testState(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{Plan: testPlan(t, ctx, plannedModel(t))}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic")
	}
}

func TestCreateRefusesAnAdHocResponse(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"id":"ad-hoc","name":"Weekly patching","duration":60}`))
	})

	resp := &resource.CreateResponse{State: testState(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{Plan: testPlan(t, ctx, plannedModel(t))}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("an ad-hoc record must never be written to state")
	}
}

// ---------------------------------------------------------------------------
// read
// ---------------------------------------------------------------------------

func createdState(t *testing.T) *MaintenanceResourceModel {
	t.Helper()
	return &MaintenanceResourceModel{
		ID:         types.StringValue("9VKOB"),
		CustomerID: types.StringNull(),
		Name:       types.StringValue("Weekly patching"),
		Enabled:    types.BoolValue(true),
		Cron:       types.StringValue("0 3 * * 0"),
		Duration:   types.Int64Value(60),
		Checklist:  checklistValue(t, "C1", "C2"),
		Processing: types.BoolValue(false),
		NextRun:    types.Int64Null(),
	}
}

func TestReadRefreshesState(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		nextRun := int64(1700000000000)
		_, _ = w.Write(maintenanceBody(t, client.Maintenance{
			ID: "9VKOB", Name: "Renamed", Enabled: false, Cron: "0 4 * * 0",
			Duration: 90, Checklist: []string{"C3"}, Processing: true, NextRun: &nextRun,
		}))
	})

	resp := &resource.ReadResponse{State: testState(t, ctx, createdState(t))}
	r.Read(ctx, resource.ReadRequest{State: testState(t, ctx, createdState(t))}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	got := decodeState(t, ctx, resp.State)
	if got.Name.ValueString() != "Renamed" || got.Duration.ValueInt64() != 90 ||
		got.Enabled.ValueBool() || !got.Processing.ValueBool() {
		t.Fatalf("state was not refreshed: %+v", got)
	}
	if ids := checklistStrings(t, got.Checklist); len(ids) != 1 || ids[0] != "C3" {
		t.Fatalf("checklist = %v", ids)
	}
}

// TestReadRemovesDisappearedResource covers the case that makes ad-hoc
// maintenance unmanageable: the record is gone from the API. For a scheduled
// window this can only mean an out-of-band delete, so removing it from state is
// correct. Ad-hoc windows delete themselves, which is why they are rejected at
// create and import time rather than handled here.
func TestReadRemovesDisappearedResource(t *testing.T) {
	ctx := context.Background()

	for name, handler := range map[string]http.HandlerFunc{
		"404 response": func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
		},
		"empty 200 body": func(w http.ResponseWriter, req *http.Request) {
			_, _ = w.Write([]byte(`{}`))
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := newTestResource(t, handler)

			resp := &resource.ReadResponse{State: testState(t, ctx, createdState(t))}
			r.Read(ctx, resource.ReadRequest{State: testState(t, ctx, createdState(t))}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("a disappeared resource must not be an error: %v", resp.Diagnostics)
			}
			if !resp.State.Raw.IsNull() {
				t.Fatal("expected the resource to be removed from state")
			}
		})
	}
}

func TestReadSurfacesAPIError(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	resp := &resource.ReadResponse{State: testState(t, ctx, createdState(t))}
	r.Read(ctx, resource.ReadRequest{State: testState(t, ctx, createdState(t))}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic")
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("a transient failure must not remove the resource from state")
	}
}

// ---------------------------------------------------------------------------
// update
// ---------------------------------------------------------------------------

func TestUpdateSendsFullDesiredValue(t *testing.T) {
	ctx := context.Background()

	var body map[string]interface{}
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPut || req.URL.Path != "/maintenance/9VKOB" {
			t.Errorf("unexpected request %s %s", req.Method, req.URL.Path)
		}
		raw, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = w.Write(maintenanceBody(t, client.Maintenance{
			ID: "9VKOB", Name: "Renamed", Enabled: true, Cron: "0 5 * * 1",
			Duration: 120, Checklist: []string{"C1", "C9"},
		}))
	})

	plan := plannedModel(t)
	plan.ID = types.StringValue("9VKOB")
	plan.Name = types.StringValue("Renamed")
	plan.Cron = types.StringValue("0 5 * * 1")
	plan.Duration = types.Int64Value(120)
	plan.Checklist = checklistValue(t, "C9", "C1")

	resp := &resource.UpdateResponse{State: testState(t, ctx, createdState(t))}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  testPlan(t, ctx, plan),
		State: testState(t, ctx, createdState(t)),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", resp.Diagnostics)
	}

	// duration and checklist are required by the API on every PUT.
	if body["duration"] != float64(120) {
		t.Fatalf("duration was not sent: %#v", body["duration"])
	}
	if list, ok := body["checklist"].([]interface{}); !ok || len(list) != 2 || list[0] != "C1" {
		t.Fatalf("checklist was not sent sorted: %#v", body["checklist"])
	}
	if body["id"] != "9VKOB" {
		t.Fatalf("id was not sent in the body: %#v", body["id"])
	}

	got := decodeState(t, ctx, resp.State)
	if got.Name.ValueString() != "Renamed" || got.Duration.ValueInt64() != 120 {
		t.Fatalf("state was not updated: %+v", got)
	}
}

func TestUpdateReportsDeletedWindowClearly(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	plan := plannedModel(t)
	plan.ID = types.StringValue("9VKOB")

	resp := &resource.UpdateResponse{State: testState(t, ctx, createdState(t))}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  testPlan(t, ctx, plan),
		State: testState(t, ctx, createdState(t)),
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic")
	}
	if resp.Diagnostics.Errors()[0].Summary() != "Maintenance Window No Longer Exists" {
		t.Fatalf("unexpected diagnostic: %v", resp.Diagnostics.Errors()[0])
	}
}

func TestUpdateSurfacesAPIError(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})

	plan := plannedModel(t)
	plan.ID = types.StringValue("9VKOB")

	resp := &resource.UpdateResponse{State: testState(t, ctx, createdState(t))}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  testPlan(t, ctx, plan),
		State: testState(t, ctx, createdState(t)),
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic")
	}
}

func TestUpdateUsesTheStateCustomerID(t *testing.T) {
	ctx := context.Background()

	var seen string
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		seen = req.URL.Query().Get("customerid")
		_, _ = w.Write(maintenanceBody(t, client.Maintenance{
			ID: "9VKOB", CustomerID: "SUB1", Name: "Weekly patching", Enabled: true,
			Cron: "0 3 * * 0", Duration: 60, Checklist: []string{"C1", "C2"},
		}))
	})

	state := createdState(t)
	state.CustomerID = types.StringValue("SUB1")

	plan := plannedModel(t)
	plan.ID = types.StringValue("9VKOB")
	plan.CustomerID = types.StringValue("SUB1")

	resp := &resource.UpdateResponse{State: testState(t, ctx, state)}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  testPlan(t, ctx, plan),
		State: testState(t, ctx, state),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", resp.Diagnostics)
	}
	if seen != "SUB1" {
		t.Fatalf("customerid query = %q", seen)
	}
}

// ---------------------------------------------------------------------------
// delete
// ---------------------------------------------------------------------------

func TestDeleteIsIdempotent(t *testing.T) {
	ctx := context.Background()

	for name, handler := range map[string]http.HandlerFunc{
		"ok": func(w http.ResponseWriter, req *http.Request) {
			if req.Method != http.MethodDelete {
				t.Errorf("unexpected method %s", req.Method)
			}
			_, _ = w.Write([]byte(`{"ok":true,"id":"9VKOB"}`))
		},
		"already gone": func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := newTestResource(t, handler)

			resp := &resource.DeleteResponse{State: testState(t, ctx, createdState(t))}
			r.Delete(ctx, resource.DeleteRequest{State: testState(t, ctx, createdState(t))}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("delete diagnostics: %v", resp.Diagnostics)
			}
		})
	}
}

func TestDeleteSurfacesAPIError(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	resp := &resource.DeleteResponse{State: testState(t, ctx, createdState(t))}
	r.Delete(ctx, resource.DeleteRequest{State: testState(t, ctx, createdState(t))}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic")
	}
}

// ---------------------------------------------------------------------------
// import
// ---------------------------------------------------------------------------

func TestParseImportID(t *testing.T) {
	cases := map[string]struct {
		raw        string
		maintID    string
		customerID string
		wantErr    bool
	}{
		"bare id":           {raw: "9VKOB", maintID: "9VKOB"},
		"scoped id":         {raw: "SUB1:9VKOB", maintID: "9VKOB", customerID: "SUB1"},
		"empty":             {raw: "", wantErr: true},
		"blank maintenance": {raw: "SUB1:  ", wantErr: true},
		"blank customer":    {raw: " :9VKOB", wantErr: true},
		"too many segments": {raw: "A:B:C", wantErr: true},
		"ad-hoc":            {raw: "ad-hoc", wantErr: true},
		"scoped ad-hoc":     {raw: "SUB1:AD-HOC", wantErr: true},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			maintID, customerID, err := parseImportID(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error for %q", tc.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if maintID != tc.maintID || customerID != tc.customerID {
				t.Fatalf("got (%q, %q)", maintID, customerID)
			}
		})
	}
}

func TestImportStatePersistsCustomerID(t *testing.T) {
	ctx := context.Background()

	var seen string
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		seen = req.URL.Query().Get("customerid")
		_, _ = w.Write(maintenanceBody(t, client.Maintenance{
			ID: "9VKOB", CustomerID: "SUB1", Name: "Weekly patching", Enabled: true,
			Cron: "0 3 * * 0", Duration: 60, Checklist: []string{"C1"},
		}))
	})

	resp := &resource.ImportStateResponse{State: testState(t, ctx, nil)}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "SUB1:9VKOB"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}
	if seen != "SUB1" {
		t.Fatalf("customerid query = %q", seen)
	}

	got := decodeState(t, ctx, resp.State)
	if got.ID.ValueString() != "9VKOB" || got.CustomerID.ValueString() != "SUB1" {
		t.Fatalf("unexpected state: %+v", got)
	}
}

func TestImportStateAdoptsCustomerIDFromResponse(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write(maintenanceBody(t, client.Maintenance{
			ID: "9VKOB", CustomerID: "SUB2", Name: "Weekly patching", Enabled: true,
			Cron: "0 3 * * 0", Duration: 60, Checklist: []string{"C1"},
		}))
	})

	resp := &resource.ImportStateResponse{State: testState(t, ctx, nil)}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "9VKOB"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}
	if decodeState(t, ctx, resp.State).CustomerID.ValueString() != "SUB2" {
		t.Fatal("customer_id from the API response must be persisted")
	}
}

func TestImportStateRejectsAdHocAndUnscheduledWindows(t *testing.T) {
	ctx := context.Background()

	t.Run("literal ad-hoc import id", func(t *testing.T) {
		r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
			t.Error("no request should be made")
		})
		resp := &resource.ImportStateResponse{State: testState(t, ctx, nil)}
		r.ImportState(ctx, resource.ImportStateRequest{ID: "ad-hoc"}, resp)
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected the ad-hoc import to be rejected")
		}
	})

	t.Run("api returns an ad-hoc record", func(t *testing.T) {
		r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
			_, _ = w.Write([]byte(`{"id":"ad-hoc","name":"one off","duration":30,"checklist":["C1"]}`))
		})
		resp := &resource.ImportStateResponse{State: testState(t, ctx, nil)}
		r.ImportState(ctx, resource.ImportStateRequest{ID: "9VKOB"}, resp)
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected an ad-hoc record to be rejected")
		}
	})

	t.Run("window has no cron", func(t *testing.T) {
		r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
			_, _ = w.Write([]byte(`{"id":"9VKOB","name":"one off","duration":30,"checklist":["C1"]}`))
		})
		resp := &resource.ImportStateResponse{State: testState(t, ctx, nil)}
		r.ImportState(ctx, resource.ImportStateRequest{ID: "9VKOB"}, resp)
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected an unscheduled window to be rejected")
		}
	})

	t.Run("api error", func(t *testing.T) {
		r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
		resp := &resource.ImportStateResponse{State: testState(t, ctx, nil)}
		r.ImportState(ctx, resource.ImportStateRequest{ID: "9VKOB"}, resp)
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected an error diagnostic")
		}
	})

	t.Run("malformed import id", func(t *testing.T) {
		r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
			t.Error("no request should be made")
		})
		resp := &resource.ImportStateResponse{State: testState(t, ctx, nil)}
		r.ImportState(ctx, resource.ImportStateRequest{ID: "A:B:C"}, resp)
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected an error diagnostic")
		}
	})
}

// ---------------------------------------------------------------------------
// mapping helpers
// ---------------------------------------------------------------------------

func TestChecklistFromModelSortsAndHandlesNull(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	got := checklistFromModel(ctx, checklistValue(t, "C3", "C1", "C2"), &diags)
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if len(got) != 3 || got[0] != "C1" || got[2] != "C3" {
		t.Fatalf("checklist was not sorted: %v", got)
	}

	if checklistFromModel(ctx, types.SetNull(types.StringType), &diags) != nil {
		t.Fatal("a null set must produce nil")
	}
	if checklistFromModel(ctx, types.SetUnknown(types.StringType), &diags) != nil {
		t.Fatal("an unknown set must produce nil")
	}
}

func TestApplyAPIToModelNullsAbsentValues(t *testing.T) {
	ctx := context.Background()

	model := MaintenanceResourceModel{CustomerID: types.StringUnknown()}
	diags := applyAPIToModel(ctx, &client.Maintenance{
		ID: "9VKOB", Name: "Weekly", Duration: 60, Checklist: []string{"C2", "C1"},
	}, &model)
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if !model.CustomerID.IsNull() {
		t.Fatal("an unknown customer_id with no API value must become null")
	}
	if !model.NextRun.IsNull() {
		t.Fatal("a missing nextrun must become null")
	}
	if ids := checklistStrings(t, model.Checklist); ids[0] != "C1" || ids[1] != "C2" {
		t.Fatalf("checklist = %v", ids)
	}
}
