package subaccount

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.ResourceWithIdentity = &SubaccountResource{}

// SubaccountIdentityModel is the resource identity for nodeping_subaccount.
//
// Unlike every other resource in this provider it carries no customer_id: a
// subaccount *is* the account, so the ID is already the account scope and a
// second scoping attribute would be meaningless.
type SubaccountIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// SubaccountIdentitySchema describes the identity of a nodeping_subaccount.
//
// It enables importing with an "identity" block instead of a bare string ID:
//
//	import {
//	  to = nodeping_subaccount.example
//	  identity = {
//	    id = "201203232048C76FH"
//	  }
//	}
func SubaccountIdentitySchema() identityschema.Schema {
	return identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			// Server-assigned at creation and never mutated, so it is stable
			// for the lifetime of the account.
			"id": identityschema.StringAttribute{
				RequiredForImport: true,
				Description:       "The NodePing account ID of the subaccount.",
			},
		},
	}
}

// IdentitySchema implements resource.ResourceWithIdentity.
func (r *SubaccountResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = SubaccountIdentitySchema()
}

// setIdentity writes the identity data alongside state.
//
// It must be called from Create, Read and ImportState alike: an identity set
// only on create is absent from state after the first refresh.
//
// A nil identity means the running core does not support resource identity
// (anything older than OpenTofu 1.12). That is not an error, so this is a
// no-op rather than a diagnostic. Feature detection is deliberately done on
// the nil-ness of the identity and never on req.TerraformVersion, which
// OpenTofu reports as a hardcoded "1.13.0" regardless of its real version.
func setIdentity(ctx context.Context, identity *tfsdk.ResourceIdentity, id types.String, diags *diag.Diagnostics) {
	if identity == nil {
		return
	}

	diags.Append(identity.Set(ctx, SubaccountIdentityModel{ID: knownIdentityValue(id)})...)
}

// knownIdentityValue collapses an unknown to null. Unknown values are legal in
// a plan but must never be written into identity data.
func knownIdentityValue(value types.String) types.String {
	if value.IsUnknown() {
		return types.StringNull()
	}
	return value
}

// identityImportValues reads the id an "identity" import block supplied.
//
// ok is false when the practitioner imported by string ID instead, or when the
// core does not support identity at all; the caller then falls back to req.ID.
func identityImportValues(ctx context.Context, req resource.ImportStateRequest, diags *diag.Diagnostics) (id string, ok bool) {
	if req.ID != "" || req.Identity == nil || req.Identity.Raw.IsNull() {
		return "", false
	}

	var identity SubaccountIdentityModel
	diags.Append(req.Identity.Get(ctx, &identity)...)
	if diags.HasError() {
		return "", false
	}

	return identity.ID.ValueString(), true
}
