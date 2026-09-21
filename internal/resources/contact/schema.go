package contact

import (
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type ContactResourceModel struct {
	ID         types.String   `tfsdk:"id"`
	CustomerID types.String   `tfsdk:"customer_id"`
	Name       types.String   `tfsdk:"name"`
	CustRole   types.String   `tfsdk:"custrole"`
	Addresses  []AddressModel `tfsdk:"address"`
}

type AddressModel struct {
	ID            types.String `tfsdk:"id"`
	Type          types.String `tfsdk:"type"`
	Address       types.String `tfsdk:"address"`
	SuppressUp    types.Bool   `tfsdk:"suppress_up"`
	SuppressDown  types.Bool   `tfsdk:"suppress_down"`
	SuppressFirst types.Bool   `tfsdk:"suppress_first"`
	SuppressDiag  types.Bool   `tfsdk:"suppress_diag"`
	SuppressAll   types.Bool   `tfsdk:"suppress_all"`
	Mute          types.Bool   `tfsdk:"mute"`
	Action        types.String `tfsdk:"action"`
	Headers       types.Map    `tfsdk:"headers"`
	QueryStrings  types.Map    `tfsdk:"querystrings"`
	Data          JSONString   `tfsdk:"data"`
	Priority      types.Int64  `tfsdk:"priority"`
}

func ContactSchema() schema.Schema {
	return schema.Schema{
		Description: "Manages a NodePing contact.",
		MarkdownDescription: `
Manages a NodePing contact.

Contacts are used to receive notifications when checks fail or recover. Each contact can have multiple addresses (email, SMS, webhook, etc.).

## Example Usage

` + "```hcl" + `
resource "nodeping_contact" "example" {
  name     = "John Doe"
  custrole = "notify"

  address {
    type    = "email"
    address = "john@example.com"
  }

  address {
    type    = "webhook"
    address = "https://hooks.example.com/notify"
    action  = "post"
    headers = {
      "Content-Type" = "application/json"
    }
  }
}
` + "```" + `

### Webhook with JSON Body (MS Teams, Slack, etc.)

When using webhooks with JSON bodies, write the body with heredoc syntax. If the
value is valid JSON it is compared *semantically*, so differences in whitespace,
indentation and key ordering between your configuration and what the API returns
never produce a diff and never cause an inconsistent-result error:

` + "```hcl" + `
resource "nodeping_contact" "msteams" {
  name     = "MS Teams Notifications"
  custrole = "notify"

  address {
    type    = "webhook"
    address = "https://outlook.office.com/webhook/..."
    action  = "post"
    headers = {
      "content-type" = "application/json"
    }
    querystrings = {
      "key" = "1"
    }
    # Use chomp() with heredoc to match exact API format
    data = chomp(<<-EOT
{"@context": "https://schema.org/extensions","@type": "MessageCard","title": "Host {label} : {type} is {event}","text": "Host {label} : {type} is {event} {if downtime}after {downtime} {if downminutes}minutes{else}minute{/if} of downtime {/if}as of {checktime}: 
 {message}"}
EOT
)
  }
}
` + "```" + `

**Note:** The ` + "`data`" + ` field stores the exact string as provided in your configuration.
Values that are valid JSON are compared semantically against the API, so formatting
differences are ignored. Values that are not JSON are compared literally.


## Import

Contacts can be imported using the contact ID:

` + "```shell" + `
terraform import nodeping_contact.example 201205050153W2Q4C-BKPGH
` + "```" + `

For SubAccount contacts, use the format ` + "`customer_id:contact_id`" + `:

` + "```shell" + `
terraform import nodeping_contact.example 201205050153W2Q4C:201205050153W2Q4C-BKPGH
` + "```" + `

The imported ` + "`customer_id`" + ` is written to state and is used for every subsequent
API call for this resource. Set ` + "`customer_id`" + ` in your configuration to match, otherwise
the next plan will show a replacement. Changing ` + "`customer_id`" + ` forces a new contact.
`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the contact.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"customer_id": schema.StringAttribute{
				Description: "The customer ID (SubAccount ID) that owns this contact. " +
					"Set this to manage a contact that belongs to a SubAccount. " +
					"Changing it forces a new contact to be created.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the contact. Used as a label.",
				Optional:    true,
			},
			"custrole": schema.StringAttribute{
				Description: "The permission role for this contact. Valid values: 'edit', 'view', 'notify'. Defaults to 'notify'.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("notify"),
				Validators: []validator.String{
					stringvalidator.OneOf("edit", "view", "notify"),
				},
			},
		},
		Blocks: map[string]schema.Block{
			"address": schema.ListNestedBlock{
				Description: "Contact addresses for receiving notifications.",
				PlanModifiers: []planmodifier.List{
					AddressIDPlanModifier(),
				},
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "The unique identifier of the address, assigned by NodePing.",
							Computed:    true,
						},
						"type": schema.StringAttribute{
							Description: "The type of address. Commonly used values: 'email', 'sms', 'voice', " +
								"'webhook', 'slack', 'hipchat', 'pushover', 'pushbullet', 'rocketchat', 'pagerduty'. " +
								"NodePing may support additional notification types; the value is validated by the API.",
							Required: true,
						},
						// Deliberately not Sensitive. Config generation writes
						// `null # sensitive` for a sensitive attribute, which
						// leaves a Required attribute unset and makes the
						// contact impossible to import. See A-015.
						"address": schema.StringAttribute{
							Description: "The address value (email, phone number, webhook URL, etc.).",
							Required:    true,
						},
						"suppress_up": schema.BoolAttribute{
							Description: "Suppress 'up' notifications to this address.",
							Optional:    true,
							Computed:    true,
							Default:     booldefault.StaticBool(false),
						},
						"suppress_down": schema.BoolAttribute{
							Description: "Suppress 'down' notifications to this address.",
							Optional:    true,
							Computed:    true,
							Default:     booldefault.StaticBool(false),
						},
						"suppress_first": schema.BoolAttribute{
							Description: "Suppress 'first result' notifications to this address.",
							Optional:    true,
							Computed:    true,
							Default:     booldefault.StaticBool(false),
						},
						"suppress_diag": schema.BoolAttribute{
							Description: "Suppress diagnostic notifications to this address.",
							Optional:    true,
							Computed:    true,
							Default:     booldefault.StaticBool(false),
						},
						"suppress_all": schema.BoolAttribute{
							Description: "Suppress all notifications to this address.",
							Optional:    true,
							Computed:    true,
							Default:     booldefault.StaticBool(false),
						},
						"mute": schema.BoolAttribute{
							Description: "Mute all notifications to this address.",
							Optional:    true,
							Computed:    true,
							Default:     booldefault.StaticBool(false),
						},
						"action": schema.StringAttribute{
							Description: "HTTP method for webhook addresses. Valid values: 'get', 'put', 'post', 'head', 'delete'.",
							Optional:    true,
							Validators: []validator.String{
								stringvalidator.OneOf("get", "put", "post", "head", "delete"),
							},
						},
						// Not marked sensitive, for the reason recorded in A-015:
						// generated configuration writes null for a sensitive
						// attribute, so importing a webhook contact would plan to
						// delete whatever these hold. These can carry an auth
						// token, so plan output shows it.
						"headers": schema.MapAttribute{
							Description: "HTTP headers for webhook addresses. Often carries authentication tokens, " +
								"which appear in plan output and in generated configuration.",
							Optional:    true,
							ElementType: types.StringType,
						},
						"querystrings": schema.MapAttribute{
							Description: "Query string parameters for webhook addresses. Often carries authentication " +
								"tokens, which appear in plan output and in generated configuration.",
							Optional:    true,
							ElementType: types.StringType,
						},
						"data": schema.StringAttribute{
							Description: "Request body for webhook addresses (POST/PUT). " +
								"If the value is valid JSON it is compared semantically, so whitespace and " +
								"key ordering differences between the configuration and the API do not produce a diff.",
							Optional:   true,
							CustomType: JSONStringType{},
						},
						"priority": schema.Int64Attribute{
							Description: "Priority for Pushover addresses. Valid values: -2 to 2.",
							Optional:    true,
							Validators: []validator.Int64{
								int64validator.Between(-2, 2),
							},
						},
					},
				},
			},
		},
	}
}
