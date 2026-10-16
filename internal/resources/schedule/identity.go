package schedule

import (
	"context"

	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/resources/identity"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
)

var _ resource.ResourceWithIdentity = &ScheduleResource{}

// ScheduleIdentityModel is the resource identity for nodeping_schedule: id plus
// customer_id. See identity.Model.
type ScheduleIdentityModel = identity.Model

// ScheduleIdentitySchema describes the identity of a nodeping_schedule, which enables
// importing with an "identity" block instead of a colon-delimited string.
func ScheduleIdentitySchema() identityschema.Schema {
	return identity.Schema("schedule")
}

// IdentitySchema implements resource.ResourceWithIdentity.
func (r *ScheduleResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = ScheduleIdentitySchema()
}
