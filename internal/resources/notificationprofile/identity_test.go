package notificationprofile

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
func newIdentity(t *testing.T, ctx context.Context, model *NotificationProfileIdentityModel) *tfsdk.ResourceIdentity {
	t.Helper()
	s := NotificationProfileIdentitySchema()
	identity := &tfsdk.ResourceIdentity{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if model != nil {
		if diags := identity.Set(ctx, model); diags.HasError() {
			t.Fatalf("failed to build identity: %v", diags)
		}
	}
	return identity
}

func decodeIdentity(t *testing.T, ctx context.Context, identity *tfsdk.ResourceIdentity) NotificationProfileIdentityModel {
	t.Helper()
	var out NotificationProfileIdentityModel
	if diags := identity.Get(ctx, &out); diags.HasError() {
		t.Fatalf("failed to read identity: %v", diags)
	}
	return out
}

func identityTestHandler(t *testing.T, customerID string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(profileJSON(t, "ACC-P-1", customerID, "Escalation", []map[string]interface{}{
			apiNotif("A-1", 0, "All"),
		}))
	}
}

func identityTestModel(id, customerID types.String) NotificationProfileResourceModel {
	return NotificationProfileResourceModel{
		ID:            id,
		CustomerID:    customerID,
		Name:          types.StringValue("Escalation"),
		Notifications: []NotificationModel{notif("A-1", 0, "All")},
	}
}

