// Package identity holds the resource identity and create-recovery helpers
// shared by every NodePing resource.
//
// Two identity shapes exist. Objects that live inside an account (checks,
// contacts, contact groups, schedules, maintenance windows, notification
// profiles) are located by id plus customer_id, and use Model. A subaccount is
// itself an account, so it is located by id alone, and uses AccountModel.
package identity

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Model is the identity of an object inside a NodePing account: the smallest
// stable pair of values that locates it.
//
// Both members are immutable for the lifetime of the object, which is what an
// identity requires: the id is server-assigned (or, for schedules,
// RequiresReplace) and never mutated in place, and customer_id is
// RequiresReplace on every managed schema, so moving an object between
// accounts is a destroy and recreate, never an update.
type Model struct {
	ID         types.String `tfsdk:"id"`
	CustomerID types.String `tfsdk:"customer_id"`
}

// Schema describes the identity of an account-scoped object. noun names the
// object in the attribute descriptions, for example "contact group".
//
// It enables importing with an "identity" block instead of a colon-delimited
// string:
//
//	import {
//	  to = nodeping_contactgroup.example
//	  identity = {
//	    id          = "..."
//	    customer_id = "..." # omit for the root account
//	  }
//	}
func Schema(noun string) identityschema.Schema {
	return identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				RequiredForImport: true,
				Description:       fmt.Sprintf("The NodePing ID of the %s.", noun),
			},
			"customer_id": identityschema.StringAttribute{
				OptionalForImport: true,
				Description:       fmt.Sprintf("The NodePing subaccount ID the %s belongs to. Omit for the root account.", noun),
			},
		},
	}
}

// Set writes the identity data alongside state.
//
// It must be called from Create, Read, Update and ImportState alike: an
// identity set only on create is absent from state after the first refresh.
//
// A nil identity means the running core does not support resource identity
// (anything older than OpenTofu 1.12). That is not an error, so this is a
// no-op rather than a diagnostic. Feature detection is deliberately done on
// the nil-ness of the identity and never on req.TerraformVersion, which
// OpenTofu reports as a hardcoded "1.13.0" regardless of its real version.
func Set(ctx context.Context, identity *tfsdk.ResourceIdentity, id, customerID types.String, diags *diag.Diagnostics) {
	if identity == nil {
		return
	}

	diags.Append(identity.Set(ctx, Model{
		ID:         KnownValue(id),
		CustomerID: KnownValue(customerID),
	})...)
}

// FromImport resolves the id and customer ID supplied by an "identity" import
// block.
//
// fromIdentity is false when the practitioner imported by string ID instead
// (the "CUSTOMERID:ID" and bare "ID" forms), or when the core does not support
// identity at all; the caller then parses req.ID itself. When fromIdentity is
// true and the identity names no id, an "Invalid Import Identity" error naming
// noun is added. Either way the caller must check diags.HasError() before
// using the result.
func FromImport(ctx context.Context, req resource.ImportStateRequest, diags *diag.Diagnostics, noun string) (id, customerID string, fromIdentity bool) {
	if req.ID != "" || req.Identity == nil || req.Identity.Raw.IsNull() {
		return "", "", false
	}

	var model Model
	diags.Append(req.Identity.Get(ctx, &model)...)
	if diags.HasError() {
		return "", "", false
	}

	id = model.ID.ValueString()
	if id == "" {
		diags.AddError(
			"Invalid Import Identity",
			fmt.Sprintf("The 'id' attribute of the import identity must be a non-empty %s ID.", noun),
		)
	}
	return id, model.CustomerID.ValueString(), true
}

// AccountModel is the identity of a subaccount, which is an account rather
// than an object inside one, so its account ID alone locates it.
type AccountModel struct {
	ID types.String `tfsdk:"id"`
}

// SetAccount writes an AccountModel identity. See Set for when it must be
// called and why a nil identity is a no-op.
func SetAccount(ctx context.Context, identity *tfsdk.ResourceIdentity, id types.String, diags *diag.Diagnostics) {
	if identity == nil {
		return
	}

	diags.Append(identity.Set(ctx, AccountModel{ID: KnownValue(id)})...)
}

// AccountFromImport reads the id an "identity" import block supplied for an
// account-scoped resource. fromIdentity is false for a string-ID import or a
// core without identity support. The id is returned as given; validating it is
// the caller's job.
func AccountFromImport(ctx context.Context, req resource.ImportStateRequest, diags *diag.Diagnostics) (id string, fromIdentity bool) {
	if req.ID != "" || req.Identity == nil || req.Identity.Raw.IsNull() {
		return "", false
	}

	var model AccountModel
	diags.Append(req.Identity.Get(ctx, &model)...)
	if diags.HasError() {
		return "", false
	}

	return model.ID.ValueString(), true
}

// KnownValue collapses an unknown to null. Unknown values are legal in a plan
// but must never be written into identity data.
func KnownValue(value types.String) types.String {
	if value.IsUnknown() {
		return types.StringNull()
	}
	return value
}
