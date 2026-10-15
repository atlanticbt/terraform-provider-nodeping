package contactgroup

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/atlanticbt/terraform-provider-nodeping/internal/client"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newTestResource(t *testing.T, handler http.HandlerFunc) *ContactGroupResource {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &ContactGroupResource{client: client.NewClient(client.ClientConfig{
		APIToken:     "test-token",
		BaseURL:      srv.URL,
		RateLimit:    1000,
		MaxRetries:   1,
		RetryMinWait: time.Millisecond,
		RetryMaxWait: time.Millisecond,
		Timeout:      5 * time.Second,
	})}
}

func memberSet(t *testing.T, values ...string) types.Set {
	t.Helper()
	set, diags := types.SetValueFrom(context.Background(), types.StringType, values)
	if diags.HasError() {
		t.Fatalf("failed to build member set: %v", diags)
	}
	return set
}

func buildPlan(t *testing.T, ctx context.Context, model ContactGroupResourceModel) tfsdk.Plan {
	t.Helper()
	s := ContactGroupSchema()
	p := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if diags := p.Set(ctx, &model); diags.HasError() {
		t.Fatalf("failed to build plan: %v", diags)
	}
	return p
}

func buildState(t *testing.T, ctx context.Context, model *ContactGroupResourceModel) tfsdk.State {
	t.Helper()
	s := ContactGroupSchema()
	st := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if model != nil {
		if diags := st.Set(ctx, model); diags.HasError() {
			t.Fatalf("failed to build state: %v", diags)
		}
	}
	return st
}

func decodeState(t *testing.T, ctx context.Context, st tfsdk.State) ContactGroupResourceModel {
	t.Helper()
	var out ContactGroupResourceModel
	if diags := st.Get(ctx, &out); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	return out
}

func groupJSON(t *testing.T, id, customerID, name string, members []string) []byte {
	t.Helper()
	body, err := json.Marshal(client.ContactGroup{
		ID:         id,
		Type:       "group",
		CustomerID: customerID,
		Name:       name,
		Members:    members,
	})
	if err != nil {
		t.Fatalf("failed to encode group: %v", err)
	}
	return body
}

func memberStrings(t *testing.T, ctx context.Context, set types.Set) []string {
	t.Helper()
	if set.IsNull() {
		t.Fatal("expected members to be a known set, got null")
	}
	var out []string
	if diags := set.ElementsAs(ctx, &out, false); diags.HasError() {
		t.Fatalf("failed to read members: %v", diags)
	}
	return out
}

// ---------------------------------------------------------------------------
// wiring
// ---------------------------------------------------------------------------

func TestMetadataUsesContactgroupTypeName(t *testing.T) {
	r := &ContactGroupResource{}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "nodeping"}, resp)

	if resp.TypeName != "nodeping_contactgroup" {
		t.Errorf("unexpected type name %q", resp.TypeName)
	}
}

func TestSchemaMembersIsAnOptionalComputedStringSet(t *testing.T) {
	r := &ContactGroupResource{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)

	attr, ok := resp.Schema.Attributes["members"]
	if !ok {
		t.Fatal("members attribute missing from schema")
	}
	if !attr.IsOptional() || !attr.IsComputed() {
		t.Error("members must be Optional and Computed")
	}
	if _, isSet := attr.GetType().(types.SetType); !isSet {
		t.Errorf("members must be a set, got %T", attr.GetType())
	}

	customerID, ok := resp.Schema.Attributes["customer_id"]
	if !ok {
		t.Fatal("customer_id attribute missing from schema")
	}
	if !customerID.IsOptional() || !customerID.IsComputed() {
		t.Error("customer_id must be Optional and Computed")
	}
}

func TestConfigureRejectsWrongProviderData(t *testing.T) {
	r := &ContactGroupResource{}
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "not a client"}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic for the wrong provider data type")
	}
}

func TestConfigureIgnoresNilProviderData(t *testing.T) {
	r := &ContactGroupResource{}
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if r.client != nil {
		t.Error("client must stay nil when the provider has not configured yet")
	}
}

func TestConfigureStoresClient(t *testing.T) {
	c := client.NewClient(client.ClientConfig{APIToken: "t"})
	r := &ContactGroupResource{}
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: c}, resp)

	if r.client != c {
		t.Error("expected the provider client to be stored on the resource")
	}
}

