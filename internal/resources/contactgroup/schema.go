package contactgroup

import (
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type ContactGroupResourceModel struct {
	ID         types.String `tfsdk:"id"`
	CustomerID types.String `tfsdk:"customer_id"`
	Name       types.String `tfsdk:"name"`
	Members    types.Set    `tfsdk:"members"`
}

// membersDescription is repeated in the plain and markdown descriptions because
// getting this wrong is the most common failure mode for this resource.
const membersDescription = "Contact **address** IDs that belong to this group. " +
	"These are NOT contact IDs. A NodePing contact owns one or more addresses, " +
	"each with its own 8-character ID (for example `K5SP9CQP`), and it is the " +
	"address that joins a group. Reference them as " +
	"`nodeping_contact.<name>.address[<index>].id`."

func ContactGroupSchema() schema.Schema {
	return schema.Schema{
		Description: "Manages a NodePing contact group.",
		MarkdownDescription: `
Manages a NodePing contact group.

A contact group bundles notification targets so a check can notify several
people at once by referencing a single group ID.

## Members are address IDs, not contact IDs

` + "`members`" + ` holds contact **address** IDs. A NodePing contact is a person or
integration that owns one or more addresses (an email, a phone number, a
webhook URL), and each of those addresses has its own 8-character ID. Group
membership is per address, so one contact can have its email in a group while
its SMS address stays out.

Passing a contact ID (` + "`<accountid>-BKPGH`" + `) instead of an address ID
(` + "`K5SP9CQP`" + `) is accepted by the API and produces a group that never
notifies anyone.

## Example Usage

` + "```hcl" + `
resource "nodeping_contact" "ops" {
  name     = "Operations"
  custrole = "notify"

  address {
    type    = "email"
    address = "ops@example.com"
  }

  address {
    type    = "sms"
    address = "+15551230000"
  }
}

resource "nodeping_contact" "oncall" {
  name     = "On Call"
  custrole = "notify"

  address {
    type    = "email"
    address = "oncall@example.com"
  }
}

resource "nodeping_contactgroup" "alerts" {
  name = "Production Alerts"

  members = [
    # the email address of the ops contact -- not nodeping_contact.ops.id
    nodeping_contact.ops.address[0].id,
    nodeping_contact.oncall.address[0].id,
  ]
}
` + "```" + `

To include every address a contact owns:

` + "```hcl" + `
resource "nodeping_contactgroup" "everything" {
  name    = "All Ops Channels"
  members = [for a in nodeping_contact.ops.address : a.id]
}
` + "```" + `

## Import

Contact groups are imported by group ID:

` + "```shell" + `
terraform import nodeping_contactgroup.alerts 201205050153W2Q4C-G-ISYB7
` + "```" + `

For SubAccount groups, use the format ` + "`customer_id:group_id`" + `:

` + "```shell" + `
terraform import nodeping_contactgroup.alerts 201205050153W2Q4C:201205050153W2Q4C-G-ISYB7
` + "```" + `

The imported ` + "`customer_id`" + ` is written to state and is used for every subsequent
API call for this resource. Set ` + "`customer_id`" + ` in your configuration to match,
otherwise the next plan will show a replacement. Changing ` + "`customer_id`" + ` forces a
new contact group.
`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the contact group, in the form `<accountid>-G-XXXXX`.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"customer_id": schema.StringAttribute{
				Description: "The customer ID (SubAccount ID) that owns this contact group. " +
					"Set this to manage a group that belongs to a SubAccount. " +
					"Changing it forces a new contact group to be created.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the contact group. Used as a label.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"members": schema.SetAttribute{
				Description: membersDescription,
				MarkdownDescription: membersDescription +
					"\n\nThis is a set: the order you write the IDs in is not significant and " +
					"NodePing reordering them server-side does not produce a diff.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}
