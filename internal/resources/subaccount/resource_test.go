package subaccount

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/atlanticbt/terraform-provider-nodeping/internal/client"
)

const accountID = "201203232048C76FH"

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newTestResource(t *testing.T, handler http.HandlerFunc) *SubaccountResource {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &SubaccountResource{client: client.NewClient(client.ClientConfig{
		APIToken:     "test-token",
		BaseURL:      srv.URL,
		RateLimit:    1000,
		MaxRetries:   1,
		RetryMinWait: time.Millisecond,
		RetryMaxWait: time.Millisecond,
		Timeout:      5 * time.Second,
	})}
}

func emptyPlan(t *testing.T, ctx context.Context, model SubaccountResourceModel) tfsdk.Plan {
	t.Helper()
	s := SubaccountSchema()
	p := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if diags := p.Set(ctx, &model); diags.HasError() {
		t.Fatalf("failed to build plan: %v", diags)
	}
	return p
}

func emptyState(t *testing.T, ctx context.Context, model *SubaccountResourceModel) tfsdk.State {
	t.Helper()
	s := SubaccountSchema()
	st := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if model != nil {
		if diags := st.Set(ctx, model); diags.HasError() {
			t.Fatalf("failed to build state: %v", diags)
		}
	}
	return st
}

func decodeState(t *testing.T, ctx context.Context, st tfsdk.State) SubaccountResourceModel {
	t.Helper()
	var out SubaccountResourceModel
	if diags := st.Get(ctx, &out); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	return out
}

// planModel is a fully populated plan of the sort Terraform hands to Create.
func planModel() SubaccountResourceModel {
	return SubaccountResourceModel{
		ID:                    types.StringUnknown(),
		Name:                  types.StringValue("Example Client"),
		ContactName:           types.StringValue("Example Operations"),
		Email:                 types.StringValue("ops@example.com"),
		Timezone:              types.StringValue("-7"),
		Location:              types.StringValue("nam"),
		EmailMe:               types.StringValue("no"),
		AutodiagNotifications: types.StringValue("false"),
		Status:                types.StringUnknown(),
		Type:                  types.StringUnknown(),
		CreationDate:          types.Int64Unknown(),
		NextBillingDate:       types.StringUnknown(),
		DefaultLocations:      types.ListUnknown(types.StringType),
	}
}

func accountJSON(status string) []byte {
	body, err := json.Marshal(map[string]interface{}{
		"_id":              accountID,
		"type":             "subcustomer",
		"customer_name":    "Example Client",
		"creation_date":    1332535680000,
		"status":           status,
		"timezone":         "-7",
		"nextBillingDate":  "2026-09-01",
		"defaultlocations": []string{"nam"},
	})
	if err != nil {
		panic(err)
	}
	return body
}

// recordingHandler answers the create/read/update/delete calls a subaccount
// lifecycle makes and records every request it saw.
type recordingHandler struct {
	mu      sync.Mutex
	methods []string
	paths   []string
	bodies  []map[string]interface{}
	status  string
}

func (h *recordingHandler) record(r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.methods = append(h.methods, r.Method)
	h.paths = append(h.paths, r.URL.Path)

	body := map[string]interface{}{}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	h.bodies = append(h.bodies, body)
}

func (h *recordingHandler) requests() ([]string, []string, []map[string]interface{}) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.methods...), append([]string(nil), h.paths...), append([]map[string]interface{}(nil), h.bodies...)
}

func (h *recordingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.record(r)

	status := h.status
	if status == "" {
		status = "Active"
	}

	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodPost:
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"` + accountID + `"}`))
	case http.MethodGet:
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(accountJSON(status))
	case http.MethodPut:
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	case http.MethodDelete:
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":"Records queued for deletion."}`))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// ---------------------------------------------------------------------------
// schema
// ---------------------------------------------------------------------------

