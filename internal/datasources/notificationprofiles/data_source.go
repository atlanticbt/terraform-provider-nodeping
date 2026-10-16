package notificationprofiles

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/client"
	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/datasources/notificationprofile"
)

var (
	_ datasource.DataSource              = &NotificationProfilesDataSource{}
	_ datasource.DataSourceWithConfigure = &NotificationProfilesDataSource{}
)

type NotificationProfilesDataSource struct {
	client *client.Client
}

type NotificationProfilesDataSourceModel struct {
	CustomerID types.String                   `tfsdk:"customer_id"`
	Profiles   []NotificationProfileListModel `tfsdk:"profiles"`
}

type NotificationProfileListModel struct {
	ID            types.String                                      `tfsdk:"id"`
	CustomerID    types.String                                      `tfsdk:"customer_id"`
	Name          types.String                                      `tfsdk:"name"`
	Notifications []notificationprofile.NotificationDataSourceModel `tfsdk:"notifications"`
}

func NewNotificationProfilesDataSource() datasource.DataSource {
	return &NotificationProfilesDataSource{}
}

func (d *NotificationProfilesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notificationprofiles"
}

func (d *NotificationProfilesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches all NodePing notification profiles.",
		MarkdownDescription: `
Fetches all NodePing notification profiles, ordered by profile ID.

## Example Usage

` + "```hcl" + `
data "nodeping_notificationprofiles" "all" {}

output "profile_names" {
  value = { for p in data.nodeping_notificationprofiles.all.profiles : p.id => p.name }
}
` + "```" + `
`,
		Attributes: map[string]schema.Attribute{
			"customer_id": schema.StringAttribute{
				Description: "The subaccount (customer) ID to list profiles for. Defaults to the account the provider is configured for.",
				Optional:    true,
			},
			"profiles": schema.ListNestedAttribute{
				Description: "Notification profiles, ordered by ID.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "The unique identifier of the notification profile.",
							Computed:    true,
						},
						"customer_id": schema.StringAttribute{
							Description: "The customer ID (account ID) that owns this profile.",
							Computed:    true,
						},
						"name": schema.StringAttribute{
							Description: "The name of the notification profile.",
							Computed:    true,
						},
						"notifications": schema.ListNestedAttribute{
							Description: "Notification targets belonging to the profile.",
							Computed:    true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"contact_id": schema.StringAttribute{
										Description: "Contact or contact group ID that is notified.",
										Computed:    true,
									},
									"delay": schema.Int64Attribute{
										Description: "Delay in minutes before this target is notified.",
										Computed:    true,
									},
									"schedule": schema.StringAttribute{
										Description: "Notification schedule name.",
										Computed:    true,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (d *NotificationProfilesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

// clientFor scopes the listing to the configured subaccount.
func (d *NotificationProfilesDataSource) clientFor(customerID types.String) *client.Client {
	if customerID.IsNull() || customerID.IsUnknown() || customerID.ValueString() == "" {
		return d.client
	}
	return d.client.WithCustomerID(customerID.ValueString())
}

func (d *NotificationProfilesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config NotificationProfilesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading notification profiles data source", map[string]interface{}{
		"customer_id": config.CustomerID.ValueString(),
	})

	profiles, err := d.clientFor(config.CustomerID).ListNotificationProfiles(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Notification Profiles",
			"Could not list notification profiles: "+err.Error(),
		)
		return
	}

	config.Profiles = MapProfiles(profiles)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// MapProfiles converts the API's profile map into a list ordered by profile ID.
// Ranging the map directly yields Go's randomized iteration order, which makes
// the data source produce a different result on every read.
func MapProfiles(profiles map[string]client.NotificationProfile) []NotificationProfileListModel {
	keys := make([]string, 0, len(profiles))
	for key := range profiles {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	result := make([]NotificationProfileListModel, 0, len(keys))
	for _, key := range keys {
		profile := profiles[key]

		id := profile.ID
		if id == "" {
			id = key
		}

		result = append(result, NotificationProfileListModel{
			ID:            types.StringValue(id),
			CustomerID:    types.StringValue(profile.CustomerID),
			Name:          types.StringValue(profile.Name),
			Notifications: notificationprofile.MapNotifications(profile.Notifications),
		})
	}

	return result
}
