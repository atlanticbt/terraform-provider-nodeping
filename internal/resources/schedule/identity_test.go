package schedule

import (
	"context"
	"io"
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
func newIdentity(t *testing.T, ctx context.Context, model *ScheduleIdentityModel) *tfsdk.ResourceIdentity {
	t.Helper()
	s := ScheduleIdentitySchema()
	identity := &tfsdk.ResourceIdentity{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if model != nil {
		if diags := identity.Set(ctx, model); diags.HasError() {
			t.Fatalf("failed to build identity: %v", diags)
		}
	}
	return identity
}

func decodeIdentity(t *testing.T, ctx context.Context, identity *tfsdk.ResourceIdentity) ScheduleIdentityModel {
	t.Helper()
	var out ScheduleIdentityModel
	if diags := identity.Get(ctx, &out); diags.HasError() {
		t.Fatalf("failed to read identity: %v", diags)
	}
	return out
}

func TestIdentitySchema_DescribesTheImportableAttributes(t *testing.T) {
	ctx := context.Background()

	resp := &resource.IdentitySchemaResponse{}
	(&ScheduleResource{}).IdentitySchema(ctx, resource.IdentitySchemaRequest{}, resp)
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
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		if _, err := io.WriteString(w, `{"id":"BusinessHours","customer_id":"SUB1","data":{"monday":{"time1":"8:00","time2":"18:00"}}}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})

	resp := &resource.CreateResponse{State: newState(t, ctx, nil), Identity: newIdentity(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{Plan: newPlan(t, ctx, businessHoursPlan())}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}

	got := decodeIdentity(t, ctx, resp.Identity)
	if got.ID.ValueString() != "BusinessHours" {
		t.Fatalf("identity id: %v", got.ID)
	}
	if got.CustomerID.ValueString() != "SUB1" {
		t.Fatalf("identity customer_id: %v", got.CustomerID)
	}
}

func TestRead_PopulatesIdentity(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		if _, err := io.WriteString(w, `{"id":"BusinessHours","customer_id":"CUSTFROMAPI","data":{"monday":{"time1":"7:00","time2":"19:00"}}}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})

	state := businessHoursPlan()
	// An identity written at create time is not carried into refresh by the
	// provider, so Read has to write it again or it disappears from state.
	resp := &resource.ReadResponse{State: newState(t, ctx, &state), Identity: newIdentity(t, ctx, nil)}
	r.Read(ctx, resource.ReadRequest{State: newState(t, ctx, &state)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	got := decodeIdentity(t, ctx, resp.Identity)
	if got.ID.ValueString() != "BusinessHours" || got.CustomerID.ValueString() != "CUSTFROMAPI" {
		t.Fatalf("identity after read: %+v", got)
	}
}

func TestImportState_ByIdentityMatchesTheStringForm(t *testing.T) {
	ctx := context.Background()

	handler := func(w http.ResponseWriter, req *http.Request) {
		if _, err := io.WriteString(w, `{"id":"BusinessHours","customer_id":"SUB1","data":{"monday":{"time1":"8:00","time2":"18:00"}}}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	}

	byString := &resource.ImportStateResponse{State: newState(t, ctx, nil), Identity: newIdentity(t, ctx, nil)}
	newTestResource(t, handler).ImportState(ctx, resource.ImportStateRequest{ID: "SUB1:BusinessHours"}, byString)
	if byString.Diagnostics.HasError() {
		t.Fatalf("string import diagnostics: %v", byString.Diagnostics)
	}

	byIdentity := &resource.ImportStateResponse{State: newState(t, ctx, nil)}
	byIdentity.Identity = newIdentity(t, ctx, &ScheduleIdentityModel{
		ID:         types.StringValue("BusinessHours"),
		CustomerID: types.StringValue("SUB1"),
	})
	newTestResource(t, handler).ImportState(ctx, resource.ImportStateRequest{Identity: byIdentity.Identity}, byIdentity)
	if byIdentity.Diagnostics.HasError() {
		t.Fatalf("identity import diagnostics: %v", byIdentity.Diagnostics)
	}

	// Identity import is additive: it must land on exactly the state the
	// colon-delimited form produces.
	if want, got := decodeState(t, ctx, byString.State), decodeState(t, ctx, byIdentity.State); !reflect.DeepEqual(want, got) {
		t.Fatalf("identity import produced different state:\nstring:   %+v\nidentity: %+v", want, got)
	}
	if got := decodeIdentity(t, ctx, byIdentity.Identity); got.ID.ValueString() != "BusinessHours" || got.CustomerID.ValueString() != "SUB1" {
		t.Fatalf("identity after import: %+v", got)
	}
}

func TestImportState_ByIdentityWithoutCustomerIDTargetsTheRootAccount(t *testing.T) {
	ctx := context.Background()

	var query string
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		query = req.URL.RawQuery
		if _, err := io.WriteString(w, `{"id":"BusinessHours","data":{"monday":{"time1":"8:00","time2":"18:00"}}}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})

	identity := newIdentity(t, ctx, &ScheduleIdentityModel{
		ID:         types.StringValue("BusinessHours"),
		CustomerID: types.StringNull(),
	})
	resp := &resource.ImportStateResponse{State: newState(t, ctx, nil), Identity: identity}
	r.ImportState(ctx, resource.ImportStateRequest{Identity: identity}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}

	if query != "" {
		t.Fatalf("an omitted customer_id must not scope the request, got query %q", query)
	}
	if got := decodeState(t, ctx, resp.State); got.ID.ValueString() != "BusinessHours" {
		t.Fatalf("unexpected state: %+v", got)
	}
}

func TestImportState_RejectsAnEmptyIdentityID(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		t.Error("the API must not be called for an invalid identity")
	})

	identity := newIdentity(t, ctx, &ScheduleIdentityModel{
		ID:         types.StringValue(""),
		CustomerID: types.StringNull(),
	})
	resp := &resource.ImportStateResponse{State: newState(t, ctx, nil), Identity: identity}
	r.ImportState(ctx, resource.ImportStateRequest{Identity: identity}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for an empty identity id")
	}
}

// A core older than OpenTofu 1.12 never sends identity data, so every entry
// point has to cope with a nil identity rather than panicking.
func TestCRUDAndImport_TolerateANilIdentity(t *testing.T) {
	ctx := context.Background()

	handler := func(w http.ResponseWriter, req *http.Request) {
		if _, err := io.WriteString(w, `{"id":"BusinessHours","data":{"monday":{"time1":"8:00","time2":"18:00"}}}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	}

	t.Run("create", func(t *testing.T) {
		r := newTestResource(t, handler)
		resp := &resource.CreateResponse{State: newState(t, ctx, nil)}
		r.Create(ctx, resource.CreateRequest{Plan: newPlan(t, ctx, businessHoursPlan())}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("create diagnostics: %v", resp.Diagnostics)
		}
	})

	t.Run("read", func(t *testing.T) {
		r := newTestResource(t, handler)
		state := businessHoursPlan()
		resp := &resource.ReadResponse{State: newState(t, ctx, &state)}
		r.Read(ctx, resource.ReadRequest{State: newState(t, ctx, &state)}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("read diagnostics: %v", resp.Diagnostics)
		}
	})

	t.Run("import", func(t *testing.T) {
		r := newTestResource(t, handler)
		resp := &resource.ImportStateResponse{State: newState(t, ctx, nil)}
		r.ImportState(ctx, resource.ImportStateRequest{ID: "BusinessHours"}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("import diagnostics: %v", resp.Diagnostics)
		}
	})
}

// A core that supports identity but is importing by string ID sends a non-nil
// but null identity. That must fall through to the string parsing rather than
// being mistaken for an identity import.
func TestImportState_NullIdentityFallsBackToTheStringID(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		if _, err := io.WriteString(w, `{"id":"BusinessHours","data":{"monday":{"time1":"8:00","time2":"18:00"}}}`); err != nil {
			t.Errorf("write failed: %v", err)
		}
	})

	identity := newIdentity(t, ctx, nil)
	resp := &resource.ImportStateResponse{State: newState(t, ctx, nil), Identity: identity}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "BusinessHours", Identity: identity}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}

	if got := decodeState(t, ctx, resp.State); got.ID.ValueString() != "BusinessHours" {
		t.Fatalf("unexpected state: %+v", got)
	}
	if got := decodeIdentity(t, ctx, resp.Identity); got.ID.ValueString() != "BusinessHours" {
		t.Fatalf("identity after string import: %+v", got)
	}
}