func TestSchemaMarksCreateOnlyFieldsAndComputedReads(t *testing.T) {
	ctx := context.Background()
	s := SubaccountSchema()

	schemaResp := &resource.SchemaResponse{}
	(&SubaccountResource{}).Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema produced diagnostics: %v", schemaResp.Diagnostics)
	}

	if diags := s.ValidateImplementation(ctx); diags.HasError() {
		t.Fatalf("schema is not a valid implementation: %v", diags)
	}

	// Optional so an existing subaccount can be imported: NodePing never
	// returns them. Create still requires them, enforced in ModifyPlan.
	for _, name := range []string{"contactname", "email", "location"} {
		attr, ok := s.Attributes[name]
		if !ok {
			t.Fatalf("expected attribute %q", name)
		}
		if attr.IsRequired() || !attr.IsOptional() {
			t.Errorf("%q should be optional", name)
		}
	}

	for _, name := range []string{"type", "creation_date", "next_billing_date", "default_locations", "id"} {
		attr, ok := s.Attributes[name]
		if !ok {
			t.Fatalf("expected attribute %q", name)
		}
		if !attr.IsComputed() || attr.IsOptional() {
			t.Errorf("%q should be read-only computed", name)
		}
	}

	if _, ok := s.Attributes["accountsuppressall"]; ok {
		t.Error("accountsuppressall is a PUT-only action flag and must not be a resource attribute")
	}
	if _, ok := s.Attributes["customer_id"]; ok {
		t.Error("the resource is the account, so it must not carry a customer_id attribute")
	}
}

func TestMetadataUsesSubaccountTypeName(t *testing.T) {
	resp := &resource.MetadataResponse{}
	(&SubaccountResource{}).Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "nodeping"}, resp)
	if resp.TypeName != "nodeping_subaccount" {
		t.Errorf("expected 'nodeping_subaccount', got %q", resp.TypeName)
	}
}

