package subaccount

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ValidLocations is the set of NodePing region codes accepted for a
// subaccount's default probe region.
//
// NodePing's POST /accounts documentation lists only nam, eur, eao and wlw and
// omits lam. Both the PUT /accounts documentation and the `runlocations` field
// on checks accept lam, so the omission is a documentation bug rather than a
// server-side restriction, and lam is accepted here.
var ValidLocations = []string{"nam", "lam", "eur", "eao", "wlw"}

// ValidStatuses are the two values NodePing's account `status` field accepts.
// Status is not supported on a primary account, only on subaccounts.
var ValidStatuses = []string{"Active", "Suspend"}

// emailPattern is a deliberately loose shape check. It rejects obvious
// mistakes (missing @, whitespace, no dot in the domain) before the value
// reaches the API, without trying to out-guess RFC 5322.
var emailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@.]+(\.[^\s@.]+)+$`)

// timezonePattern matches the GMT offsets NodePing accepts: a signed integer or
// half/quarter-hour decimal, for example "-7", "0", "+5.5", "5.75".
var timezonePattern = regexp.MustCompile(`^[+-]?\d{1,2}(\.\d{1,2})?$`)

type SubaccountResourceModel struct {
	ID                    types.String `tfsdk:"id"`
	Name                  types.String `tfsdk:"name"`
	ContactName           types.String `tfsdk:"contactname"`
	Email                 types.String `tfsdk:"email"`
	Timezone              types.String `tfsdk:"timezone"`
	Location              types.String `tfsdk:"location"`
	EmailMe               types.String `tfsdk:"emailme"`
	AutodiagNotifications types.String `tfsdk:"autodiagnotifications"`
	Status                types.String `tfsdk:"status"`
	Type                  types.String `tfsdk:"type"`
	CreationDate          types.Int64  `tfsdk:"creation_date"`
	NextBillingDate       types.String `tfsdk:"next_billing_date"`
	DefaultLocations      types.List   `tfsdk:"default_locations"`
}

// unchangeableAfterCreate refuses a change to a create-only attribute on an
// existing subaccount.
//
// NodePing accepts contactname and email only when a subaccount is created: its
// update endpoint cannot change them and no read returns them. The only way
// Terraform could apply a change is to replace the subaccount, and replacing
// one deletes every check, contact and result in it, so the change is an error
// instead. Prior state holding no value (after an import, which cannot recover
// either field) is not a change: the configured value is recorded with no API
// call.
func unchangeableAfterCreate() planmodifier.String {
	return unchangeableAfterCreateModifier{}
}

type unchangeableAfterCreateModifier struct{}

func (unchangeableAfterCreateModifier) Description(context.Context) string {
	return "Refuses a change after the subaccount exists; a null prior value, as left by an import, is not a change."
}

func (m unchangeableAfterCreateModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (unchangeableAfterCreateModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.State.Raw.IsNull() || req.StateValue.IsNull() || req.StateValue.ValueString() == "" {
		return
	}
	if req.PlanValue.IsNull() || req.PlanValue.IsUnknown() || req.PlanValue.Equal(req.StateValue) {
		return
	}
	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Cannot Change After Creation",
		fmt.Sprintf("%s cannot be changed on an existing subaccount: NodePing only accepts it when the subaccount is created. "+
			"Change it in the NodePing UI, then set the configuration to the new value. "+
			"Replacing the subaccount to apply it would delete every check and contact in it.", req.Path),
	)
}

func SubaccountSchema() schema.Schema {
	return schema.Schema{
		Description: "Manages a NodePing subaccount.",
		MarkdownDescription: `
Manages a NodePing subaccount.

NodePing's ` + "`/accounts`" + ` endpoint can only create subaccounts. A top-level account
cannot be created, imported or destroyed through the API, which is why this resource is
named ` + "`nodeping_subaccount`" + ` and not ` + "`nodeping_account`" + `. To read the
primary account, use the ` + "`nodeping_account`" + ` data source.

## Destroying a subaccount destroys everything inside it

Deleting a subaccount removes every check, contact and stored result that belongs to it.
There is no undo and no export. Guard production subaccounts:

` + "```hcl" + `
resource "nodeping_subaccount" "client" {
  name        = "Example Client"
  contactname = "Example Operations"
  email       = "ops@example.com"
  timezone    = "-7"
  location    = "nam"

  lifecycle {
    prevent_destroy = true
  }
}
` + "```" + `