func TestScopedClientFallsBackToProviderClient(t *testing.T) {
	c := client.NewClient(client.ClientConfig{APIToken: "t"})
	r := &ContactGroupResource{client: c}

	if r.scopedClient(types.StringNull()) != c {
		t.Error("a null customer_id must reuse the provider client")
	}
	if r.scopedClient(types.StringValue("")) != c {
		t.Error("an empty customer_id must reuse the provider client")
	}
	if r.scopedClient(types.StringUnknown()) != c {
		t.Error("an unknown customer_id must reuse the provider client")
	}
	if r.scopedClient(types.StringValue("SUB")) == c {
		t.Error("a set customer_id must produce a scoped client")
	}
}

// ---------------------------------------------------------------------------
// create
// ---------------------------------------------------------------------------

func TestCreateWritesIDAndSortedMembersToState(t *testing.T) {
	ctx := context.Background()

	var body client.ContactGroupCreateRequest
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", req.Method)
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		// The API returns members in its own order.
		if _, err := w.Write(groupJSON(t, "ACC-G-ISYB7", "ACC", "Alerts", []string{"K5SP9CQP", "1664V"})); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	})

	resp := &resource.CreateResponse{State: buildState(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{
		Plan: buildPlan(t, ctx, ContactGroupResourceModel{
			ID:         types.StringUnknown(),
			CustomerID: types.StringUnknown(),
			Name:       types.StringValue("Alerts"),
			Members:    memberSet(t, "1664V", "K5SP9CQP"),
		}),
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	// The request body must be sorted so repeated plans produce identical calls.
	if len(body.Members) != 2 || body.Members[0] != "1664V" || body.Members[1] != "K5SP9CQP" {
		t.Errorf("expected sorted members in the request, got %v", body.Members)
	}

	got := decodeState(t, ctx, resp.State)
	if got.ID.ValueString() != "ACC-G-ISYB7" {
		t.Errorf("unexpected id %q", got.ID.ValueString())
	}
	if got.CustomerID.ValueString() != "ACC" {
		t.Errorf("expected customer_id to be filled in from the response, got %q", got.CustomerID.ValueString())
	}
	members := memberStrings(t, ctx, got.Members)
	if len(members) != 2 {
		t.Errorf("expected 2 members, got %v", members)
	}
}

func TestCreateWithoutMembersSendsAndStoresEmptySet(t *testing.T) {
	ctx := context.Background()

	var raw map[string]json.RawMessage
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		if err := json.NewDecoder(req.Body).Decode(&raw); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		if _, err := w.Write(groupJSON(t, "ACC-G-1", "ACC", "Empty", nil)); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	})

	resp := &resource.CreateResponse{State: buildState(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{
		Plan: buildPlan(t, ctx, ContactGroupResourceModel{
			ID:         types.StringUnknown(),
			CustomerID: types.StringUnknown(),
			Name:       types.StringValue("Empty"),
			Members:    types.SetNull(types.StringType),
		}),
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if got := string(raw["members"]); got != "[]" {
		t.Errorf("expected members to be sent as [], got %s", got)
	}

	got := decodeState(t, ctx, resp.State)
	if got.Members.IsNull() || got.Members.IsUnknown() {
		t.Fatal("members must resolve to a known empty set, not null")
	}
	if len(got.Members.Elements()) != 0 {
		t.Errorf("expected no members, got %v", got.Members)
	}
}

func TestCreatePreservesConfiguredCustomerIDAndScopesRequest(t *testing.T) {
	ctx := context.Background()

	var seen string
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		seen = req.URL.Query().Get("customerid")
		// The API echoes back the account that actually owns the group.
		if _, err := w.Write(groupJSON(t, "SUB-G-1", "SUB", "Alerts", []string{"AAA"})); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	})

	resp := &resource.CreateResponse{State: buildState(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{
		Plan: buildPlan(t, ctx, ContactGroupResourceModel{
			ID:         types.StringUnknown(),
			CustomerID: types.StringValue("SUB"),
			Name:       types.StringValue("Alerts"),
			Members:    memberSet(t, "AAA"),
		}),
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if seen != "SUB" {
		t.Errorf("expected the request to be scoped to customerid=SUB, got %q", seen)
	}
	if got := decodeState(t, ctx, resp.State); got.CustomerID.ValueString() != "SUB" {
		t.Errorf("expected the configured customer_id to survive, got %q", got.CustomerID.ValueString())
	}
}

func TestCreateSurfacesAPIError(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})

	resp := &resource.CreateResponse{State: buildState(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{
		Plan: buildPlan(t, ctx, ContactGroupResourceModel{
			ID:         types.StringUnknown(),
			CustomerID: types.StringUnknown(),
			Name:       types.StringValue("Alerts"),
			Members:    memberSet(t, "AAA"),
		}),
	}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic for the failed create")
	}
}

// ---------------------------------------------------------------------------
// read
// ---------------------------------------------------------------------------

func TestReadRefreshesNameAndMembers(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", req.Method)
		}
		if _, err := w.Write(groupJSON(t, "ACC-G-1", "ACC", "Renamed Upstream", []string{"ZZZ", "AAA"})); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	})

	prior := ContactGroupResourceModel{
		ID:         types.StringValue("ACC-G-1"),
		CustomerID: types.StringValue("ACC"),
		Name:       types.StringValue("Old Name"),
		Members:    memberSet(t, "AAA"),
	}

	resp := &resource.ReadResponse{State: buildState(t, ctx, &prior)}
	r.Read(ctx, resource.ReadRequest{State: buildState(t, ctx, &prior)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	got := decodeState(t, ctx, resp.State)
	if got.Name.ValueString() != "Renamed Upstream" {
		t.Errorf("expected the upstream name, got %q", got.Name.ValueString())
	}
	members := memberStrings(t, ctx, got.Members)
	if len(members) != 2 || members[0] != "AAA" || members[1] != "ZZZ" {
		t.Errorf("expected sorted upstream members, got %v", members)
	}
}

func TestReadRemovesResourceOnNotFound(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	prior := ContactGroupResourceModel{
		ID:         types.StringValue("GONE-G-1"),
		CustomerID: types.StringValue("ACC"),
		Name:       types.StringValue("Gone"),
		Members:    memberSet(t),
	}

	resp := &resource.ReadResponse{State: buildState(t, ctx, &prior)}
	r.Read(ctx, resource.ReadRequest{State: buildState(t, ctx, &prior)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("a missing group must not produce an error: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("expected the resource to be removed from state")
	}
}

func TestReadSurfacesUnexpectedError(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	prior := ContactGroupResourceModel{
		ID:         types.StringValue("ACC-G-1"),
		CustomerID: types.StringValue("ACC"),
		Name:       types.StringValue("Alerts"),
		Members:    memberSet(t),
	}

	resp := &resource.ReadResponse{State: buildState(t, ctx, &prior)}
	r.Read(ctx, resource.ReadRequest{State: buildState(t, ctx, &prior)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic for a 500")
	}
	if resp.State.Raw.IsNull() {
		t.Error("a server error must not silently drop the resource from state")
	}
}

// ---------------------------------------------------------------------------
// update
// ---------------------------------------------------------------------------

func TestUpdateSendsNewMembershipAndKeepsCustomerID(t *testing.T) {
	ctx := context.Background()

	var body client.ContactGroupUpdateRequest
	var seen string
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", req.Method)
		}
		seen = req.URL.Query().Get("customerid")
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		if _, err := w.Write(groupJSON(t, "SUB-G-1", "SUB", "Renamed", []string{"BBB", "CCC"})); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	})

	state := ContactGroupResourceModel{
		ID:         types.StringValue("SUB-G-1"),
		CustomerID: types.StringValue("SUB"),
		Name:       types.StringValue("Alerts"),
		Members:    memberSet(t, "AAA"),
	}
	plan := ContactGroupResourceModel{
		ID:         types.StringValue("SUB-G-1"),
		CustomerID: types.StringValue("SUB"),
		Name:       types.StringValue("Renamed"),
		Members:    memberSet(t, "CCC", "BBB"),
	}

	resp := &resource.UpdateResponse{State: buildState(t, ctx, &state)}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  buildPlan(t, ctx, plan),
		State: buildState(t, ctx, &state),
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if seen != "SUB" {
		t.Errorf("expected the update to be scoped to customerid=SUB, got %q", seen)
	}
	if body.ID != "SUB-G-1" {
		t.Errorf("expected the group ID in the request body, got %q", body.ID)
	}
	if len(body.Members) != 2 || body.Members[0] != "BBB" {
		t.Errorf("expected sorted members in the request, got %v", body.Members)
	}

	got := decodeState(t, ctx, resp.State)
	if got.Name.ValueString() != "Renamed" {
		t.Errorf("unexpected name %q", got.Name.ValueString())
	}
	if got.CustomerID.ValueString() != "SUB" {
		t.Errorf("expected customer_id to be preserved, got %q", got.CustomerID.ValueString())
	}
}

func TestUpdateInheritsCustomerIDFromStateWhenPlanIsNull(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		if _, err := w.Write(groupJSON(t, "ACC-G-1", "", "Alerts", []string{"AAA"})); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	})

	state := ContactGroupResourceModel{
		ID:         types.StringValue("ACC-G-1"),
		CustomerID: types.StringValue("ACC"),
		Name:       types.StringValue("Alerts"),
		Members:    memberSet(t),
	}
	plan := ContactGroupResourceModel{
		ID:         types.StringValue("ACC-G-1"),
		CustomerID: types.StringNull(),
		Name:       types.StringValue("Alerts"),
		Members:    memberSet(t, "AAA"),
	}

	resp := &resource.UpdateResponse{State: buildState(t, ctx, &state)}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  buildPlan(t, ctx, plan),
		State: buildState(t, ctx, &state),
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if got := decodeState(t, ctx, resp.State); got.CustomerID.ValueString() != "ACC" {
		t.Errorf("expected customer_id to be carried over from state, got %q", got.CustomerID.ValueString())
	}
}

