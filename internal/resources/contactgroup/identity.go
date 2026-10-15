package contactgroup

import (
	"context"

	"github.com/atlanticbt/terraform-provider-nodeping/internal/resources/identity"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
)

var _ resource.ResourceWithIdentity = &ContactGroupResource{}

// ContactGroupIdentityModel is the resource identity for nodeping_contactgroup: id plus
// customer_id. See identity.Model.
type ContactGroupIdentityModel = identity.Model

// ContactGroupIdentitySchema describes the identity of a nodeping_contactgroup, which enables
// importing with an "identity" block instead of a colon-delimited string.
func ContactGroupIdentitySchema() identityschema.Schema {
	return identity.Schema("contact group")
}

// IdentitySchema implements resource.ResourceWithIdentity.
func (r *ContactGroupResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = ContactGroupIdentitySchema()
}
