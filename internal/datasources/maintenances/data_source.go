package maintenances

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/atlanticbt/terraform-provider-nodeping/internal/client"
)

var (
	_ datasource.DataSource              = &MaintenancesDataSource{}
	_ datasource.DataSourceWithConfigure = &MaintenancesDataSource{}
)

type MaintenancesDataSource struct {
	client *client.Client
}

type MaintenancesDataSourceModel struct {
	CustomerID   types.String       `tfsdk:"customer_id"`
	Maintenances []MaintenanceModel `tfsdk:"maintenances"`
}

type MaintenanceModel struct {
	ID           types.String   `tfsdk:"id"`
	CustomerID   types.String   `tfsdk:"customer_id"`
	Name         types.String   `tfsdk:"name"`
	Enabled      types.Bool     `tfsdk:"enabled"`
	Cron         types.String   `tfsdk:"cron"`
	Duration     types.Int64    `tfsdk:"duration"`
	Checklist    []types.String `tfsdk:"checklist"`
	Processing   types.Bool     `tfsdk:"processing"`
	NextRun      types.Int64    `tfsdk:"nextrun"`
	AdHoc        types.Bool     `tfsdk:"ad_hoc"`
	LastEditedBy types.String   `tfsdk:"last_edited_by"`
	LastEditedAt types.Int64    `tfsdk:"last_edited_at"`
}

func NewMaintenancesDataSource() datasource.DataSource {
	return &MaintenancesDataSource{}
}

func (d *MaintenancesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_maintenances"
}

func (d *MaintenancesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches all NodePing maintenance windows.",
		MarkdownDescription: `
Fetches all NodePing maintenance windows, sorted by ID.

The list includes ad-hoc windows that happen to exist at the moment of the read.
NodePing deletes an ad-hoc window as soon as it finishes running, so the count
returned by this data source is not stable over time. Filter on ` + "`ad_hoc`" + ` when
that matters.

## Example Usage

` + "```hcl" + `
data "nodeping_maintenances" "all" {}

output "scheduled_window_ids" {
  value = [for m in data.nodeping_maintenances.all.maintenances : m.id if !m.ad_hoc]
}
` + "```" + `
`,
		Attributes: map[string]schema.Attribute{
			"customer_id": schema.StringAttribute{
				Description: "The customer ID (SubAccount ID) to list windows for. " +
					"Leave unset to use the provider-level account.",
				Optional: true,
			},
			"maintenances": schema.ListNestedAttribute{
				Description: "Maintenance windows, sorted by ID.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "The unique identifier of the maintenance window.",
							Computed:    true,
						},
						"customer_id": schema.StringAttribute{
							Description: "The customer ID that owns this window.",
							Computed:    true,
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
							Description: "The 5-field cron expression. Empty for ad-hoc windows.",
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
							Description: "True while NodePing is currently running this window.",
							Computed:    true,
						},
						"nextrun": schema.Int64Attribute{
							Description: "Unix timestamp in milliseconds of the next scheduled run.",
							Computed:    true,
						},
						"ad_hoc": schema.BoolAttribute{
							Description: "True when this is an ad-hoc window. Ad-hoc windows are deleted by " +
								"NodePing once they finish and cannot be managed by the nodeping_maintenance resource.",
							Computed: true,
						},
						"last_edited_by": schema.StringAttribute{
							Description: "Email address of the account that last edited this window.",
							Computed:    true,
						},
						"last_edited_at": schema.Int64Attribute{
							Description: "Unix timestamp in milliseconds of the last edit.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func (d *MaintenancesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

// clientFor scopes the listing to a SubAccount when customer_id is configured.
func (d *MaintenancesDataSource) clientFor(customerID types.String) *client.Client {
	if customerID.IsNull() || customerID.IsUnknown() || customerID.ValueString() == "" {
		return d.client
	}
	return d.client.WithCustomerID(customerID.ValueString())
}

func (d *MaintenancesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config MaintenancesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading maintenances data source", map[string]interface{}{
		"customer_id": config.CustomerID.ValueString(),
	})

	windows, err := d.clientFor(config.CustomerID).ListMaintenance(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Maintenance Windows",
			"Could not list maintenance windows: "+err.Error(),
		)
		return
	}

	config.Maintenances = MapMaintenances(windows)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// MapMaintenances converts the API's ID-keyed map into a list ordered by ID.
// Ranging the map directly yields Go's randomized iteration order, which would
// make the data source produce a different result on every read.
func MapMaintenances(windows map[string]client.Maintenance) []MaintenanceModel {
	keys := make([]string, 0, len(windows))
	for key := range windows {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]MaintenanceModel, 0, len(keys))
	for _, key := range keys {
		maint := windows[key]

		id := maint.Identifier()
		if id == "" {
			id = key
		}

		model := MaintenanceModel{
			ID:         types.StringValue(id),
			CustomerID: types.StringValue(maint.CustomerID),
			Name:       types.StringValue(maint.Name),
			Enabled:    types.BoolValue(maint.Enabled),
			Cron:       types.StringValue(maint.Cron),
			Duration:   types.Int64Value(int64(maint.Duration)),
			Processing: types.BoolValue(maint.Processing),
			AdHoc:      types.BoolValue(maint.IsAdHoc()),
			Checklist:  sortedStrings(maint.Checklist),
		}

		if maint.NextRun != nil {
			model.NextRun = types.Int64Value(*maint.NextRun)
		} else {
			model.NextRun = types.Int64Null()
		}

		if maint.Update != nil {
			model.LastEditedBy = types.StringValue(maint.Update.User)
			model.LastEditedAt = types.Int64Value(maint.Update.Timestamp)
		} else {
			model.LastEditedBy = types.StringNull()
			model.LastEditedAt = types.Int64Null()
		}

		out = append(out, model)
	}

	return out
}

func sortedStrings(in []string) []types.String {
	sorted := append([]string(nil), in...)
	sort.Strings(sorted)

	out := make([]types.String, 0, len(sorted))
	for _, s := range sorted {
		out = append(out, types.StringValue(s))
	}
	return out
}
