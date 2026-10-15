package contactgroup

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
)

// newIdentity builds the identity container the framework hands to a resource
// on a core that supports resource identity. Passing a nil model leaves it
// null, which is what Create and Read receive.
func newIdentity(t *testing.T, ctx context.Context, model *ContactGroupIdentityModel) *tfsdk.ResourceIdentity {
	t.Helper()
	s := ContactGroupIdentitySchema()
	identity := &tfsdk.ResourceIdentity{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if model != nil {
		if diags := identity.Set(ctx, model); diags.HasError() {
			t.Fatalf("failed to build identity: %v", diags)
		}
	}
	return identity
}

func decodeIdentity(t *testing.T, ctx context.Context, identity *tfsdk.ResourceIdentity) ContactGroupIdentityModel {
	t.Helper()
	var out ContactGroupIdentityModel
	if diags := identity.Get(ctx, &out); diags.HasError() {
		t.Fatalf("failed to read identity: %v", diags)
	}
	return out
}

func TestIdentitySchemaDescribesTheImportableAttributes(t *testing.T) {
	ctx := context.Background()

	resp := &resource.IdentitySchemaResponse{}
	(&ContactGroupResource{}).IdentitySchema(ctx, resource.IdentitySchemaRequest{}, resp)
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

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		if _, err := w.Write(groupJSON(t, "ACC-G-ISYB7", "ACC", "Alerts", []string{"K5SP9CQP"})); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	})

	resp := &resource.CreateResponse{State: buildState(t, ctx, nil), Identity: newIdentity(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{
		Plan: buildPlan(t, ctx, ContactGroupResourceModel{
			ID:         types.StringUnknown(),
			CustomerID: types.StringUnknown(),
			Name:       types.StringValue("Alerts"),
			Members:    memberSet(t, "K5SP9CQP"),
		}),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}

	got := decodeIdentity(t, ctx, resp.Identity)
	if got.ID.ValueString() != "ACC-G-ISYB7" || got.CustomerID.ValueString() != "ACC" {
		t.Fatalf("identity after create: %+v", got)
	}
}

func TestReadPopulatesIdentity(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		if _, err := w.Write(groupJSON(t, "ACC-G-ISYB7", "ACC", "Alerts", []string{"K5SP9CQP"})); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	})

	state := ContactGroupResourceModel{
		ID:         types.StringValue("ACC-G-ISYB7"),
		CustomerID: types.StringValue("ACC"),
		Name:       types.StringValue("Alerts"),
		Members:    memberSet(t, "K5SP9CQP"),
	}

	// An identity written at create time is not carried into refresh by the
	// provider, so Read has to write it again or it disappears from state.
	resp := &resource.ReadResponse{State: buildState(t, ctx, &state), Identity: newIdentity(t, ctx, nil)}
	r.Read(ctx, resource.ReadRequest{State: buildState(t, ctx, &state)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	got := decodeIdentity(t, ctx, resp.Identity)
	if got.ID.ValueString() != "ACC-G-ISYB7" || got.CustomerID.ValueString() != "ACC" {
		t.Fatalf("identity after read: %+v", got)
	}
}

func TestImportStateByIdentityMatchesTheStringForm(t *testing.T) {
	ctx := context.Background()

	handler := func(w http.ResponseWriter, req *http.Request) {
		if _, err := w.Write(groupJSON(t, "SUB-G-1", "SUB", "Alerts", []string{"K5SP9CQP"})); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}

	byString := &resource.ImportStateResponse{State: buildState(t, ctx, nil), Identity: newIdentity(t, ctx, nil)}
	newTestResource(t, handler).ImportState(ctx, resource.ImportStateRequest{ID: "SUB:SUB-G-1"}, byString)
	if byString.Diagnostics.HasError() {
		t.Fatalf("string import diagnostics: %v", byString.Diagnostics)
	}

	identity := newIdentity(t, ctx, &ContactGroupIdentityModel{
		ID:         types.StringValue("SUB-G-1"),
		CustomerID: types.StringValue("SUB"),
	})
	byIdentity := &resource.ImportStateResponse{State: buildState(t, ctx, nil), Identity: identity}
	newTestResource(t, handler).ImportState(ctx, resource.ImportStateRequest{Identity: identity}, byIdentity)
	if byIdentity.Diagnostics.HasError() {
		t.Fatalf("identity import diagnostics: %v", byIdentity.Diagnostics)
	}

	// Identity import is additive: it must land on exactly the state the
	// colon-delimited form produces.
	if want, got := decodeState(t, ctx, byString.State), decodeState(t, ctx, byIdentity.State); !reflect.DeepEqual(want, got) {
		t.Fatalf("identity import produced different state:\nstring:   %+v\nidentity: %+v", want, got)
	}
	if got := decodeIdentity(t, ctx, byIdentity.Identity); got.ID.ValueString() != "SUB-G-1" || got.CustomerID.ValueString() != "SUB" {
		t.Fatalf("identity after import: %+v", got)
	}
}

func TestImportStateByIdentityWithoutCustomerIDTargetsTheRootAccount(t *testing.T) {
	ctx := context.Background()

	var seen string
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		seen = req.URL.Query().Get("customerid")
		if _, err := w.Write(groupJSON(t, "ACC-G-ISYB7", "ACC", "Alerts", nil)); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	})

	identity := newIdentity(t, ctx, &ContactGroupIdentityModel{
		ID:         types.StringValue("ACC-G-ISYB7"),
		CustomerID: types.StringNull(),
	})
	resp := &resource.ImportStateResponse{State: buildState(t, ctx, nil), Identity: identity}
	r.ImportState(ctx, resource.ImportStateRequest{Identity: identity}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}

	if seen != "" {
		t.Fatalf("an omitted customer_id must not scope the request, got customerid=%q", seen)
	}
	if got := decodeState(t, ctx, resp.State); got.CustomerID.ValueString() != "ACC" {
		t.Fatalf("expected customer_id from the API, got %+v", got.CustomerID)
	}
}

