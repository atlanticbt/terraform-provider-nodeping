package check

import (
	"context"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// newIdentity builds the identity container the framework hands to a resource
// on a core that supports resource identity. Passing a nil model leaves it
// null, which is what Create and Read receive.
func newIdentity(t *testing.T, ctx context.Context, model *CheckIdentityModel) *tfsdk.ResourceIdentity {
	t.Helper()
	s := CheckIdentitySchema()
	identity := &tfsdk.ResourceIdentity{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if model != nil {
		if diags := identity.Set(ctx, model); diags.HasError() {
			t.Fatalf("failed to build identity: %v", diags)
		}
	}
	return identity
}

func decodeIdentity(t *testing.T, ctx context.Context, identity *tfsdk.ResourceIdentity) CheckIdentityModel {
	t.Helper()
	var out CheckIdentityModel
	if diags := identity.Get(ctx, &out); diags.HasError() {
		t.Fatalf("failed to read identity: %v", diags)
	}
	return out
}

// storeCheck seeds the mock API with a minimal check owned by SUB-1.
func storeCheck(api *mockAPI, id string) {
	api.stored[id] = map[string]interface{}{
		"_id":         id,
		"customer_id": "SUB-1",
		"type":        "HTTP",
		"label":       "web",
		"enable":      "active",
		"interval":    float64(5),
		"parameters":  map[string]interface{}{"target": "https://example.com"},
	}
}

func TestIdentitySchema_DescribesTheImportableAttributes(t *testing.T) {
	ctx := testContext()

	resp := &resource.IdentitySchemaResponse{}
	(&CheckResource{}).IdentitySchema(ctx, resource.IdentitySchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("identity schema diagnostics: %v", resp.Diagnostics)
	}

	attrs := resp.IdentitySchema.Attributes
	if len(attrs) != 2 {
		t.Fatalf("expected exactly id and customer_id, got %d attributes: %v", len(attrs), attrs)
	}

	id, ok := attrs["id"].(identityschema.StringAttribute)
	if !ok {
		t.Fatalf("id must be a string attribute, got %T", attrs["id"])
	}
	if !id.RequiredForImport || id.OptionalForImport {
		t.Fatalf("id must be required for import, got %+v", id)
	}

	customerID, ok := attrs["customer_id"].(identityschema.StringAttribute)
	if !ok {
		t.Fatalf("customer_id must be a string attribute, got %T", attrs["customer_id"])
	}
	// Omitting customer_id must remain legal: that is how the root account is
	// addressed.
	if !customerID.OptionalForImport || customerID.RequiredForImport {
		t.Fatalf("customer_id must be optional for import, got %+v", customerID)
	}

	if diags := resp.IdentitySchema.ValidateImplementation(ctx); diags.HasError() {
		t.Fatalf("identity schema is not a valid implementation: %v", diags)
	}
}

func TestCreate_PopulatesIdentity(t *testing.T) {
	ctx := testContext()
	api := newMockAPI(t)
	r := api.resource()

	resp := &resource.CreateResponse{State: emptyState(ctx), Identity: newIdentity(t, ctx, nil)}
	r.Create(ctx, newCreateRequest(t, ctx, baseModel()), resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}

	got := decodeIdentity(t, ctx, resp.Identity)
	if got.ID.ValueString() != "MOCK-CHECK-1" {
		t.Fatalf("identity id: %v", got.ID)
	}
	if got.CustomerID.ValueString() != "ACCT-1" {
		t.Fatalf("identity customer_id: %v", got.CustomerID)
	}
}

func TestRead_PopulatesIdentity(t *testing.T) {
	ctx := testContext()
	api := newMockAPI(t)
	r := api.resource()
	storeCheck(api, "CHK-1")

	prior := baseModel()
	prior.ID = types.StringValue("CHK-1")
	prior.CustomerID = types.StringValue("SUB-1")

	// An identity written at create time is not carried into refresh by the
	// provider, so Read has to write it again or it disappears from state.
	resp := &resource.ReadResponse{State: emptyState(ctx), Identity: newIdentity(t, ctx, nil)}
	r.Read(ctx, resource.ReadRequest{State: newState(t, ctx, prior)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	got := decodeIdentity(t, ctx, resp.Identity)
	if got.ID.ValueString() != "CHK-1" || got.CustomerID.ValueString() != "SUB-1" {
		t.Fatalf("identity after read: %+v", got)
	}
}

func TestImportState_ByIdentityMatchesTheStringForm(t *testing.T) {
	ctx := testContext()

	stringAPI := newMockAPI(t)
	storeCheck(stringAPI, "CHK-1")
	byString := &resource.ImportStateResponse{State: emptyState(ctx), Identity: newIdentity(t, ctx, nil)}
	stringAPI.resource().ImportState(ctx, resource.ImportStateRequest{ID: "SUB-1:CHK-1"}, byString)
	if byString.Diagnostics.HasError() {
		t.Fatalf("string import diagnostics: %v", byString.Diagnostics)
	}

	identityAPI := newMockAPI(t)
	storeCheck(identityAPI, "CHK-1")
	identity := newIdentity(t, ctx, &CheckIdentityModel{
		ID:         types.StringValue("CHK-1"),
		CustomerID: types.StringValue("SUB-1"),
	})
	byIdentity := &resource.ImportStateResponse{State: emptyState(ctx), Identity: identity}
	identityAPI.resource().ImportState(ctx, resource.ImportStateRequest{Identity: identity}, byIdentity)
	if byIdentity.Diagnostics.HasError() {
		t.Fatalf("identity import diagnostics: %v", byIdentity.Diagnostics)
	}

	// Identity import is additive: it must land on exactly the state the
	// colon-delimited form produces.
	want := stateModel(t, ctx, byString.State)
	got := stateModel(t, ctx, byIdentity.State)
	// `modified` is a server timestamp that moves between the two calls.
	want.Modified, got.Modified = types.Int64Null(), types.Int64Null()
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("identity import produced different state:\nstring:   %+v\nidentity: %+v", want, got)
	}
	if id := decodeIdentity(t, ctx, byIdentity.Identity); id.ID.ValueString() != "CHK-1" || id.CustomerID.ValueString() != "SUB-1" {
		t.Fatalf("identity after import: %+v", id)
	}
}

func TestImportState_ByIdentityWithoutCustomerIDTargetsTheRootAccount(t *testing.T) {
	ctx := testContext()
	api := newMockAPI(t)
	storeCheck(api, "CHK-1")

	identity := newIdentity(t, ctx, &CheckIdentityModel{
		ID:         types.StringValue("CHK-1"),
		CustomerID: types.StringNull(),
	})
	resp := &resource.ImportStateResponse{State: emptyState(ctx), Identity: identity}
	api.resource().ImportState(ctx, resource.ImportStateRequest{Identity: identity}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}

	if api.lastQuery != "" {
		t.Fatalf("an omitted customer_id must not scope the request, got %q", api.lastQuery)
	}
	// The check's real account comes back from the API and must reach the
	// identity, otherwise the next refresh addresses the wrong account.
	if got := decodeIdentity(t, ctx, resp.Identity); got.CustomerID.ValueString() != "SUB-1" {
		t.Fatalf("identity after import: %+v", got)
	}
}

func TestImportState_RejectsAnEmptyIdentityID(t *testing.T) {
	ctx := testContext()
	api := newMockAPI(t)

	identity := newIdentity(t, ctx, &CheckIdentityModel{
		ID:         types.StringValue(""),
		CustomerID: types.StringNull(),
	})
	resp := &resource.ImportStateResponse{State: emptyState(ctx), Identity: identity}
	api.resource().ImportState(ctx, resource.ImportStateRequest{Identity: identity}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for an empty identity id")
	}
}

// A core older than OpenTofu 1.12 never sends identity data, so every entry
// point has to cope with a nil identity rather than panicking.
func TestCRUDAndImport_TolerateANilIdentity(t *testing.T) {
	ctx := testContext()

	t.Run("create", func(t *testing.T) {
		api := newMockAPI(t)
		resp := &resource.CreateResponse{State: emptyState(ctx)}
		api.resource().Create(ctx, newCreateRequest(t, ctx, baseModel()), resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("create diagnostics: %v", resp.Diagnostics)
		}
	})

	t.Run("read", func(t *testing.T) {
		api := newMockAPI(t)
		storeCheck(api, "CHK-1")
		prior := baseModel()
		prior.ID = types.StringValue("CHK-1")
		prior.CustomerID = types.StringValue("SUB-1")
		resp := &resource.ReadResponse{State: emptyState(ctx)}
		api.resource().Read(ctx, resource.ReadRequest{State: newState(t, ctx, prior)}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("read diagnostics: %v", resp.Diagnostics)
		}
	})

	t.Run("import", func(t *testing.T) {
		api := newMockAPI(t)
		storeCheck(api, "CHK-1")
		resp := &resource.ImportStateResponse{State: emptyState(ctx)}
		api.resource().ImportState(ctx, resource.ImportStateRequest{ID: "CHK-1"}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("import diagnostics: %v", resp.Diagnostics)
		}
	})
}

// A core that supports identity but is importing by string ID sends a non-nil
// but null identity. That must fall through to the string parsing rather than
// being mistaken for an identity import.
func TestImportState_NullIdentityFallsBackToTheStringID(t *testing.T) {
	ctx := testContext()
	api := newMockAPI(t)
	storeCheck(api, "CHK-1")

	identity := newIdentity(t, ctx, nil)
	resp := &resource.ImportStateResponse{State: emptyState(ctx), Identity: identity}
	api.resource().ImportState(ctx, resource.ImportStateRequest{ID: "CHK-1", Identity: identity}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}

	if got := stateModel(t, ctx, resp.State); got.ID.ValueString() != "CHK-1" {
		t.Fatalf("unexpected state: %+v", got.ID)
	}
	if got := decodeIdentity(t, ctx, resp.Identity); got.ID.ValueString() != "CHK-1" {
		t.Fatalf("identity after string import: %+v", got)
	}
}
