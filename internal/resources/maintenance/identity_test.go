package maintenance

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/atlanticbt/terraform-provider-nodeping/internal/client"
)

// newIdentity builds the identity container the framework hands to a resource
// on a core that supports resource identity. Passing a nil model leaves it
// null, which is what Create and Read receive.
func newIdentity(t *testing.T, ctx context.Context, model *MaintenanceIdentityModel) *tfsdk.ResourceIdentity {
	t.Helper()
	s := MaintenanceIdentitySchema()
	identity := &tfsdk.ResourceIdentity{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if model != nil {
		if diags := identity.Set(ctx, model); diags.HasError() {
			t.Fatalf("failed to build identity: %v", diags)
		}
	}
	return identity
}

func decodeIdentity(t *testing.T, ctx context.Context, identity *tfsdk.ResourceIdentity) MaintenanceIdentityModel {
	t.Helper()
	var out MaintenanceIdentityModel
	if diags := identity.Get(ctx, &out); diags.HasError() {
		t.Fatalf("failed to read identity: %v", diags)
	}
	return out
}

// identityTestHandler answers every request with the same sub-account scoped
// maintenance window.
func identityTestHandler(t *testing.T, customerID string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write(maintenanceBody(t, client.Maintenance{
			ID: "9VKOB", CustomerID: customerID, Name: "Weekly patching", Enabled: true,
			Cron: "0 3 * * 0", Duration: 60, Checklist: []string{"C1", "C2"},
		}))
	}
}

func TestIdentitySchemaDescribesTheImportableAttributes(t *testing.T) {
	ctx := context.Background()

	resp := &resource.IdentitySchemaResponse{}
	(&MaintenanceResource{}).IdentitySchema(ctx, resource.IdentitySchemaRequest{}, resp)
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

func TestCreatePopulatesIdentity(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, identityTestHandler(t, "SUB1"))

	resp := &resource.CreateResponse{State: testState(t, ctx, nil), Identity: newIdentity(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{Plan: testPlan(t, ctx, plannedModel(t))}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}

	got := decodeIdentity(t, ctx, resp.Identity)
	if got.ID.ValueString() != "9VKOB" {
		t.Fatalf("identity after create: %+v", got)
	}
}

func TestReadPopulatesIdentity(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, identityTestHandler(t, ""))

	state := createdState(t)
	// An identity written at create time is not carried into refresh by the
	// provider, so Read has to write it again or it disappears from state.
	resp := &resource.ReadResponse{State: testState(t, ctx, state), Identity: newIdentity(t, ctx, nil)}
	r.Read(ctx, resource.ReadRequest{State: testState(t, ctx, state)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	got := decodeIdentity(t, ctx, resp.Identity)
	if got.ID.ValueString() != "9VKOB" {
		t.Fatalf("identity after read: %+v", got)
	}
}

func TestImportStateByIdentityMatchesTheStringForm(t *testing.T) {
	ctx := context.Background()

	byString := &resource.ImportStateResponse{State: testState(t, ctx, nil), Identity: newIdentity(t, ctx, nil)}
	newTestResource(t, identityTestHandler(t, "SUB1")).
		ImportState(ctx, resource.ImportStateRequest{ID: "SUB1:9VKOB"}, byString)
	if byString.Diagnostics.HasError() {
		t.Fatalf("string import diagnostics: %v", byString.Diagnostics)
	}

	identity := newIdentity(t, ctx, &MaintenanceIdentityModel{
		ID:         types.StringValue("9VKOB"),
		CustomerID: types.StringValue("SUB1"),
	})
	byIdentity := &resource.ImportStateResponse{State: testState(t, ctx, nil), Identity: identity}
	newTestResource(t, identityTestHandler(t, "SUB1")).
		ImportState(ctx, resource.ImportStateRequest{Identity: identity}, byIdentity)
	if byIdentity.Diagnostics.HasError() {
		t.Fatalf("identity import diagnostics: %v", byIdentity.Diagnostics)
	}

	// Identity import is additive: it must land on exactly the state the
	// colon-delimited form produces.
	if want, got := decodeState(t, ctx, byString.State), decodeState(t, ctx, byIdentity.State); !reflect.DeepEqual(want, got) {
		t.Fatalf("identity import produced different state:\nstring:   %+v\nidentity: %+v", want, got)
	}
	if got := decodeIdentity(t, ctx, byIdentity.Identity); got.ID.ValueString() != "9VKOB" || got.CustomerID.ValueString() != "SUB1" {
		t.Fatalf("identity after import: %+v", got)
	}
}

func TestImportStateByIdentityWithoutCustomerIDTargetsTheRootAccount(t *testing.T) {
	ctx := context.Background()

	var seen string
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		seen = req.URL.Query().Get("customerid")
		identityTestHandler(t, "SUB2")(w, req)
	})

	identity := newIdentity(t, ctx, &MaintenanceIdentityModel{
		ID:         types.StringValue("9VKOB"),
		CustomerID: types.StringNull(),
	})
	resp := &resource.ImportStateResponse{State: testState(t, ctx, nil), Identity: identity}
	r.ImportState(ctx, resource.ImportStateRequest{Identity: identity}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}

	if seen != "" {
		t.Fatalf("an omitted customer_id must not scope the request, got customerid=%q", seen)
	}
	// The window's real account is adopted from the response, and it must end
	// up in the identity too so the next refresh addresses the same account.
	if got := decodeIdentity(t, ctx, resp.Identity); got.CustomerID.ValueString() != "SUB2" {
		t.Fatalf("identity after import: %+v", got)
	}
}