func TestConfigureAcceptsClientAndRejectsOtherTypes(t *testing.T) {
	ctx := context.Background()

	r := &SubaccountResource{}
	resp := &resource.ConfigureResponse{}
	r.Configure(ctx, resource.ConfigureRequest{}, resp)
	if resp.Diagnostics.HasError() || r.client != nil {
		t.Error("a nil ProviderData must be ignored")
	}

	c := client.NewClient(client.ClientConfig{APIToken: "t"})
	resp = &resource.ConfigureResponse{}
	r.Configure(ctx, resource.ConfigureRequest{ProviderData: c}, resp)
	if resp.Diagnostics.HasError() || r.client != c {
		t.Errorf("expected the client to be stored, diags: %v", resp.Diagnostics)
	}

	resp = &resource.ConfigureResponse{}
	(&SubaccountResource{}).Configure(ctx, resource.ConfigureRequest{ProviderData: "nope"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Error("expected an error for an unexpected ProviderData type")
	}
}

// planCreateOnly runs the create-only plan modifier against a state/plan pair
// for the `contactname` attribute. A nil stateValue models a resource that has
// no prior state at all, which is what a create looks like.
func planCreateOnly(t *testing.T, ctx context.Context, stateValue *types.String, planValue types.String) *planmodifier.StringResponse {
	t.Helper()

	planned := stateModel()
	planned.ContactName = planValue
	plan := emptyPlan(t, ctx, planned)

	var st tfsdk.State
	stateAttr := types.StringNull()
	if stateValue == nil {
		st = emptyState(t, ctx, nil)
	} else {
		prior := stateModel()
		prior.ContactName = *stateValue
		st = emptyState(t, ctx, &prior)
		stateAttr = *stateValue
	}

	req := planmodifier.StringRequest{
		Path:        path.Root("contactname"),
		State:       st,
		Plan:        plan,
		StateValue:  stateAttr,
		PlanValue:   planValue,
		ConfigValue: planValue,
	}
	resp := &planmodifier.StringResponse{PlanValue: planValue}
	unchangeableAfterCreate().PlanModifyString(ctx, req, resp)
	return resp
}

// Replacing a subaccount deletes every check and contact in it, and NodePing
// can neither update nor return contactname and email. So a change to either
// on an existing subaccount is refused at plan time instead of replacing it.
func TestUnchangeableAfterCreate(t *testing.T) {
	ctx := context.Background()

	if unchangeableAfterCreate().Description(ctx) == "" ||
		unchangeableAfterCreate().MarkdownDescription(ctx) == "" {
		t.Error("expected the modifier to describe itself")
	}

	empty := types.StringValue("")
	prior := types.StringValue("Example Operations")
	null := types.StringNull()

	cases := []struct {
		name    string
		state   *types.String
		plan    types.String
		wantErr bool
	}{
		{"should allow any value when creating", nil, types.StringValue("Example Operations"), false},
		{"should record the value when prior state is null after an import", &null, types.StringValue("Example Operations"), false},
		{"should record the value when the prior value is empty", &empty, types.StringValue("Example Operations"), false},
		{"should be a no-op when unchanged", &prior, types.StringValue("Example Operations"), false},
		{"should allow removing the value from configuration", &prior, types.StringNull(), false},
		{"should allow an unknown planned value", &prior, types.StringUnknown(), false},
		{"should refuse a change on an existing subaccount", &prior, types.StringValue("Someone Else"), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := planCreateOnly(t, ctx, tc.state, tc.plan)
			if resp.RequiresReplace {
				t.Error("must never plan a replacement: replacing a subaccount deletes everything in it")
			}
			if got := resp.Diagnostics.HasError(); got != tc.wantErr {
				t.Errorf("error = %v, want %v (%v)", got, tc.wantErr, resp.Diagnostics)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func TestCreateStoresAPIValuesAndSkipsRedundantPut(t *testing.T) {
	ctx := context.Background()
	h := &recordingHandler{}
	r := newTestResource(t, h.ServeHTTP)

	resp := &resource.CreateResponse{State: emptyState(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{Plan: emptyPlan(t, ctx, planModel())}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	methods, paths, bodies := h.requests()
	if len(methods) != 2 || methods[0] != http.MethodPost || methods[1] != http.MethodGet {
		t.Fatalf("expected POST then GET with no PUT, got %v %v", methods, paths)
	}
	if bodies[0]["contactname"] != "Example Operations" || bodies[0]["email"] != "ops@example.com" {
		t.Errorf("create body lost the primary contact: %v", bodies[0])
	}

	state := decodeState(t, ctx, resp.State)
	if state.ID.ValueString() != accountID {
		t.Errorf("expected id %q, got %q", accountID, state.ID.ValueString())
	}
	if state.Status.ValueString() != "Active" {
		t.Errorf("expected status Active, got %q", state.Status.ValueString())
	}
	if state.CreationDate.ValueInt64() != 1332535680000 {
		t.Errorf("unexpected creation_date %d", state.CreationDate.ValueInt64())
	}
	if state.ContactName.ValueString() != "Example Operations" {
		t.Errorf("contactname must survive from the plan, got %q", state.ContactName.ValueString())
	}
	if state.Email.ValueString() != "ops@example.com" {
		t.Errorf("email must survive from the plan, got %q", state.Email.ValueString())
	}
	if state.Location.ValueString() != "nam" {
		t.Errorf("location must survive from the plan, got %q", state.Location.ValueString())
	}
	if len(state.DefaultLocations.Elements()) != 1 {
		t.Errorf("expected one default location, got %v", state.DefaultLocations)
	}
}

func TestCreateIssuesFollowUpPutForNonDefaultStatus(t *testing.T) {
	ctx := context.Background()
	h := &recordingHandler{status: "Suspend"}
	r := newTestResource(t, h.ServeHTTP)

	plan := planModel()
	plan.Status = types.StringValue("Suspend")
	plan.AutodiagNotifications = types.StringValue("true")

	resp := &resource.CreateResponse{State: emptyState(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{Plan: emptyPlan(t, ctx, plan)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	methods, _, bodies := h.requests()
	if len(methods) != 3 || methods[1] != http.MethodPut {
		t.Fatalf("expected POST, PUT, GET, got %v", methods)
	}
	if bodies[1]["status"] != "Suspend" || bodies[1]["autodiagnotifications"] != "true" {
		t.Errorf("unexpected follow-up body: %v", bodies[1])
	}
	if _, ok := bodies[1]["name"]; ok {
		t.Error("the follow-up PUT must not resend create-time fields")
	}

	state := decodeState(t, ctx, resp.State)
	if state.Status.ValueString() != "Suspend" {
		t.Errorf("expected status Suspend, got %q", state.Status.ValueString())
	}
}

func TestCreateReportsAPIFailure(t *testing.T) {
	ctx := context.Background()
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid location"}`))
	})

	resp := &resource.CreateResponse{State: emptyState(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{Plan: emptyPlan(t, ctx, planModel())}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), "Error Creating Subaccount") {
		t.Errorf("unexpected diagnostic: %v", resp.Diagnostics)
	}
}

func TestCreateRecordsTheAccountIDWhenTheFollowUpPutFails(t *testing.T) {
	ctx := context.Background()
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPost {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"` + accountID + `"}`))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"status unsupported"}`))
	})

	plan := planModel()
	plan.Status = types.StringValue("Suspend")

	resp := &resource.CreateResponse{State: emptyState(t, ctx, nil), Identity: newIdentity(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{Plan: emptyPlan(t, ctx, plan)}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error")
	}

	// The POST created a real account. It must not be left with
	// nothing in state pointing at it.
	got := decodeState(t, ctx, resp.State)
	if got.ID.ValueString() != accountID {
		t.Fatalf("state must carry the account ID so the next plan can reconcile it, got %q", got.ID.ValueString())
	}
	if !resp.Identity.Raw.IsFullyNull() {
		t.Errorf("identity must stay null on a failed create, got %v", resp.Identity.Raw)
	}
}

func TestCreateRecordsTheAccountIDWhenTheReadBackFails(t *testing.T) {
	ctx := context.Background()
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPost {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"` + accountID + `"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	})

	resp := &resource.CreateResponse{State: emptyState(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{Plan: emptyPlan(t, ctx, planModel())}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), "Error Reading Created Subaccount") {
		t.Errorf("unexpected diagnostic: %v", resp.Diagnostics)
	}

	got := decodeState(t, ctx, resp.State)
	if got.ID.ValueString() != accountID {
		t.Fatalf("state must carry the account ID so the next plan can reconcile it, got %q", got.ID.ValueString())
	}
}

func TestCreateWritesNoStateWhenTheAccountWasNeverCreated(t *testing.T) {
	ctx := context.Background()
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid location"}`))
	})

	resp := &resource.CreateResponse{State: emptyState(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{Plan: emptyPlan(t, ctx, planModel())}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error")
	}
	// No account ID came back, so there is nothing to record and state must
	// stay empty rather than gain an object OpenTofu would try to manage.
	if !resp.State.Raw.IsNull() {
		t.Fatalf("state must stay empty when no account was created, got %v", resp.State.Raw)
	}
}

// ---------------------------------------------------------------------------
// Read
// ---------------------------------------------------------------------------

func stateModel() SubaccountResourceModel {
	m := planModel()
	m.ID = types.StringValue(accountID)
	m.Status = types.StringValue("Active")
	m.Type = types.StringValue("subcustomer")
	m.CreationDate = types.Int64Value(1332535680000)
	m.NextBillingDate = types.StringValue("2026-09-01")
	locations, _ := types.ListValueFrom(context.Background(), types.StringType, []string{"nam"})
	m.DefaultLocations = locations
	return m
}

func TestReadAdoptsRemoteDrift(t *testing.T) {
	ctx := context.Background()
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"_id":"` + accountID + `","customer_name":"Renamed Remotely",` +
			`"status":"Suspend","timezone":"0","type":"subcustomer","defaultlocations":["eur"]}`))
	})

	prior := stateModel()
	resp := &resource.ReadResponse{State: emptyState(t, ctx, &prior)}
	r.Read(ctx, resource.ReadRequest{State: emptyState(t, ctx, &prior)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	state := decodeState(t, ctx, resp.State)
	if state.Name.ValueString() != "Renamed Remotely" {
		t.Errorf("a refresh must surface a remote rename, got %q", state.Name.ValueString())
	}
	if state.Status.ValueString() != "Suspend" {
		t.Errorf("expected status Suspend, got %q", state.Status.ValueString())
	}
	if state.Timezone.ValueString() != "0" {
		t.Errorf("expected timezone 0, got %q", state.Timezone.ValueString())
	}
	if state.ContactName.ValueString() != "Example Operations" {
		t.Errorf("contactname is not returned by the API and must be left alone, got %q", state.ContactName.ValueString())
	}
	if state.Email.ValueString() != "ops@example.com" {
		t.Errorf("email is not returned by the API and must be left alone, got %q", state.Email.ValueString())
	}
	if state.Location.ValueString() != "nam" {
		t.Errorf("location is not returned by the API and must be left alone, got %q", state.Location.ValueString())
	}
}

func TestReadSeedsDefaultsForImportedState(t *testing.T) {
	ctx := context.Background()
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(accountJSON("Active"))
	})

	prior := stateModel()
	prior.EmailMe = types.StringNull()
	prior.AutodiagNotifications = types.StringNull()

	resp := &resource.ReadResponse{State: emptyState(t, ctx, &prior)}
	r.Read(ctx, resource.ReadRequest{State: emptyState(t, ctx, &prior)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	state := decodeState(t, ctx, resp.State)
	if state.EmailMe.ValueString() != "no" {
		t.Errorf("expected emailme to default to 'no', got %q", state.EmailMe)
	}
	if state.AutodiagNotifications.ValueString() != "false" {
		t.Errorf("expected autodiagnotifications to default to 'false', got %q", state.AutodiagNotifications)
	}
}

func TestReadRemovesResourceWhenAccountIsGone(t *testing.T) {
	ctx := context.Background()
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	})

	prior := stateModel()
	resp := &resource.ReadResponse{State: emptyState(t, ctx, &prior)}
	r.Read(ctx, resource.ReadRequest{State: emptyState(t, ctx, &prior)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("expected the resource to be removed from state")
	}
}

func TestReadReportsOtherErrors(t *testing.T) {
	ctx := context.Background()
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"denied"}`))
	})

	prior := stateModel()
	resp := &resource.ReadResponse{State: emptyState(t, ctx, &prior)}
	r.Read(ctx, resource.ReadRequest{State: emptyState(t, ctx, &prior)}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error")
	}
	if resp.State.Raw.IsNull() {
		t.Error("a transient failure must not drop the resource from state")
	}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func TestUpdateSendsMutableFieldsOnly(t *testing.T) {
	ctx := context.Background()
	h := &recordingHandler{}
	r := newTestResource(t, h.ServeHTTP)

	prior := stateModel()
	plan := stateModel()
	plan.Name = types.StringValue("Renamed")
	plan.Timezone = types.StringValue("0")
	plan.Location = types.StringValue("eur")
	plan.EmailMe = types.StringValue("yes")

	resp := &resource.UpdateResponse{State: emptyState(t, ctx, &prior)}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  emptyPlan(t, ctx, plan),
		State: emptyState(t, ctx, &prior),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	methods, paths, bodies := h.requests()
	if len(methods) != 2 || methods[0] != http.MethodPut || methods[1] != http.MethodGet {
		t.Fatalf("expected PUT then GET, got %v", methods)
	}
	if paths[0] != "/accounts/"+accountID {
		t.Errorf("unexpected PUT path %q", paths[0])
	}
	body := bodies[0]
	if body["name"] != "Renamed" || body["timezone"] != "0" || body["location"] != "eur" || body["emailme"] != "yes" {
		t.Errorf("unexpected PUT body: %v", body)
	}
	for _, forbidden := range []string{"contactname", "email"} {
		if _, ok := body[forbidden]; ok {
			t.Errorf("%q is create-only and must never be sent on update", forbidden)
		}
	}

	state := decodeState(t, ctx, resp.State)
	// The mock echoes the pre-update document; the planned values must win so
	// Terraform's post-apply consistency check passes.
	if state.Name.ValueString() != "Renamed" || state.Timezone.ValueString() != "0" {
		t.Errorf("update must keep planned values, got name=%q timezone=%q",
			state.Name.ValueString(), state.Timezone.ValueString())
	}
}