Deletion is queued rather than immediate. NodePing acknowledges the request and removes
the account shortly afterwards, so the account may still be visible in the
` + "`nodeping_accounts`" + ` data source for a short time after a successful destroy.

## Create-only attributes

` + "`contactname`" + ` and ` + "`email`" + ` are accepted only when the subaccount is
created. NodePing's update endpoint does not accept them and no read returns them, so they
cannot be refreshed or changed in place. Changing either on an existing subaccount is an
error at plan time: change it in the NodePing UI, then update the configuration to match.
The provider never replaces a subaccount to apply such a change, because replacing one
deletes everything in it.

## Import

` + "```shell" + `
terraform import nodeping_subaccount.client 201205102227VZ6XU
` + "```" + `

Import cannot recover ` + "`contactname`" + `, ` + "`email`" + `,
or ` + "`autodiagnotifications`" + ` — the API never returns them. ` + "`contactname`" + ` and
` + "`email`" + ` may stay unset on an imported subaccount; setting them later records the
configured value with no API call. ` + "`autodiagnotifications`" + ` is set to its default,
` + "`emailme`" + ` is read from the API, and ` + "`location`" + ` is taken from
` + "`default_locations`" + ` when that holds a single region.
`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The NodePing customer ID of the subaccount. This resource is the account, so there is no separate customer_id attribute.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The subaccount name. Returned by the API as customer_name.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 255),
				},
			},
			"contactname": schema.StringAttribute{
				Description: "Name of the subaccount's primary contact. Sent only when the subaccount is created; " +
					"the API neither returns nor updates it, so changing it on an existing subaccount is refused at plan time; change it in the NodePing UI. " +
					"Required to create a subaccount; may be left unset on one that was imported.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 255),
				},
				PlanModifiers: []planmodifier.String{
					unchangeableAfterCreate(),
				},
			},
			"email": schema.StringAttribute{
				Description: "Email address of the subaccount's primary contact. Sent only when the subaccount is created; " +
					"the API neither returns nor updates it, so changing it on an existing subaccount is refused at plan time; change it in the NodePing UI. " +
					"Required to create a subaccount; may be left unset on one that was imported.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(3, 320),
					stringvalidator.RegexMatches(emailPattern, "must be an email address"),
				},
				PlanModifiers: []planmodifier.String{
					unchangeableAfterCreate(),
				},
			},
			"timezone": schema.StringAttribute{
				Description: "GMT offset for the subaccount, expressed as a signed number in a string, for example \"-7\" or \"5.5\".",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(timezonePattern, "must be a GMT offset such as \"-7\", \"0\" or \"5.5\""),
				},
			},
			"location": schema.StringAttribute{
				Description: "Default probe region for the subaccount. One of: nam, lam, eur, eao, wlw. " +
					"Required to create a subaccount. NodePing does not return it, so an imported subaccount takes it " +
					"from default_locations when that holds a single region.",
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					stringvalidator.OneOf(ValidLocations...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"emailme": schema.StringAttribute{
				Description: "Whether NodePing emails the parent account about this subaccount. " +
					"'yes' or 'no'. NodePing returns it as a boolean, which is read back as 'yes' or 'no'.",
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("no"),
				Validators: []validator.String{
					stringvalidator.OneOf("yes", "no"),
				},
			},
			"autodiagnotifications": schema.StringAttribute{
				Description: "Whether automatic diagnostics are attached to notifications. " +
					"'true' or 'false'. The API never returns this value, so it is tracked from configuration only.",
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("false"),
				Validators: []validator.String{
					stringvalidator.OneOf("true", "false"),
				},
			},
			"status": schema.StringAttribute{
				Description: "Subaccount status. 'Active' or 'Suspend'. A suspended subaccount stops running its checks.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.String{
					stringvalidator.OneOf(ValidStatuses...),
				},
			},
			"type": schema.StringAttribute{
				Description: "Account type reported by NodePing, for example 'subcustomer'.",
				Computed:    true,
			},
			"creation_date": schema.Int64Attribute{
				Description: "Creation timestamp reported by NodePing.",
				Computed:    true,
			},
			"next_billing_date": schema.StringAttribute{
				Description: "Next billing date reported by NodePing.",
				Computed:    true,
			},
			"default_locations": schema.ListAttribute{
				Description: "Probe regions NodePing has recorded as the subaccount's defaults. " +
					"Derived from `location`; NodePing may normalise or expand the value.",
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}