func TestImportStateRejectsAnEmptyIdentityID(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		t.Error("the API must not be called for an invalid identity")
	})

	identity := newIdentity(t, ctx, &MaintenanceIdentityModel{
		ID:         types.StringValue(""),
		CustomerID: types.StringNull(),
	})
	resp := &resource.ImportStateResponse{State: testState(t, ctx, nil), Identity: identity}
	r.ImportState(ctx, resource.ImportStateRequest{Identity: identity}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for an empty identity id")
	}
}

// A core older than OpenTofu 1.12 never sends identity data, so every entry
// point has to cope with a nil identity rather than panicking.
func TestCRUDAndImportTolerateANilIdentity(t *testing.T) {
	ctx := context.Background()

	t.Run("create", func(t *testing.T) {
		r := newTestResource(t, identityTestHandler(t, ""))
		resp := &resource.CreateResponse{State: testState(t, ctx, nil)}
		r.Create(ctx, resource.CreateRequest{Plan: testPlan(t, ctx, plannedModel(t))}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("create diagnostics: %v", resp.Diagnostics)
		}
	})

	t.Run("read", func(t *testing.T) {
		r := newTestResource(t, identityTestHandler(t, ""))
		state := createdState(t)
		resp := &resource.ReadResponse{State: testState(t, ctx, state)}
		r.Read(ctx, resource.ReadRequest{State: testState(t, ctx, state)}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("read diagnostics: %v", resp.Diagnostics)
		}
	})

	t.Run("import", func(t *testing.T) {
		r := newTestResource(t, identityTestHandler(t, ""))
		resp := &resource.ImportStateResponse{State: testState(t, ctx, nil)}
		r.ImportState(ctx, resource.ImportStateRequest{ID: "9VKOB"}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("import diagnostics: %v", resp.Diagnostics)
		}
	})
}

// A core that supports identity but is importing by string ID sends a non-nil
// but null identity. That must fall through to the string parsing rather than
// being mistaken for an identity import.
func TestImportStateNullIdentityFallsBackToTheStringID(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, identityTestHandler(t, ""))

	identity := newIdentity(t, ctx, nil)
	resp := &resource.ImportStateResponse{State: testState(t, ctx, nil), Identity: identity}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "9VKOB", Identity: identity}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}

	if got := decodeState(t, ctx, resp.State); got.ID.ValueString() != "9VKOB" {
		t.Fatalf("unexpected state: %+v", got)
	}
	if got := decodeIdentity(t, ctx, resp.Identity); got.ID.ValueString() != "9VKOB" {
		t.Fatalf("identity after string import: %+v", got)
	}
}
