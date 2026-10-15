package contact

import (
	"context"

	"github.com/atlanticbt/terraform-provider-nodeping/internal/resources/identity"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
)

var _ resource.ResourceWithIdentity = &ContactResource{}

// ContactIdentityModel is the resource identity for nodeping_contact: id plus
// customer_id. See identity.Model.
type ContactIdentityModel = identity.Model

// ContactIdentitySchema describes the identity of a nodeping_contact, which enables
// importing with an "identity" block instead of a colon-delimited string.
func ContactIdentitySchema() identityschema.Schema {
	return identity.Schema("contact")
}

// IdentitySchema implements resource.ResourceWithIdentity.
func (r *ContactResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = ContactIdentitySchema()
}
