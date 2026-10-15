package schedules

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/atlanticbt/terraform-provider-nodeping/internal/client"
	scheduleds "github.com/atlanticbt/terraform-provider-nodeping/internal/datasources/schedule"
)

var (
	_ datasource.DataSource              = &SchedulesDataSource{}
	_ datasource.DataSourceWithConfigure = &SchedulesDataSource{}
)

type SchedulesDataSource struct {
	client *client.Client
}

type SchedulesDataSourceModel struct {
	CustomerID types.String    `tfsdk:"customer_id"`
	Schedules  []ScheduleModel `tfsdk:"schedules"`
}

// ScheduleModel mirrors the singular data source so both surfaces expose the
// same day shape.
type ScheduleModel struct {
	ID        types.String         `tfsdk:"id"`
	Monday    *scheduleds.DayModel `tfsdk:"monday"`
	Tuesday   *scheduleds.DayModel `tfsdk:"tuesday"`
	Wednesday *scheduleds.DayModel `tfsdk:"wednesday"`
	Thursday  *scheduleds.DayModel `tfsdk:"thursday"`
	Friday    *scheduleds.DayModel `tfsdk:"friday"`
	Saturday  *scheduleds.DayModel `tfsdk:"saturday"`
	Sunday    *scheduleds.DayModel `tfsdk:"sunday"`
}

func (m *ScheduleModel) setDay(name string, day *scheduleds.DayModel) {
	switch name {
	case "monday":
		m.Monday = day
	case "tuesday":
		m.Tuesday = day
	case "wednesday":
		m.Wednesday = day
	case "thursday":
		m.Thursday = day
	case "friday":
		m.Friday = day
	case "saturday":
		m.Saturday = day
	case "sunday":
		m.Sunday = day
	}
}

func NewSchedulesDataSource() datasource.DataSource {
	return &SchedulesDataSource{}
}

func (d *SchedulesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_schedules"
}

func (d *SchedulesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches all NodePing notification schedules.",
		MarkdownDescription: `
Fetches all NodePing notification schedules, ordered by schedule ID.

## Example Usage

` + "```hcl" + `
data "nodeping_schedules" "all" {}

output "schedule_ids" {
  value = [for s in data.nodeping_schedules.all.schedules : s.id]
}
` + "```" + `
`,
		Attributes: map[string]schema.Attribute{
			"customer_id": schema.StringAttribute{
				Description: "The customer ID (SubAccount ID) to list schedules for. " +
					"Omit to use the account the provider is configured for.",
				Optional: true,
			},
			"schedules": schema.ListNestedAttribute{
				Description: "All schedules, ordered by ID.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "The schedule name.",
							Computed:    true,
						},
						"monday":    scheduleds.DayAttribute("Monday"),
						"tuesday":   scheduleds.DayAttribute("Tuesday"),
						"wednesday": scheduleds.DayAttribute("Wednesday"),
						"thursday":  scheduleds.DayAttribute("Thursday"),
						"friday":    scheduleds.DayAttribute("Friday"),
						"saturday":  scheduleds.DayAttribute("Saturday"),
						"sunday":    scheduleds.DayAttribute("Sunday"),
					},
				},
			},
		},
	}
}

func (d *SchedulesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SchedulesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config SchedulesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading schedules data source")

	schedules, err := d.clientFor(config.CustomerID).ListSchedules(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Schedules",
			"Could not list schedules: "+err.Error(),
		)
		return
	}

	config.Schedules = MapSchedules(schedules)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func (d *SchedulesDataSource) clientFor(customerID types.String) *client.Client {
	if customerID.IsNull() || customerID.IsUnknown() || customerID.ValueString() == "" {
		return d.client
	}
	return d.client.WithCustomerID(customerID.ValueString())
}

// MapSchedules converts the API's schedule map into a list ordered by schedule
// ID. Ranging the map directly yields Go's randomised iteration order, which
// would make the data source produce a different result on every read.
func MapSchedules(schedules map[string]client.Schedule) []ScheduleModel {
	keys := make([]string, 0, len(schedules))
	for key := range schedules {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	result := make([]ScheduleModel, 0, len(keys))
	for _, key := range keys {
		schedule := schedules[key]

		id := schedule.ID
		if id == "" {
			id = key
		}

		model := ScheduleModel{ID: types.StringValue(id)}
		for _, name := range client.ScheduleDayNames {
			apiDay, ok := schedule.Data[name]
			if !ok {
				model.setDay(name, nil)
				continue
			}
			model.setDay(name, scheduleds.MapDay(apiDay))
		}

		result = append(result, model)
	}

	return result
}
