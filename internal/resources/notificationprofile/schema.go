package notificationprofile

import (
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// maxNameLength bounds the profile name so a runaway value never reaches the
// API. NodePing does not document a limit; this is a defensive ceiling.
const maxNameLength = 255

// maxContactIDLength bounds a contact or contact group ID. NodePing IDs are of
// the form "<accountid>-<suffix>" and are well under this.
const maxContactIDLength = 128

// NotificationProfileResourceModel is the state model for
// nodeping_notificationprofile.
type NotificationProfileResourceModel struct {
	ID            types.String        `tfsdk:"id"`
	CustomerID    types.String        `tfsdk:"customer_id"`
	Name          types.String        `tfsdk:"name"`
	Notifications []NotificationModel `tfsdk:"notifications"`
}

// NotificationModel mirrors the notification block used by nodeping_check so
// the two resources are configured the same way.
type NotificationModel struct {
	ContactID types.String `tfsdk:"contact_id"`
	Delay     types.Int64  `tfsdk:"delay"`
	Schedule  types.String `tfsdk:"schedule"`
}

func NotificationProfileSchema() schema.Schema {
	return schema.Schema{
		Description: "Manages a NodePing notification profile.",
		MarkdownDescription: `
Manages a NodePing notification profile.

A notification profile is a named, reusable list of notification targets. Each
entry points at a contact or a contact group and carries its own delay and
schedule.

## Example Usage

` + "```hcl" + `
resource "nodeping_contact" "oncall" {
  name     = "On-Call Engineer"
  custrole = "notify"

  address {
    type    = "email"
    address = "oncall@example.com"
  }
}

resource "nodeping_notificationprofile" "escalation" {
  name = "Production Escalation"

  # A contact: paged immediately.
  notifications {
    contact_id = nodeping_contact.oncall.address[0].id
    delay      = 0
    schedule   = "All"
  }

  # A contact group: paged 15 minutes later if the check is still down.
  notifications {
    contact_id = "201205050153W2Q4C-G-3QJWG"
    delay      = 15
    schedule   = "Days"
  }
}
` + "```" + `

## Update Semantics

NodePing replaces the entire notifications array on every update; it does not
merge. The provider always sends the complete list, so removing a ` + "`notifications`" + `
block removes it from the profile.
`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the notification profile, in the form `<accountid>-P-3QJWG`.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"customer_id": schema.StringAttribute{
				Description: "The subaccount (customer) ID that owns this notification profile. Defaults to the account the provider is configured for. Changing this forces a new profile.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtMost(maxContactIDLength),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the notification profile.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, maxNameLength),
				},
			},
		},
		Blocks: map[string]schema.Block{
			"notifications": schema.ListNestedBlock{
				Description: "A notification target. Repeat the block for each contact or contact group the profile notifies.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"contact_id": schema.StringAttribute{
							Description: "Contact or contact group ID to notify.",
							Required:    true,
							Validators: []validator.String{
								stringvalidator.LengthBetween(1, maxContactIDLength),
							},
						},
						"delay": schema.Int64Attribute{
							Description: "Delay in minutes before this target is notified.",
							Optional:    true,
							Computed:    true,
							Default:     int64default.StaticInt64(0),
							Validators: []validator.Int64{
								int64validator.AtLeast(0),
							},
						},
						"schedule": schema.StringAttribute{
							Description: "Notification schedule name. Defaults to the account's `All` schedule.",
							Optional:    true,
							Computed:    true,
							Validators: []validator.String{
								stringvalidator.LengthAtMost(maxNameLength),
							},
						},
					},
				},
			},
		},
	}
}
