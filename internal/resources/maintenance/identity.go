package maintenance

import (
	"context"

	"github.com/atlanticbt/terraform-provider-nodeping/internal/resources/identity"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
)

var _ resource.ResourceWithIdentity = &MaintenanceResource{}

// MaintenanceIdentityModel is the resource identity for nodeping_maintenance: id plus
// customer_id. See identity.Model.
type MaintenanceIdentityModel = identity.Model

// MaintenanceIdentitySchema describes the identity of a nodeping_maintenance, which enables
// importing with an "identity" block instead of a colon-delimited string.
func MaintenanceIdentitySchema() identityschema.Schema {
	return identity.Schema("maintenance schedule")
}

// IdentitySchema implements resource.ResourceWithIdentity.
func (r *MaintenanceResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = MaintenanceIdentitySchema()
}