func TestUpdateSurfacesAPIError(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})

	state := ContactGroupResourceModel{
		ID:         types.StringValue("ACC-G-1"),
		CustomerID: types.StringValue("ACC"),
		Name:       types.StringValue("Alerts"),
		Members:    memberSet(t),
	}

	resp := &resource.UpdateResponse{State: buildState(t, ctx, &state)}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  buildPlan(t, ctx, state),
		State: buildState(t, ctx, &state),
	}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic for the failed update")
	}
}

// ---------------------------------------------------------------------------
// delete
// ---------------------------------------------------------------------------

func TestDeleteSucceeds(t *testing.T) {
	ctx := context.Background()

	called := false
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		called = true
		if req.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", req.Method)
		}
		if _, err := w.Write([]byte(`{"ok":true}`)); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	})

	state := ContactGroupResourceModel{
		ID:         types.StringValue("ACC-G-1"),
		CustomerID: types.StringValue("ACC"),
		Name:       types.StringValue("Alerts"),
		Members:    memberSet(t),
	}

	resp := &resource.DeleteResponse{State: buildState(t, ctx, &state)}
	r.Delete(ctx, resource.DeleteRequest{State: buildState(t, ctx, &state)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if !called {
		t.Error("expected a DELETE request to be sent")
	}
}

func TestDeleteIsIdempotentOnNotFound(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	state := ContactGroupResourceModel{
		ID:         types.StringValue("GONE-G-1"),
		CustomerID: types.StringValue("ACC"),
		Name:       types.StringValue("Alerts"),
		Members:    memberSet(t),
	}

	resp := &resource.DeleteResponse{State: buildState(t, ctx, &state)}
	r.Delete(ctx, resource.DeleteRequest{State: buildState(t, ctx, &state)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("deleting an already-absent group must succeed: %v", resp.Diagnostics)
	}
}

func TestDeleteSurfacesUnexpectedError(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	state := ContactGroupResourceModel{
		ID:         types.StringValue("ACC-G-1"),
		CustomerID: types.StringValue("ACC"),
		Name:       types.StringValue("Alerts"),
		Members:    memberSet(t),
	}

	resp := &resource.DeleteResponse{State: buildState(t, ctx, &state)}
	r.Delete(ctx, resource.DeleteRequest{State: buildState(t, ctx, &state)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic for a 500")
	}
}

// ---------------------------------------------------------------------------
// import
// ---------------------------------------------------------------------------

func TestParseImportID(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		groupID    string
		customerID string
		ok         bool
	}{
		{"bare group id", "ACC-G-ISYB7", "ACC-G-ISYB7", "", true},
		{"customer scoped", "SUB:ACC-G-ISYB7", "ACC-G-ISYB7", "SUB", true},
		{"empty", "", "", "", false},
		{"blank group id", "SUB:", "", "", false},
		{"whitespace group id", "SUB:   ", "", "", false},
		{"too many parts", "A:B:C", "", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			groupID, customerID, ok := parseImportID(tc.raw)
			if ok != tc.ok {
				t.Fatalf("expected ok=%v, got %v", tc.ok, ok)
			}
			if groupID != tc.groupID || customerID != tc.customerID {
				t.Errorf("expected (%q, %q), got (%q, %q)", tc.groupID, tc.customerID, groupID, customerID)
			}
		})
	}
}

func TestImportStateBareIDPersistsCustomerIDFromAPI(t *testing.T) {
	ctx := context.Background()

	var seen string
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		seen = req.URL.Query().Get("customerid")
		if _, err := w.Write(groupJSON(t, "ACC-G-ISYB7", "ACC", "Alerts", []string{"K5SP9CQP"})); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	})

	resp := &resource.ImportStateResponse{State: buildState(t, ctx, nil)}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "ACC-G-ISYB7"}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if seen != "" {
		t.Errorf("a bare import ID must not scope the request, got customerid=%q", seen)
	}

	got := decodeState(t, ctx, resp.State)
	if got.CustomerID.ValueString() != "ACC" {
		t.Errorf("expected customer_id to be persisted from the API, got %q", got.CustomerID.ValueString())
	}
	if got.ID.ValueString() != "ACC-G-ISYB7" {
		t.Errorf("unexpected id %q", got.ID.ValueString())
	}
	if len(memberStrings(t, ctx, got.Members)) != 1 {
		t.Errorf("expected 1 member, got %v", got.Members)
	}
}

