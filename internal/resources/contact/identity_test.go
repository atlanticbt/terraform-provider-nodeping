package contact

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

	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/client"
)

// newIdentity builds the identity container the framework hands to a resource
// on a core that supports resource identity. Passing a nil model leaves it
// null, which is what Create and Read receive.
func newIdentity(t *testing.T, ctx context.Context, model *ContactIdentityModel) *tfsdk.ResourceIdentity {
	t.Helper()
	s := ContactIdentitySchema()
	identity := &tfsdk.ResourceIdentity{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if model != nil {
		if diags := identity.Set(ctx, model); diags.HasError() {
			t.Fatalf("failed to build identity: %v", diags)
		}
	}
	return identity
}

func decodeIdentity(t *testing.T, ctx context.Context, identity *tfsdk.ResourceIdentity) ContactIdentityModel {
	t.Helper()
	var out ContactIdentityModel
	if diags := identity.Get(ctx, &out); diags.HasError() {
		t.Fatalf("failed to read identity: %v", diags)
	}
	return out
}

func identityTestPlan() ContactResourceModel {
	return ContactResourceModel{
		ID:         types.StringUnknown(),
		CustomerID: types.StringUnknown(),
		Name:       types.StringValue("Example"),
		CustRole:   types.StringValue("notify"),
		Addresses:  []AddressModel{addrModel("", "email", "a@example.com")},
	}
}

func identityTestState() ContactResourceModel {
	return ContactResourceModel{
		ID:         types.StringValue("C1"),
		CustomerID: types.StringValue("SUB1"),
		Name:       types.StringValue("Example"),
		CustRole:   types.StringValue("notify"),
		Addresses:  []AddressModel{addrModel("A1", "email", "a@example.com")},
	}
}

func identityTestHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, req *http.Request) {
		if _, err := w.Write(contactJSON("C1", "SUB1", "Example", map[string]client.ContactAddress{
			"A1": apiAddr("email", "a@example.com"),
		})); err != nil {
			t.Errorf("write failed: %v", err)
		}
	}
}

