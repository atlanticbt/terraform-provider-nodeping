package identity

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func emptyIdentity(ctx context.Context, s identityschema.Schema) *tfsdk.ResourceIdentity {
	return &tfsdk.ResourceIdentity{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
}

func identityWith(t *testing.T, ctx context.Context, s identityschema.Schema, model any) *tfsdk.ResourceIdentity {
	t.Helper()
	id := emptyIdentity(ctx, s)
	if diags := id.Set(ctx, model); diags.HasError() {
		t.Fatalf("building identity: %v", diags)
	}
	return id
}

func TestKnownValue_ShouldCollapseUnknownToNullAndPassOthersThrough(t *testing.T) {
	if got := KnownValue(types.StringUnknown()); !got.IsNull() {
		t.Errorf("unknown: got %v, want null", got)
	}
	if got := KnownValue(types.StringValue("x")); got.ValueString() != "x" {
		t.Errorf("known: got %v, want x", got)
	}
	if got := KnownValue(types.StringNull()); !got.IsNull() {
		t.Errorf("null: got %v, want null", got)
	}
}

func TestSchema_ShouldRequireIDAndMakeCustomerIDOptional(t *testing.T) {
	s := Schema("contact group")
	if diags := s.ValidateImplementation(context.Background()); diags.HasError() {
		t.Fatalf("schema invalid: %v", diags)
	}
	id := s.Attributes["id"].(identityschema.StringAttribute)
	cust := s.Attributes["customer_id"].(identityschema.StringAttribute)
	if !id.RequiredForImport || !cust.OptionalForImport {
		t.Errorf("id required=%v, customer_id optional=%v", id.RequiredForImport, cust.OptionalForImport)
	}
	if !strings.Contains(id.Description, "contact group") || !strings.Contains(cust.Description, "contact group") {
		t.Errorf("descriptions should name the noun: %q / %q", id.Description, cust.Description)
	}
}

func TestSet_ShouldWriteKnownValuesAndIgnoreANilIdentity(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	Set(ctx, nil, types.StringValue("a"), types.StringValue("b"), &diags)
	if diags.HasError() {
		t.Fatalf("nil identity must be a no-op: %v", diags)
	}

	ri := emptyIdentity(ctx, Schema("check"))
	Set(ctx, ri, types.StringValue("ID1"), types.StringUnknown(), &diags)
	var got Model
	diags.Append(ri.Get(ctx, &got)...)
	if diags.HasError() || got.ID.ValueString() != "ID1" || !got.CustomerID.IsNull() {
		t.Fatalf("got %+v, %v; want ID1 with a null customer_id", got, diags)
	}
}

func TestFromImport(t *testing.T) {
	ctx := context.Background()
	s := Schema("check")

	t.Run("should defer to the string form when an import ID is given", func(t *testing.T) {
		var diags diag.Diagnostics
		_, _, from := FromImport(ctx, resource.ImportStateRequest{ID: "CUST:ID"}, &diags, "check")
		if from || diags.HasError() {
			t.Fatalf("from=%v diags=%v", from, diags)
		}
	})

	t.Run("should defer to the string form when the identity is nil or null", func(t *testing.T) {
		var diags diag.Diagnostics
		for _, ri := range []*tfsdk.ResourceIdentity{nil, emptyIdentity(ctx, s)} {
			if _, _, from := FromImport(ctx, resource.ImportStateRequest{Identity: ri}, &diags, "check"); from {
				t.Fatalf("identity %v: from=true", ri)
			}
		}
	})

	t.Run("should return both values from an identity block", func(t *testing.T) {
		var diags diag.Diagnostics
		ri := identityWith(t, ctx, s, Model{ID: types.StringValue("ID1"), CustomerID: types.StringValue("CUST")})
		id, cust, from := FromImport(ctx, resource.ImportStateRequest{Identity: ri}, &diags, "check")
		if !from || diags.HasError() || id != "ID1" || cust != "CUST" {
			t.Fatalf("got %q %q from=%v diags=%v", id, cust, from, diags)
		}
	})

	t.Run("should reject an identity block with an empty id, naming the noun", func(t *testing.T) {
		var diags diag.Diagnostics
		ri := identityWith(t, ctx, s, Model{ID: types.StringValue(""), CustomerID: types.StringNull()})
		_, _, from := FromImport(ctx, resource.ImportStateRequest{Identity: ri}, &diags, "maintenance window")
		if !from || !diags.HasError() {
			t.Fatalf("from=%v diags=%v; want an error", from, diags)
		}
		if d := diags.Errors()[0]; d.Summary() != "Invalid Import Identity" || !strings.Contains(d.Detail(), "non-empty maintenance window ID") {
			t.Errorf("diagnostic = %q / %q", d.Summary(), d.Detail())
		}
	})
}

func TestAccountHelpers(t *testing.T) {
	ctx := context.Background()
	s := identityschema.Schema{Attributes: map[string]identityschema.Attribute{
		"id": identityschema.StringAttribute{RequiredForImport: true},
	}}

	t.Run("should write the account id and ignore a nil identity", func(t *testing.T) {
		var diags diag.Diagnostics
		SetAccount(ctx, nil, types.StringValue("A"), &diags)
		ri := emptyIdentity(ctx, s)
		SetAccount(ctx, ri, types.StringValue("A"), &diags)
		var got AccountModel
		diags.Append(ri.Get(ctx, &got)...)
		if diags.HasError() || got.ID.ValueString() != "A" {
			t.Fatalf("got %+v, %v", got, diags)
		}
	})

	t.Run("should read the id from an identity block and defer otherwise", func(t *testing.T) {
		var diags diag.Diagnostics
		if _, from := AccountFromImport(ctx, resource.ImportStateRequest{ID: "A"}, &diags); from {
			t.Fatal("string import must not be treated as identity")
		}
		ri := identityWith(t, ctx, s, AccountModel{ID: types.StringValue("A")})
		id, from := AccountFromImport(ctx, resource.ImportStateRequest{Identity: ri}, &diags)
		if !from || id != "A" || diags.HasError() {
			t.Fatalf("got %q from=%v diags=%v", id, from, diags)
		}
	})
}

func TestRecordCreatedID(t *testing.T) {
	ctx := context.Background()
	rs := schema.Schema{Attributes: map[string]schema.Attribute{
		"id":          schema.StringAttribute{Computed: true},
		"customer_id": schema.StringAttribute{Optional: true},
	}}
	newResp := func() *resource.CreateResponse {
		return &resource.CreateResponse{State: tfsdk.State{Schema: rs, Raw: tftypes.NewValue(rs.Type().TerraformType(ctx), nil)}}
	}
	read := func(t *testing.T, resp *resource.CreateResponse) (id, cust types.String) {
		t.Helper()
		resp.State.GetAttribute(ctx, path.Root("id"), &id)
		resp.State.GetAttribute(ctx, path.Root("customer_id"), &cust)
		return id, cust
	}

	t.Run("should leave state alone when the create succeeded", func(t *testing.T) {
		resp := newResp()
		RecordCreatedID(ctx, resp, "ID1", types.StringValue("CUST"))
		if !resp.State.Raw.IsNull() {
			t.Fatal("state written on a successful create")
		}
	})

	t.Run("should record the id and a configured customer_id when the create errored", func(t *testing.T) {
		resp := newResp()
		resp.Diagnostics.AddError("boom", "after the POST")
		RecordCreatedID(ctx, resp, "ID1", types.StringValue("CUST"))
		id, cust := read(t, resp)
		if id.ValueString() != "ID1" || cust.ValueString() != "CUST" {
			t.Fatalf("got %v / %v", id, cust)
		}
	})

	t.Run("should not write an unset, unknown or empty customer_id", func(t *testing.T) {
		for _, c := range []types.String{types.StringNull(), types.StringUnknown(), types.StringValue("")} {
			resp := newResp()
			resp.Diagnostics.AddError("boom", "after the POST")
			RecordCreatedID(ctx, resp, "ID1", c)
			id, cust := read(t, resp)
			if id.ValueString() != "ID1" || !cust.IsNull() {
				t.Fatalf("customer_id %v: got %v / %v", c, id, cust)
			}
		}
	})
}
