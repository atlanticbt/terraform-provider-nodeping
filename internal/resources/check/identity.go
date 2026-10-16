package check

import (
	"context"

	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/resources/identity"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
)

var _ resource.ResourceWithIdentity = &CheckResource{}

// CheckIdentityModel is the resource identity for nodeping_check: id plus
// customer_id. See identity.Model.
type CheckIdentityModel = identity.Model

// CheckIdentitySchema describes the identity of a nodeping_check, which enables
// importing with an "identity" block instead of a colon-delimited string.
func CheckIdentitySchema() identityschema.Schema {
	return identity.Schema("check")
}

// IdentitySchema implements resource.ResourceWithIdentity.
func (r *CheckResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = CheckIdentitySchema()
}
