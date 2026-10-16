package maintenance

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/client"
)

var (
	_ datasource.DataSource              = &MaintenanceDataSource{}
	_ datasource.DataSourceWithConfigure = &MaintenanceDataSource{}
)

type MaintenanceDataSource struct {
	client *client.Client
}

type MaintenanceDataSourceModel struct {
	ID           types.String   `tfsdk:"id"`
	CustomerID   types.String   `tfsdk:"customer_id"`
	Name         types.String   `tfsdk:"name"`
	Enabled      types.Bool     `tfsdk:"enabled"`
	Cron         types.String   `tfsdk:"cron"`
	Duration     types.Int64    `tfsdk:"duration"`
	Checklist    []types.String `tfsdk:"checklist"`
	Processing   types.Bool     `tfsdk:"processing"`
	NextRun      types.Int64    `tfsdk:"nextrun"`
	LastEditedBy types.String   `tfsdk:"last_edited_by"`
	LastEditedAt types.Int64    `tfsdk:"last_edited_at"`
	History      []HistoryModel `tfsdk:"history"`
}

// HistoryModel is one recorded run of a maintenance window.
type HistoryModel struct {
	Timestamp      types.Int64    `tfsdk:"timestamp"`
	DisabledChecks []types.String `tfsdk:"disabled_checks"`
	SkippedChecks  []types.String `tfsdk:"skipped_checks"`
}

func NewMaintenanceDataSource() datasource.DataSource {
	return &MaintenanceDataSource{}
}

func (d *MaintenanceDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_maintenance"
}

func (d *MaintenanceDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches a NodePing maintenance window by ID.",
		MarkdownDescription: `
Fetches a NodePing maintenance window by ID.

Unlike the ` + "`nodeping_maintenance`" + ` resource, this data source will also return an
ad-hoc window while it exists. NodePing deletes ad-hoc windows once they finish
running, so a configuration that reads one by ID will start failing without
warning. Prefer reading scheduled windows.

## Example Usage

` + "```hcl" + `
data "nodeping_maintenance" "patching" {
  id = "9VKOB"
}

output "next_run" {
  value = data.nodeping_maintenance.patching.nextrun
}
` + "```" + `
`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the maintenance window.",
				Required:    true,
			},
			"customer_id": schema.StringAttribute{
				Description: "The customer ID (SubAccount ID) to look the window up in. " +
					"Leave unset to use the provider-level account.",
				Optional: true,
				Computed: true,
			},
			"name": schema.StringAttribute{
				Description: "Friendly name for the maintenance window.",
				Computed:    true,
			},
			"enabled": schema.BoolAttribute{
				Description: "Whether the schedule is active.",
				Computed:    true,
			},
			"cron": schema.StringAttribute{
				Description: "The 5-field cron expression giving the window's start time. Empty for ad-hoc windows.",
				Computed:    true,
			},
			"duration": schema.Int64Attribute{
				Description: "How long the checks stay disabled, in minutes.",
				Computed:    true,
			},
			"checklist": schema.ListAttribute{
				Description: "IDs of the checks disabled by this window, sorted by ID.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"processing": schema.BoolAttribute{
				Description: "True while NodePing is currently running this maintenance window.",
				Computed:    true,
			},
			"nextrun": schema.Int64Attribute{
				Description: "Unix timestamp in milliseconds of the next scheduled run.",
				Computed:    true,
			},
			"last_edited_by": schema.StringAttribute{
				Description: "Email address of the account that last edited this window.",
				Computed:    true,
			},
			"last_edited_at": schema.Int64Attribute{
				Description: "Unix timestamp in milliseconds of the last edit.",
				Computed:    true,
			},
			"history": schema.ListNestedAttribute{
				Description: "The ten most recent runs, oldest first.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"timestamp": schema.Int64Attribute{
							Description: "Unix timestamp in milliseconds of the run.",
							Computed:    true,
						},
						"disabled_checks": schema.ListAttribute{
							Description: "Check IDs that were disabled by this run.",
							Computed:    true,
							ElementType: types.StringType,
						},
						"skipped_checks": schema.ListAttribute{
							Description: "Check IDs that were skipped, typically because they were already disabled.",
							Computed:    true,
							ElementType: types.StringType,
						},
					},
				},
			},
		},
	}
}

func (d *MaintenanceDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T.", req.ProviderData),
		)
		return
	}

	d.client = c
}

// clientFor scopes the lookup to a SubAccount when customer_id is configured.
func (d *MaintenanceDataSource) clientFor(customerID types.String) *client.Client {
	if customerID.IsNull() || customerID.IsUnknown() || customerID.ValueString() == "" {
		return d.client
	}
	return d.client.WithCustomerID(customerID.ValueString())
}

func (d *MaintenanceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config MaintenanceDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := config.ID.ValueString()
	tflog.Debug(ctx, "Reading maintenance data source", map[string]interface{}{"id": id})

	maint, err := d.clientFor(config.CustomerID).GetMaintenance(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Maintenance Window",
			"Could not read maintenance window ID "+id+": "+err.Error(),
		)
		return
	}

	config.ID = types.StringValue(maint.Identifier())
	if maint.CustomerID != "" {
		config.CustomerID = types.StringValue(maint.CustomerID)
	} else if config.CustomerID.IsNull() || config.CustomerID.IsUnknown() {
		config.CustomerID = types.StringNull()
	}
	config.Name = types.StringValue(maint.Name)
	config.Enabled = types.BoolValue(maint.Enabled)
	config.Cron = types.StringValue(maint.Cron)
	config.Duration = types.Int64Value(int64(maint.Duration))
	config.Processing = types.BoolValue(maint.Processing)
	config.Checklist = SortedStrings(maint.Checklist)

	if maint.NextRun != nil {
		config.NextRun = types.Int64Value(*maint.NextRun)
	} else {
		config.NextRun = types.Int64Null()
	}

	if maint.Update != nil {
		config.LastEditedBy = types.StringValue(maint.Update.User)
		config.LastEditedAt = types.Int64Value(maint.Update.Timestamp)
	} else {
		config.LastEditedBy = types.StringNull()
		config.LastEditedAt = types.Int64Null()
	}

	config.History = MapHistory(maint)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// SortedStrings returns a sorted copy of the given IDs. The API does not
// promise an order, so sorting is what keeps repeated reads identical.
func SortedStrings(in []string) []types.String {
	sorted := append([]string(nil), in...)
	sort.Strings(sorted)

	out := make([]types.String, 0, len(sorted))
	for _, s := range sorted {
		out = append(out, types.StringValue(s))
	}
	return out
}

// MapHistory flattens the API's timestamp-keyed history map into a list ordered
// oldest first. Ranging the map directly would yield Go's randomized iteration
// order and a data source result that changes on every read.
func MapHistory(maint *client.Maintenance) []HistoryModel {
	entries := maint.SortedHistory()
	if len(entries) == 0 {
		return nil
	}

	out := make([]HistoryModel, 0, len(entries))
	for _, e := range entries {
		out = append(out, HistoryModel{
			Timestamp:      types.Int64Value(e.Timestamp),
			DisabledChecks: SortedStrings(e.DisabledChecklist),
			SkippedChecks:  SortedStrings(e.SkippedChecklist),
		})
	}
	return out
}