func TestUpdateReportsMissingAccount(t *testing.T) {
	ctx := context.Background()
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	})

	prior := stateModel()
	resp := &resource.UpdateResponse{State: emptyState(t, ctx, &prior)}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  emptyPlan(t, ctx, stateModel()),
		State: emptyState(t, ctx, &prior),
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), "No Longer Exists") {
		t.Errorf("unexpected diagnostic: %v", resp.Diagnostics)
	}
}

func TestUpdateReportsAPIFailure(t *testing.T) {
	ctx := context.Background()
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	})

	prior := stateModel()
	resp := &resource.UpdateResponse{State: emptyState(t, ctx, &prior)}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  emptyPlan(t, ctx, stateModel()),
		State: emptyState(t, ctx, &prior),
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error")
	}
}

func TestUpdateReportsReadBackFailure(t *testing.T) {
	ctx := context.Background()
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPut {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"denied"}`))
	})

	prior := stateModel()
	resp := &resource.UpdateResponse{State: emptyState(t, ctx, &prior)}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  emptyPlan(t, ctx, stateModel()),
		State: emptyState(t, ctx, &prior),
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), "Error Reading Updated Subaccount") {
		t.Errorf("unexpected diagnostic: %v", resp.Diagnostics)
	}
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func TestDeleteAcceptsQueuedAcknowledgement(t *testing.T) {
	ctx := context.Background()
	h := &recordingHandler{}
	r := newTestResource(t, h.ServeHTTP)

	prior := stateModel()
	resp := &resource.DeleteResponse{State: emptyState(t, ctx, &prior)}
	r.Delete(ctx, resource.DeleteRequest{State: emptyState(t, ctx, &prior)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	methods, paths, _ := h.requests()
	if len(methods) != 1 || methods[0] != http.MethodDelete {
		t.Fatalf("expected exactly one DELETE and no polling, got %v %v", methods, paths)
	}
}

func TestDeleteIsIdempotent(t *testing.T) {
	ctx := context.Background()
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	})

	prior := stateModel()
	resp := &resource.DeleteResponse{State: emptyState(t, ctx, &prior)}
	r.Delete(ctx, resource.DeleteRequest{State: emptyState(t, ctx, &prior)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("deleting an already-absent subaccount must succeed: %v", resp.Diagnostics)
	}
}

func TestDeleteReportsAPIFailure(t *testing.T) {
	ctx := context.Background()
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"cannot delete your own account"}`))
	})

	prior := stateModel()
	resp := &resource.DeleteResponse{State: emptyState(t, ctx, &prior)}
	r.Delete(ctx, resource.DeleteRequest{State: emptyState(t, ctx, &prior)}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error")
	}
}