func TestImportStateCustomerScopedIDScopesRequest(t *testing.T) {
	ctx := context.Background()

	var seen string
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		seen = req.URL.Query().Get("customerid")
		if _, err := w.Write(groupJSON(t, "SUB-G-1", "SUB", "Alerts", nil)); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	})

	resp := &resource.ImportStateResponse{State: buildState(t, ctx, nil)}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "SUB:SUB-G-1"}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if seen != "SUB" {
		t.Errorf("expected customerid=SUB on the import request, got %q", seen)
	}
	if got := decodeState(t, ctx, resp.State); got.CustomerID.ValueString() != "SUB" {
		t.Errorf("expected customer_id=SUB in state, got %q", got.CustomerID.ValueString())
	}
}

func TestImportStateRejectsMalformedID(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		t.Error("no request should be sent for a malformed import ID")
	})

	resp := &resource.ImportStateResponse{State: buildState(t, ctx, nil)}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "a:b:c"}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic for a malformed import ID")
	}
}

func TestImportStateSurfacesAPIError(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	resp := &resource.ImportStateResponse{State: buildState(t, ctx, nil)}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "GONE-G-1"}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic when the group does not exist")
	}
}

// ---------------------------------------------------------------------------
// mapping helpers
// ---------------------------------------------------------------------------