func TestIdentitySchemaDescribesTheImportableAttributes(t *testing.T) {
	ctx := context.Background()

	resp := &resource.IdentitySchemaResponse{}
	(&ContactResource{}).IdentitySchema(ctx, resource.IdentitySchemaRequest{}, resp)
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

	r, _ := newTestResource(t, identityTestHandler(t))

	resp := &resource.CreateResponse{State: emptyState(t, ctx, nil), Identity: newIdentity(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{Plan: emptyPlan(t, ctx, identityTestPlan())}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}

	got := decodeIdentity(t, ctx, resp.Identity)
	if got.ID.ValueString() != "C1" || got.CustomerID.ValueString() != "SUB1" {
		t.Fatalf("identity after create: %+v", got)
	}
}

func TestReadPopulatesIdentity(t *testing.T) {
	ctx := context.Background()

	r, _ := newTestResource(t, identityTestHandler(t))

	state := identityTestState()
	// An identity written at create time is not carried into refresh by the
	// provider, so Read has to write it again or it disappears from state.
	resp := &resource.ReadResponse{State: emptyState(t, ctx, &state), Identity: newIdentity(t, ctx, nil)}
	r.Read(ctx, resource.ReadRequest{State: emptyState(t, ctx, &state)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	got := decodeIdentity(t, ctx, resp.Identity)
	if got.ID.ValueString() != "C1" || got.CustomerID.ValueString() != "SUB1" {
		t.Fatalf("identity after read: %+v", got)
	}
}

func TestImportStateByIdentityMatchesTheStringForm(t *testing.T) {
	ctx := context.Background()

	rString, _ := newTestResource(t, identityTestHandler(t))
	byString := &resource.ImportStateResponse{State: emptyState(t, ctx, nil), Identity: newIdentity(t, ctx, nil)}
	rString.ImportState(ctx, resource.ImportStateRequest{ID: "SUB1:C1"}, byString)
	if byString.Diagnostics.HasError() {
		t.Fatalf("string import diagnostics: %v", byString.Diagnostics)
	}

	rIdentity, _ := newTestResource(t, identityTestHandler(t))
	identity := newIdentity(t, ctx, &ContactIdentityModel{
		ID:         types.StringValue("C1"),
		CustomerID: types.StringValue("SUB1"),
	})
	byIdentity := &resource.ImportStateResponse{State: emptyState(t, ctx, nil), Identity: identity}
	rIdentity.ImportState(ctx, resource.ImportStateRequest{Identity: identity}, byIdentity)
	if byIdentity.Diagnostics.HasError() {
		t.Fatalf("identity import diagnostics: %v", byIdentity.Diagnostics)
	}

	// Identity import is additive: it must land on exactly the state the
	// colon-delimited form produces.
	if want, got := decodeState(t, ctx, byString.State), decodeState(t, ctx, byIdentity.State); !reflect.DeepEqual(want, got) {
		t.Fatalf("identity import produced different state:\nstring:   %+v\nidentity: %+v", want, got)
	}
	if got := decodeIdentity(t, ctx, byIdentity.Identity); got.ID.ValueString() != "C1" || got.CustomerID.ValueString() != "SUB1" {
		t.Fatalf("identity after import: %+v", got)
	}
}

func TestImportStateByIdentityWithoutCustomerIDTargetsTheRootAccount(t *testing.T) {
	ctx := context.Background()

	var seen string
	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		seen = req.URL.Query().Get("customerid")
		if _, err := w.Write(contactJSON("C1", "ROOT", "Example", nil)); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})

	identity := newIdentity(t, ctx, &ContactIdentityModel{
		ID:         types.StringValue("C1"),
		CustomerID: types.StringNull(),
	})
	resp := &resource.ImportStateResponse{State: emptyState(t, ctx, nil), Identity: identity}
	r.ImportState(ctx, resource.ImportStateRequest{Identity: identity}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}

	if seen != "" {
		t.Fatalf("an omitted customer_id must not scope the request, got customerid=%q", seen)
	}
	if got := decodeState(t, ctx, resp.State); got.CustomerID.ValueString() != "ROOT" {
		t.Fatalf("expected customer_id from the API, got %+v", got.CustomerID)
	}
}

func TestImportStateRejectsAnEmptyIdentityID(t *testing.T) {
	ctx := context.Background()

	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		t.Error("the API must not be called for an invalid identity")
	})

	identity := newIdentity(t, ctx, &ContactIdentityModel{
		ID:         types.StringValue(""),
		CustomerID: types.StringNull(),
	})
	resp := &resource.ImportStateResponse{State: emptyState(t, ctx, nil), Identity: identity}
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
		r, _ := newTestResource(t, identityTestHandler(t))
		resp := &resource.CreateResponse{State: emptyState(t, ctx, nil)}
		r.Create(ctx, resource.CreateRequest{Plan: emptyPlan(t, ctx, identityTestPlan())}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("create diagnostics: %v", resp.Diagnostics)
		}
	})

	t.Run("read", func(t *testing.T) {
		r, _ := newTestResource(t, identityTestHandler(t))
		state := identityTestState()
		resp := &resource.ReadResponse{State: emptyState(t, ctx, &state)}
		r.Read(ctx, resource.ReadRequest{State: emptyState(t, ctx, &state)}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("read diagnostics: %v", resp.Diagnostics)
		}
	})

	t.Run("import", func(t *testing.T) {
		r, _ := newTestResource(t, identityTestHandler(t))
		resp := &resource.ImportStateResponse{State: emptyState(t, ctx, nil)}
		r.ImportState(ctx, resource.ImportStateRequest{ID: "C1"}, resp)
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

	r, _ := newTestResource(t, identityTestHandler(t))

	identity := newIdentity(t, ctx, nil)
	resp := &resource.ImportStateResponse{State: emptyState(t, ctx, nil), Identity: identity}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "C1", Identity: identity}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}

	if got := decodeState(t, ctx, resp.State); got.ID.ValueString() != "C1" {
		t.Fatalf("unexpected state: %+v", got)
	}
	if got := decodeIdentity(t, ctx, resp.Identity); got.ID.ValueString() != "C1" {
		t.Fatalf("identity after string import: %+v", got)
	}
}

func TestKnownIdentityValueCollapsesUnknownToNull(t *testing.T) {
	if got := knownIdentityValue(types.StringUnknown()); !got.IsNull() {
		t.Fatalf("expected null, got %v", got)
	}
	if got := knownIdentityValue(types.StringValue("x")); got.ValueString() != "x" {
		t.Fatalf("expected the value to pass through, got %v", got)
	}
}
