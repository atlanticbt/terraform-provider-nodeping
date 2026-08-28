package maintenance

import (
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// MaxChecklistEntries bounds how many checks one window may disable.
const MaxChecklistEntries = 5000

// MaxDurationMinutes caps the window at roughly one year. Anything longer is
// almost certainly a units mistake that would disable monitoring indefinitely.
const MaxDurationMinutes = 527040

type MaintenanceResourceModel struct {
	ID         types.String `tfsdk:"id"`
	CustomerID types.String `tfsdk:"customer_id"`
	Name       types.String `tfsdk:"name"`
	Enabled    types.Bool   `tfsdk:"enabled"`
	Cron       types.String `tfsdk:"cron"`
	Duration   types.Int64  `tfsdk:"duration"`
	Checklist  types.Set    `tfsdk:"checklist"`
	Processing types.Bool   `tfsdk:"processing"`
	NextRun    types.Int64  `tfsdk:"nextrun"`
}

func MaintenanceSchema() schema.Schema {
	return schema.Schema{
		Description: "Manages a scheduled NodePing maintenance window.",
		MarkdownDescription: `
Manages a **scheduled** NodePing maintenance window.

A maintenance window disables the checks in ` + "`checklist`" + ` on the schedule given by
` + "`cron`" + `, for ` + "`duration`" + ` minutes, then re-enables them.

## Ad-hoc maintenance is not supported

NodePing also accepts ad-hoc maintenance, created by posting the literal id
` + "`ad-hoc`" + `. NodePing **deletes the record as soon as the window finishes**.

That makes it impossible to manage declaratively. A resource whose object
disappears on its own looks like drift on the next refresh, so OpenTofu would
plan to recreate it, which opens another maintenance window, which is deleted
again — a loop that repeatedly disables the account's monitoring without anyone
asking it to. This resource therefore refuses to create ad-hoc windows and
requires ` + "`cron`" + `. Start an ad-hoc window from the NodePing UI or API
directly; it is an imperative operation and does not belong in state.

## Example Usage

` + "```hcl" + `
resource "nodeping_maintenance" "weekly_patching" {
  name     = "Weekly patching window"
  cron     = "0 3 * * 0"
  duration = 60
  enabled  = true

  checklist = [
    nodeping_check.web.id,
    nodeping_check.api.id,
  ]
}
` + "```" + `

## Import

Maintenance windows are imported by ID:

` + "```shell" + `
terraform import nodeping_maintenance.weekly_patching 9VKOB
` + "```" + `

For SubAccount windows, use ` + "`customer_id:maintenance_id`" + `:

` + "```shell" + `
terraform import nodeping_maintenance.weekly_patching 201205050153W2Q4C:9VKOB
` + "```" + `

The imported ` + "`customer_id`" + ` is written to state and used for every subsequent API
call for this resource. Set ` + "`customer_id`" + ` in your configuration to match, otherwise
the next plan shows a replacement. Changing ` + "`customer_id`" + ` forces a new window.
`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the maintenance window, assigned by NodePing.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"customer_id": schema.StringAttribute{
				Description: "The customer ID (SubAccount ID) that owns this maintenance window. " +
					"Set this to manage a window that belongs to a SubAccount. " +
					"Changing it forces a new window to be created.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Friendly name for the maintenance window.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 255),
				},
			},
			"enabled": schema.BoolAttribute{
				Description: "Whether the schedule is active. A disabled window never runs. Defaults to true.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"cron": schema.StringAttribute{
				Description: "Standard 5-field cron expression (minute hour day-of-month month day-of-week) " +
					"giving the start time of the window, in the account's timezone. Required: this resource " +
					"manages scheduled maintenance only.",
				Required: true,
				Validators: []validator.String{
					CronValidator(),
				},
			},
			"duration": schema.Int64Attribute{
				Description: "How long the checks stay disabled, in minutes. Must be at least 1.",
				Required:    true,
				Validators: []validator.Int64{
					int64validator.Between(1, MaxDurationMinutes),
				},
			},
			"checklist": schema.SetAttribute{
				Description: "IDs of the checks to disable for the duration of the window. Order is not significant.",
				Required:    true,
				ElementType: types.StringType,
				Validators: []validator.Set{
					setvalidator.SizeBetween(1, MaxChecklistEntries),
					setvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
				},
			},
			"processing": schema.BoolAttribute{
				Description: "True while NodePing is currently running this maintenance window.",
				Computed:    true,
			},
			"nextrun": schema.Int64Attribute{
				Description: "Unix timestamp in milliseconds of the next scheduled run, as reported by NodePing.",
				Computed:    true,
			},
		},
	}
}