func TestMembersFromSetSortsAndHandlesNull(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	got := membersFromSet(ctx, memberSet(t, "ZZZ", "AAA", "MMM"), &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(got) != 3 || got[0] != "AAA" || got[2] != "ZZZ" {
		t.Errorf("expected sorted members, got %v", got)
	}

	for _, set := range []types.Set{types.SetNull(types.StringType), types.SetUnknown(types.StringType)} {
		got := membersFromSet(ctx, set, &diags)
		if got == nil || len(got) != 0 {
			t.Errorf("expected an empty non-nil slice, got %#v", got)
		}
	}
}

func TestMembersToSetIsSortedAndNeverNull(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	set := membersToSet(ctx, nil, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if set.IsNull() || set.IsUnknown() {
		t.Fatal("an empty member list must produce a known empty set")
	}

	set = membersToSet(ctx, []string{"ZZZ", "AAA"}, &diags)
	var out []string
	if d := set.ElementsAs(ctx, &out, false); d.HasError() {
		t.Fatalf("failed to read set: %v", d)
	}
	if len(out) != 2 || out[0] != "AAA" {
		t.Errorf("expected sorted members, got %v", out)
	}
}

func TestApplyGroupToModelKeepsExplicitCustomerID(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	model := ContactGroupResourceModel{CustomerID: types.StringValue("CONFIGURED")}
	applyGroupToModel(ctx, &client.ContactGroup{
		ID:         "G1",
		CustomerID: "FROM-API",
		Name:       "n",
	}, &model, &diags)

	if model.CustomerID.ValueString() != "CONFIGURED" {
		t.Errorf("an explicit customer_id must not be overwritten, got %q", model.CustomerID.ValueString())
	}
}

func TestApplyGroupToModelLeavesCustomerIDNullWhenAPIOmitsIt(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	model := ContactGroupResourceModel{CustomerID: types.StringUnknown()}
	applyGroupToModel(ctx, &client.ContactGroup{ID: "G1", Name: "n"}, &model, &diags)

	if !model.CustomerID.IsNull() {
		t.Errorf("expected a null customer_id, got %v", model.CustomerID)
	}
}

func TestSchemaMembersPathIsAddressable(t *testing.T) {
	// Guards against the attribute being renamed without the docs following.
	s := ContactGroupSchema()
	if _, diags := s.AttributeAtPath(context.Background(), path.Root("members")); diags.HasError() {
		t.Fatalf("members must be addressable at the schema root: %v", diags)
	}
}
