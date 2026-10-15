package notificationprofile

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/atlanticbt/terraform-provider-nodeping/internal/client"
)

var (
	_ datasource.DataSource              = &NotificationProfileDataSource{}
	_ datasource.DataSourceWithConfigure = &NotificationProfileDataSource{}
)

// defaultSchedule is what NodePing falls back to when a notification carries no
// schedule of its own.
const defaultSchedule = "All"

type NotificationProfileDataSource struct {
	client *client.Client
}

type NotificationProfileDataSourceModel struct {
	ID            types.String                  `tfsdk:"id"`
	CustomerID    types.String                  `tfsdk:"customer_id"`
	Name          types.String                  `tfsdk:"name"`
	Notifications []NotificationDataSourceModel `tfsdk:"notifications"`
}

type NotificationDataSourceModel struct {
	ContactID types.String `tfsdk:"contact_id"`
	Delay     types.Int64  `tfsdk:"delay"`
	Schedule  types.String `tfsdk:"schedule"`
}

func NewNotificationProfileDataSource() datasource.DataSource {
	return &NotificationProfileDataSource{}
}

func (d *NotificationProfileDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notificationprofile"
}

func (d *NotificationProfileDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches a NodePing notification profile by ID.",
		MarkdownDescription: `
Fetches a NodePing notification profile by ID.

## Example Usage

` + "```hcl" + `
data "nodeping_notificationprofile" "escalation" {
  id = "201205050153W2Q4C-P-3QJWG"
}

output "escalation_targets" {
  value = [for n in data.nodeping_notificationprofile.escalation.notifications : n.contact_id]
}
` + "```" + `
`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the notification profile.",
				Required:    true,
			},
			"customer_id": schema.StringAttribute{
				Description: "The subaccount (customer) ID to look the profile up in. Defaults to the account the provider is configured for.",
				Optional:    true,
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
	}
}

func (d *NotificationProfileDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

// clientFor scopes the lookup to the configured subaccount. Without it a
// profile that lives on a subaccount is reported as missing.
func (d *NotificationProfileDataSource) clientFor(customerID types.String) *client.Client {
	if customerID.IsNull() || customerID.IsUnknown() || customerID.ValueString() == "" {
		return d.client
	}
	return d.client.WithCustomerID(customerID.ValueString())
}

func (d *NotificationProfileDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config NotificationProfileDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading notification profile data source", map[string]interface{}{
		"id": config.ID.ValueString(),
	})

	profile, err := d.clientFor(config.CustomerID).GetNotificationProfile(ctx, config.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Notification Profile",
			"Could not read notification profile ID "+config.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	if profile.ID != "" {
		config.ID = types.StringValue(profile.ID)
	}
	if profile.CustomerID != "" {
		config.CustomerID = types.StringValue(profile.CustomerID)
	} else if config.CustomerID.IsNull() || config.CustomerID.IsUnknown() {
		config.CustomerID = types.StringValue("")
	}
	config.Name = types.StringValue(profile.Name)
	config.Notifications = MapNotifications(profile.Notifications)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// MapNotifications flattens the API representation
// ([]{contact_id: {delay, schedule}}) into an ordered list. Contact IDs inside a
// single element are visited in sorted order so the result is deterministic
// regardless of Go's randomized map iteration order, and repeated entries are
// preserved rather than collapsed.
func MapNotifications(notifications []map[string]interface{}) []NotificationDataSourceModel {
	result := make([]NotificationDataSourceModel, 0, len(notifications))

	for _, n := range notifications {
		contactIDs := make([]string, 0, len(n))
		for contactID := range n {
			contactIDs = append(contactIDs, contactID)
		}
		sort.Strings(contactIDs)

		for _, contactID := range contactIDs {
			configMap, ok := n[contactID].(map[string]interface{})
			if !ok {
				continue
			}
			delay := int64(0)
			if d, ok := ToInt64(configMap["delay"]); ok {
				delay = d
			}
			schedule := defaultSchedule
			if s, ok := configMap["schedule"].(string); ok && s != "" {
				schedule = s
			}
			result = append(result, NotificationDataSourceModel{
				ContactID: types.StringValue(contactID),
				Delay:     types.Int64Value(delay),
				Schedule:  types.StringValue(schedule),
			})
		}
	}

	return result
}

// ToInt64 decodes the numeric representations NodePing uses interchangeably:
// JSON numbers, json.Number and decimal strings.
func ToInt64(v interface{}) (int64, bool) {
	switch val := v.(type) {
	case float64:
		return int64(val), true
	case float32:
		return int64(val), true
	case int:
		return int64(val), true
	case int64:
		return val, true
	case json.Number:
		if i, err := val.Int64(); err == nil {
			return i, true
		}
		if f, err := val.Float64(); err == nil {
			return int64(f), true
		}
		return 0, false
	case string:
		trimmed := strings.TrimSpace(val)
		if trimmed == "" {
			return 0, false
		}
		if i, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return i, true
		}
		if f, err := strconv.ParseFloat(trimmed, 64); err == nil {
			return int64(f), true
		}
		return 0, false
	default:
		return 0, false
	}
}
