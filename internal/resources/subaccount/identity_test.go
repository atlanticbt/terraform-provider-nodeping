package subaccount

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
func newIdentity(t *testing.T, ctx context.Context, model *SubaccountIdentityModel) *tfsdk.ResourceIdentity {
	t.Helper()
	s := SubaccountIdentitySchema()
	identity := &tfsdk.ResourceIdentity{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if model != nil {
		if diags := identity.Set(ctx, model); diags.HasError() {
			t.Fatalf("failed to build identity: %v", diags)
		}
	}
	return identity
}

func decodeIdentity(t *testing.T, ctx context.Context, identity *tfsdk.ResourceIdentity) SubaccountIdentityModel {
	t.Helper()
	var out SubaccountIdentityModel
	if diags := identity.Get(ctx, &out); diags.HasError() {
		t.Fatalf("failed to read identity: %v", diags)
	}
	return out
}

func identityReadHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(accountJSON("Active"))
	}
}

func TestIdentitySchemaDescribesOnlyTheAccountID(t *testing.T) {
	ctx := context.Background()

	resp := &resource.IdentitySchemaResponse{}
	(&SubaccountResource{}).IdentitySchema(ctx, resource.IdentitySchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("identity schema diagnostics: %v", resp.Diagnostics)
	}

	attrs := resp.IdentitySchema.Attributes
	// A subaccount *is* the account scope, so there is deliberately no
	// customer_id here.
	if len(attrs) != 1 {
		t.Fatalf("expected exactly id, got %d attributes: %v", len(attrs), attrs)
	}
	if _, ok := attrs["customer_id"]; ok {
		t.Fatal("a subaccount identity must not carry customer_id")
	}

	id, ok := attrs["id"].(identityschema.StringAttribute)
	if !ok {
		t.Fatalf("id must be a string attribute, got %T", attrs["id"])
	}
	if !id.RequiredForImport || id.OptionalForImport {
		t.Fatalf("id must be required for import, got %+v", id)
	}

	if diags := resp.IdentitySchema.ValidateImplementation(ctx); diags.HasError() {
		t.Fatalf("identity schema is not a valid implementation: %v", diags)
	}
}