// ---------------------------------------------------------------------------
// Import
// ---------------------------------------------------------------------------

func TestImportStateSetsID(t *testing.T) {
	ctx := context.Background()
	r := &SubaccountResource{}

	resp := &resource.ImportStateResponse{State: emptyState(t, ctx, nil)}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "  " + accountID + "  "}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var id types.String
	if diags := resp.State.GetAttribute(ctx, path.Root("id"), &id); diags.HasError() {
		t.Fatalf("failed to read the imported id: %v", diags)
	}
	if id.ValueString() != accountID {
		t.Errorf("expected %q, got %q", accountID, id.ValueString())
	}
}

func TestImportStateRejectsEmptyID(t *testing.T) {
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(t, ctx, nil)}
	(&SubaccountResource{}).ImportState(ctx, resource.ImportStateRequest{ID: "   "}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for an empty import ID")
	}
}

// ---------------------------------------------------------------------------
// helper functions
// ---------------------------------------------------------------------------

func TestStringOrEmptyAndNonDefaultString(t *testing.T) {
	if stringOrEmpty(types.StringNull()) != "" || stringOrEmpty(types.StringUnknown()) != "" {
		t.Error("an unset attribute must render as an empty string")
	}
	if stringOrEmpty(types.StringValue("x")) != "x" {
		t.Error("a set attribute must render verbatim")
	}
	if nonDefaultString(types.StringValue("false"), "false") != "" {
		t.Error("a value equal to the API default must not be sent")
	}
	if nonDefaultString(types.StringValue("true"), "false") != "true" {
		t.Error("a value differing from the API default must be sent")
	}
}

func TestIsUnset(t *testing.T) {
	if !isUnset(types.StringNull()) || !isUnset(types.StringUnknown()) {
		t.Error("null and unknown are unset")
	}
	if isUnset(types.StringValue("")) {
		t.Error("an empty string is a value, not unset")
	}
}