func TestImportStateRejectsAnEmptyIdentityID(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		t.Error("the API must not be called for an invalid identity")
	})

	identity := newIdentity(t, ctx, &ContactGroupIdentityModel{
		ID:         types.StringValue(""),
		CustomerID: types.StringNull(),
	})
	resp := &resource.ImportStateResponse{State: buildState(t, ctx, nil), Identity: identity}
	r.ImportState(ctx, resource.ImportStateRequest{Identity: identity}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for an empty identity id")
	}
}

// A core older than OpenTofu 1.12 never sends identity data, so every entry
// point has to cope with a nil identity rather than panicking.
func TestCRUDAndImportTolerateANilIdentity(t *testing.T) {
	ctx := context.Background()

	handler := func(w http.ResponseWriter, req *http.Request) {
		if _, err := w.Write(groupJSON(t, "ACC-G-ISYB7", "ACC", "Alerts", nil)); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}

	state := ContactGroupResourceModel{
		ID:         types.StringValue("ACC-G-ISYB7"),
		CustomerID: types.StringValue("ACC"),
		Name:       types.StringValue("Alerts"),
		Members:    memberSet(t),
	}

	t.Run("create", func(t *testing.T) {
		r := newTestResource(t, handler)
		resp := &resource.CreateResponse{State: buildState(t, ctx, nil)}
		r.Create(ctx, resource.CreateRequest{Plan: buildPlan(t, ctx, ContactGroupResourceModel{
			ID:         types.StringUnknown(),
			CustomerID: types.StringUnknown(),
			Name:       types.StringValue("Alerts"),
			Members:    memberSet(t),
		})}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("create diagnostics: %v", resp.Diagnostics)
		}
	})

	t.Run("read", func(t *testing.T) {
		r := newTestResource(t, handler)
		resp := &resource.ReadResponse{State: buildState(t, ctx, &state)}
		r.Read(ctx, resource.ReadRequest{State: buildState(t, ctx, &state)}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("read diagnostics: %v", resp.Diagnostics)
		}
	})

	t.Run("import", func(t *testing.T) {
		r := newTestResource(t, handler)
		resp := &resource.ImportStateResponse{State: buildState(t, ctx, nil)}
		r.ImportState(ctx, resource.ImportStateRequest{ID: "ACC-G-ISYB7"}, resp)
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

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		if _, err := w.Write(groupJSON(t, "ACC-G-ISYB7", "ACC", "Alerts", nil)); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	})

	identity := newIdentity(t, ctx, nil)
	resp := &resource.ImportStateResponse{State: buildState(t, ctx, nil), Identity: identity}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "ACC-G-ISYB7", Identity: identity}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}

	if got := decodeState(t, ctx, resp.State); got.ID.ValueString() != "ACC-G-ISYB7" {
		t.Fatalf("unexpected state: %+v", got)
	}
	if got := decodeIdentity(t, ctx, resp.Identity); got.ID.ValueString() != "ACC-G-ISYB7" {
		t.Fatalf("identity after string import: %+v", got)
	}
}
