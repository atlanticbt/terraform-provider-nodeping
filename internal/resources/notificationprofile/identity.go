package notificationprofile

import (
	"context"

	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/resources/identity"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
)

var _ resource.ResourceWithIdentity = &NotificationProfileResource{}

// NotificationProfileIdentityModel is the resource identity for nodeping_notificationprofile: id plus
// customer_id. See identity.Model.
type NotificationProfileIdentityModel = identity.Model

// NotificationProfileIdentitySchema describes the identity of a nodeping_notificationprofile, which enables
// importing with an "identity" block instead of a colon-delimited string.
func NotificationProfileIdentitySchema() identityschema.Schema {
	return identity.Schema("notification profile")
}

// IdentitySchema implements resource.ResourceWithIdentity.
func (r *NotificationProfileResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = NotificationProfileIdentitySchema()
}
