package contact

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.ResourceWithIdentity = &ContactResource{}

// ContactIdentityModel is the resource identity for nodeping_contact: the
// smallest stable pair of values that locates one contact in NodePing.
//
// Both members are immutable for the lifetime of the object, which is what an
// identity requires. See ContactIdentitySchema for the per-attribute
// reasoning.
type ContactIdentityModel struct {
	ID         types.String `tfsdk:"id"`
	CustomerID types.String `tfsdk:"customer_id"`
}

// ContactIdentitySchema describes the identity of a nodeping_contact.
//
// It enables importing with an "identity" block instead of a colon-delimited
// string:
//
//	import {
//	  to = nodeping_contact.example
//	  identity = {
//	    id          = "..."
//	    customer_id = "..." # omit for the root account
//	  }
//	}
func ContactIdentitySchema() identityschema.Schema {
	return identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			// Server-assigned (or, for schedules, RequiresReplace) and never
			// mutated in place, so it is stable for the object's lifetime.
			"id": identityschema.StringAttribute{
				RequiredForImport: true,
				Description:       "The NodePing ID of the contact.",
			},
			// RequiresReplace on the managed schema: moving an object between
			// accounts is a destroy and recreate, never an update.
			"customer_id": identityschema.StringAttribute{
				OptionalForImport: true,
				Description:       "The NodePing subaccount ID the contact belongs to. Omit for the root account.",
			},
		},
	}
}

// IdentitySchema implements resource.ResourceWithIdentity.
func (r *ContactResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = ContactIdentitySchema()
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
func setIdentity(ctx context.Context, identity *tfsdk.ResourceIdentity, id, customerID types.String, diags *diag.Diagnostics) {
	if identity == nil {
		return
	}

	diags.Append(identity.Set(ctx, ContactIdentityModel{
		ID:         knownIdentityValue(id),
		CustomerID: knownIdentityValue(customerID),
	})...)
}

// knownIdentityValue collapses an unknown to null. Unknown values are legal in
// a plan but must never be written into identity data.
func knownIdentityValue(value types.String) types.String {
	if value.IsUnknown() {
		return types.StringNull()
	}
	return value
}

// identityImportValues reads the id and customer_id an "identity" import block
// supplied.
//
// ok is false when the practitioner imported by string ID instead (the
// "CUSTOMERID:ID" and bare "ID" forms), or when the core does not support
// identity at all; the caller then falls back to parsing req.ID.
func identityImportValues(ctx context.Context, req resource.ImportStateRequest, diags *diag.Diagnostics) (id string, customerID string, ok bool) {
	if req.ID != "" || req.Identity == nil || req.Identity.Raw.IsNull() {
		return "", "", false
	}

	var identity ContactIdentityModel
	diags.Append(req.Identity.Get(ctx, &identity)...)
	if diags.HasError() {
		return "", "", false
	}

	return identity.ID.ValueString(), identity.CustomerID.ValueString(), true
}
