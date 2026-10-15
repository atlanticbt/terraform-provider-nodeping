package schedule

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/atlanticbt/terraform-provider-nodeping/internal/client"
)

var (
	_ datasource.DataSource              = &ScheduleDataSource{}
	_ datasource.DataSourceWithConfigure = &ScheduleDataSource{}
)

type ScheduleDataSource struct {
	client *client.Client
}

type ScheduleDataSourceModel struct {
	ID         types.String `tfsdk:"id"`
	CustomerID types.String `tfsdk:"customer_id"`
	Monday     *DayModel    `tfsdk:"monday"`
	Tuesday    *DayModel    `tfsdk:"tuesday"`
	Wednesday  *DayModel    `tfsdk:"wednesday"`
	Thursday   *DayModel    `tfsdk:"thursday"`
	Friday     *DayModel    `tfsdk:"friday"`
	Saturday   *DayModel    `tfsdk:"saturday"`
	Sunday     *DayModel    `tfsdk:"sunday"`
}

type DayModel struct {
	Time1    types.String `tfsdk:"time1"`
	Time2    types.String `tfsdk:"time2"`
	Exclude  types.Bool   `tfsdk:"exclude"`
	Disabled types.Bool   `tfsdk:"disabled"`
	AllDay   types.Bool   `tfsdk:"allday"`
}

func (m *ScheduleDataSourceModel) setDay(name string, day *DayModel) {
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

func NewScheduleDataSource() datasource.DataSource {
	return &ScheduleDataSource{}
}

func (d *ScheduleDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_schedule"
}

func (d *ScheduleDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches a NodePing notification schedule by ID.",
		MarkdownDescription: `
Fetches a NodePing notification schedule by ID.

Schedule IDs are chosen by whoever created the schedule, so the ID is the name
you gave it (for example ` + "`BusinessHours`" + `).

## Example Usage

` + "```hcl" + `
data "nodeping_schedule" "business_hours" {
  id = "BusinessHours"
}

output "monday_start" {
  value = data.nodeping_schedule.business_hours.monday.time1
}
` + "```" + `
`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The schedule name.",
				Required:    true,
			},
			"customer_id": schema.StringAttribute{
				Description: "The customer ID (SubAccount ID) to look the schedule up under. " +
					"Omit to use the account the provider is configured for.",
				Optional: true,
				Computed: true,
			},
			"monday":    DayAttribute("Monday"),
			"tuesday":   DayAttribute("Tuesday"),
			"wednesday": DayAttribute("Wednesday"),
			"thursday":  DayAttribute("Thursday"),
			"friday":    DayAttribute("Friday"),
			"saturday":  DayAttribute("Saturday"),
			"sunday":    DayAttribute("Sunday"),
		},
	}
}

// DayAttribute describes one day of a schedule. It is exported so the plural
// data source can reuse exactly the same shape.
func DayAttribute(day string) schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Description: "Notification window for " + day + ", or null when " + day + " is unscheduled.",
		Computed:    true,
		Attributes:  DayAttributes(),
	}
}

func DayAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"time1": schema.StringAttribute{
			Description: "Start of the notification span (24-hour H:MM or HH:MM).",
			Computed:    true,
		},
		"time2": schema.StringAttribute{
			Description: "End of the notification span (24-hour H:MM or HH:MM).",
			Computed:    true,
		},
		"exclude": schema.BoolAttribute{
			Description: "Whether the span is inverted: notify all day except between time1 and time2.",
			Computed:    true,
		},
		"disabled": schema.BoolAttribute{
			Description: "Whether notifications are disabled for the day.",
			Computed:    true,
		},
		"allday": schema.BoolAttribute{
			Description: "Whether notifications are enabled for the whole day.",
			Computed:    true,
		},
	}
}

func (d *ScheduleDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ScheduleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ScheduleDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading schedule data source", map[string]interface{}{
		"id": config.ID.ValueString(),
	})

	schedule, err := d.clientFor(config.CustomerID).GetSchedule(ctx, config.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Schedule",
			"Could not read schedule ID "+config.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	config.ID = types.StringValue(schedule.ID)
	if config.CustomerID.IsNull() || config.CustomerID.IsUnknown() || config.CustomerID.ValueString() == "" {
		if schedule.CustomerID != "" {
			config.CustomerID = types.StringValue(schedule.CustomerID)
		} else {
			config.CustomerID = types.StringNull()
		}
	}

	// Iterate the fixed day list, never the API map: ranging a map yields Go's
	// randomised order and would make the data source unstable.
	for _, name := range client.ScheduleDayNames {
		apiDay, ok := schedule.Data[name]
		if !ok {
			config.setDay(name, nil)
			continue
		}
		config.setDay(name, MapDay(apiDay))
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func (d *ScheduleDataSource) clientFor(customerID types.String) *client.Client {
	if customerID.IsNull() || customerID.IsUnknown() || customerID.ValueString() == "" {
		return d.client
	}
	return d.client.WithCustomerID(customerID.ValueString())
}

// MapDay converts one API day into its data source representation.
func MapDay(apiDay client.ScheduleDay) *DayModel {
	day := &DayModel{
		Time1:    types.StringNull(),
		Time2:    types.StringNull(),
		Exclude:  types.BoolValue(apiDay.Exclude.Bool()),
		Disabled: types.BoolValue(apiDay.Disabled.Bool()),
		AllDay:   types.BoolValue(apiDay.AllDay.Bool()),
	}
	if apiDay.Time1 != "" {
		day.Time1 = types.StringValue(string(apiDay.Time1))
	}
	if apiDay.Time2 != "" {
		day.Time2 = types.StringValue(string(apiDay.Time2))
	}
	return day
}