func TestIdentitySchemaDescribesTheImportableAttributes(t *testing.T) {
	ctx := context.Background()

	resp := &resource.IdentitySchemaResponse{}
	(&NotificationProfileResource{}).IdentitySchema(ctx, resource.IdentitySchemaRequest{}, resp)
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

	r, _ := newTestResource(t, identityTestHandler(t, "SUB1"))

	plan := identityTestModel(types.StringUnknown(), types.StringUnknown())
	resp := &resource.CreateResponse{State: testState(t, ctx, nil), Identity: newIdentity(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{Plan: testPlan(t, ctx, plan)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}

	got := decodeIdentity(t, ctx, resp.Identity)
	if got.ID.ValueString() != "ACC-P-1" || got.CustomerID.ValueString() != "SUB1" {
		t.Fatalf("identity after create: %+v", got)
	}
}

func TestReadPopulatesIdentity(t *testing.T) {
	ctx := context.Background()

	r, _ := newTestResource(t, identityTestHandler(t, "SUB1"))

	state := identityTestModel(types.StringValue("ACC-P-1"), types.StringValue("SUB1"))
	// An identity written at create time is not carried into refresh by the
	// provider, so Read has to write it again or it disappears from state.
	resp := &resource.ReadResponse{State: testState(t, ctx, &state), Identity: newIdentity(t, ctx, nil)}
	r.Read(ctx, resource.ReadRequest{State: testState(t, ctx, &state)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	got := decodeIdentity(t, ctx, resp.Identity)
	if got.ID.ValueString() != "ACC-P-1" || got.CustomerID.ValueString() != "SUB1" {
		t.Fatalf("identity after read: %+v", got)
	}
}

func TestImportStateByIdentityMatchesTheStringForm(t *testing.T) {
	ctx := context.Background()

	rString, _ := newTestResource(t, identityTestHandler(t, "SUB1"))
	byString := &resource.ImportStateResponse{State: testState(t, ctx, nil), Identity: newIdentity(t, ctx, nil)}
	rString.ImportState(ctx, resource.ImportStateRequest{ID: "SUB1:ACC-P-1"}, byString)
	if byString.Diagnostics.HasError() {
		t.Fatalf("string import diagnostics: %v", byString.Diagnostics)
	}

	rIdentity, _ := newTestResource(t, identityTestHandler(t, "SUB1"))
	identity := newIdentity(t, ctx, &NotificationProfileIdentityModel{
		ID:         types.StringValue("ACC-P-1"),
		CustomerID: types.StringValue("SUB1"),
	})
	byIdentity := &resource.ImportStateResponse{State: testState(t, ctx, nil), Identity: identity}
	rIdentity.ImportState(ctx, resource.ImportStateRequest{Identity: identity}, byIdentity)
	if byIdentity.Diagnostics.HasError() {
		t.Fatalf("identity import diagnostics: %v", byIdentity.Diagnostics)
	}

	// Identity import is additive: it must land on exactly the state the
	// colon-delimited form produces.
	if want, got := decodeState(t, ctx, byString.State), decodeState(t, ctx, byIdentity.State); !reflect.DeepEqual(want, got) {
		t.Fatalf("identity import produced different state:\nstring:   %+v\nidentity: %+v", want, got)
	}
	if got := decodeIdentity(t, ctx, byIdentity.Identity); got.ID.ValueString() != "ACC-P-1" || got.CustomerID.ValueString() != "SUB1" {
		t.Fatalf("identity after import: %+v", got)
	}
}

func TestImportStateByIdentityWithoutCustomerIDTargetsTheRootAccount(t *testing.T) {
	ctx := context.Background()

	var query string
	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		query = req.URL.Query().Get("customerid")
		identityTestHandler(t, "ROOT")(w, req)
	})

	identity := newIdentity(t, ctx, &NotificationProfileIdentityModel{
		ID:         types.StringValue("ACC-P-1"),
		CustomerID: types.StringNull(),
	})
	resp := &resource.ImportStateResponse{State: testState(t, ctx, nil), Identity: identity}
	r.ImportState(ctx, resource.ImportStateRequest{Identity: identity}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}

	if query != "" {
		t.Fatalf("an omitted customer_id must not scope the request, got customerid=%q", query)
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

	for _, id := range []string{"", "   "} {
		identity := newIdentity(t, ctx, &NotificationProfileIdentityModel{
			ID:         types.StringValue(id),
			CustomerID: types.StringNull(),
		})
		resp := &resource.ImportStateResponse{State: testState(t, ctx, nil), Identity: identity}
		r.ImportState(ctx, resource.ImportStateRequest{Identity: identity}, resp)
		if !resp.Diagnostics.HasError() {
			t.Fatalf("expected identity id %q to be rejected", id)
		}
	}
}

// A core older than OpenTofu 1.12 never sends identity data, so every entry
// point has to cope with a nil identity rather than panicking.
func TestCRUDAndImportTolerateANilIdentity(t *testing.T) {
	ctx := context.Background()

	t.Run("create", func(t *testing.T) {
		r, _ := newTestResource(t, identityTestHandler(t, "ROOT"))
		plan := identityTestModel(types.StringUnknown(), types.StringUnknown())
		resp := &resource.CreateResponse{State: testState(t, ctx, nil)}
		r.Create(ctx, resource.CreateRequest{Plan: testPlan(t, ctx, plan)}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("create diagnostics: %v", resp.Diagnostics)
		}
	})

	t.Run("read", func(t *testing.T) {
		r, _ := newTestResource(t, identityTestHandler(t, "ROOT"))
		state := identityTestModel(types.StringValue("ACC-P-1"), types.StringValue("ROOT"))
		resp := &resource.ReadResponse{State: testState(t, ctx, &state)}
		r.Read(ctx, resource.ReadRequest{State: testState(t, ctx, &state)}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("read diagnostics: %v", resp.Diagnostics)
		}
	})

	t.Run("import", func(t *testing.T) {
		r, _ := newTestResource(t, identityTestHandler(t, "ROOT"))
		resp := &resource.ImportStateResponse{State: testState(t, ctx, nil)}
		r.ImportState(ctx, resource.ImportStateRequest{ID: "ACC-P-1"}, resp)
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

	r, _ := newTestResource(t, identityTestHandler(t, "ROOT"))

	identity := newIdentity(t, ctx, nil)
	resp := &resource.ImportStateResponse{State: testState(t, ctx, nil), Identity: identity}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "ACC-P-1", Identity: identity}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}

	if got := decodeState(t, ctx, resp.State); got.ID.ValueString() != "ACC-P-1" {
		t.Fatalf("unexpected state: %+v", got)
	}
	if got := decodeIdentity(t, ctx, resp.Identity); got.ID.ValueString() != "ACC-P-1" {
		t.Fatalf("identity after string import: %+v", got)
	}
}