func TestCreatePopulatesIdentity(t *testing.T) {
	ctx := context.Background()

	h := &recordingHandler{}
	r := newTestResource(t, h.ServeHTTP)

	resp := &resource.CreateResponse{State: emptyState(t, ctx, nil), Identity: newIdentity(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{Plan: emptyPlan(t, ctx, planModel())}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}

	if got := decodeIdentity(t, ctx, resp.Identity); got.ID.ValueString() != accountID {
		t.Fatalf("identity after create: %+v", got)
	}
}

func TestReadPopulatesIdentity(t *testing.T) {
	ctx := context.Background()

	r := newTestResource(t, identityReadHandler(t))

	prior := stateModel()
	// An identity written at create time is not carried into refresh by the
	// provider, so Read has to write it again or it disappears from state.
	resp := &resource.ReadResponse{State: emptyState(t, ctx, &prior), Identity: newIdentity(t, ctx, nil)}
	r.Read(ctx, resource.ReadRequest{State: emptyState(t, ctx, &prior)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}

	if got := decodeIdentity(t, ctx, resp.Identity); got.ID.ValueString() != accountID {
		t.Fatalf("identity after read: %+v", got)
	}
}

func TestImportStateByIdentityMatchesTheStringForm(t *testing.T) {
	ctx := context.Background()

	byString := &resource.ImportStateResponse{State: emptyState(t, ctx, nil), Identity: newIdentity(t, ctx, nil)}
	(&SubaccountResource{}).ImportState(ctx, resource.ImportStateRequest{ID: accountID}, byString)
	if byString.Diagnostics.HasError() {
		t.Fatalf("string import diagnostics: %v", byString.Diagnostics)
	}

	identity := newIdentity(t, ctx, &SubaccountIdentityModel{ID: types.StringValue(accountID)})
	byIdentity := &resource.ImportStateResponse{State: emptyState(t, ctx, nil), Identity: identity}
	(&SubaccountResource{}).ImportState(ctx, resource.ImportStateRequest{Identity: identity}, byIdentity)
	if byIdentity.Diagnostics.HasError() {
		t.Fatalf("identity import diagnostics: %v", byIdentity.Diagnostics)
	}

	// Identity import is additive: it must land on exactly the state the bare
	// string ID produces.
	if want, got := decodeState(t, ctx, byString.State), decodeState(t, ctx, byIdentity.State); !reflect.DeepEqual(want, got) {
		t.Fatalf("identity import produced different state:\nstring:   %+v\nidentity: %+v", want, got)
	}
	if got := decodeIdentity(t, ctx, byIdentity.Identity); got.ID.ValueString() != accountID {
		t.Fatalf("identity after import: %+v", got)
	}
}

func TestImportStateTrimsAndRejectsAnEmptyIdentityID(t *testing.T) {
	ctx := context.Background()

	t.Run("trims whitespace", func(t *testing.T) {
		identity := newIdentity(t, ctx, &SubaccountIdentityModel{ID: types.StringValue("  " + accountID + "  ")})
		resp := &resource.ImportStateResponse{State: emptyState(t, ctx, nil), Identity: identity}
		(&SubaccountResource{}).ImportState(ctx, resource.ImportStateRequest{Identity: identity}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("import diagnostics: %v", resp.Diagnostics)
		}
		if got := decodeState(t, ctx, resp.State); got.ID.ValueString() != accountID {
			t.Fatalf("unexpected state: %+v", got)
		}
	})

	t.Run("rejects blank", func(t *testing.T) {
		identity := newIdentity(t, ctx, &SubaccountIdentityModel{ID: types.StringValue("   ")})
		resp := &resource.ImportStateResponse{State: emptyState(t, ctx, nil), Identity: identity}
		(&SubaccountResource{}).ImportState(ctx, resource.ImportStateRequest{Identity: identity}, resp)
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected an error for a blank identity id")
		}
	})
}

// A core older than OpenTofu 1.12 never sends identity data, so every entry
// point has to cope with a nil identity rather than panicking.
func TestCRUDAndImportTolerateANilIdentity(t *testing.T) {
	ctx := context.Background()

	t.Run("create", func(t *testing.T) {
		h := &recordingHandler{}
		r := newTestResource(t, h.ServeHTTP)
		resp := &resource.CreateResponse{State: emptyState(t, ctx, nil)}
		r.Create(ctx, resource.CreateRequest{Plan: emptyPlan(t, ctx, planModel())}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("create diagnostics: %v", resp.Diagnostics)
		}
	})

	t.Run("read", func(t *testing.T) {
		r := newTestResource(t, identityReadHandler(t))
		prior := stateModel()
		resp := &resource.ReadResponse{State: emptyState(t, ctx, &prior)}
		r.Read(ctx, resource.ReadRequest{State: emptyState(t, ctx, &prior)}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("read diagnostics: %v", resp.Diagnostics)
		}
	})

	t.Run("import", func(t *testing.T) {
		resp := &resource.ImportStateResponse{State: emptyState(t, ctx, nil)}
		(&SubaccountResource{}).ImportState(ctx, resource.ImportStateRequest{ID: accountID}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("import diagnostics: %v", resp.Diagnostics)
		}
	})
}

// A core that supports identity but is importing by string ID sends a non-nil
// but null identity. That must fall through to the string handling rather than
// being mistaken for an identity import.
func TestImportStateNullIdentityFallsBackToTheStringID(t *testing.T) {
	ctx := context.Background()

	identity := newIdentity(t, ctx, nil)
	resp := &resource.ImportStateResponse{State: emptyState(t, ctx, nil), Identity: identity}
	(&SubaccountResource{}).ImportState(ctx, resource.ImportStateRequest{ID: accountID, Identity: identity}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}

	if got := decodeState(t, ctx, resp.State); got.ID.ValueString() != accountID {
		t.Fatalf("unexpected state: %+v", got)
	}
	if got := decodeIdentity(t, ctx, resp.Identity); got.ID.ValueString() != accountID {
		t.Fatalf("identity after string import: %+v", got)
	}
}
