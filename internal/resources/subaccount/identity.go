package subaccount

import (
	"context"

	"github.com/atlanticbt/terraform-provider-nodeping/internal/resources/identity"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
)

var _ resource.ResourceWithIdentity = &SubaccountResource{}

// SubaccountIdentityModel is the resource identity for nodeping_subaccount.
//
// Unlike every other resource in this provider it carries no customer_id: a
// subaccount *is* the account, so the ID is already the account scope and a
// second scoping attribute would be meaningless.
type SubaccountIdentityModel = identity.AccountModel

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
