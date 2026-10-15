package check

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/atlanticbt/terraform-provider-nodeping/internal/client"
)

var _ datasource.DataSource = &CheckDataSource{}
var _ datasource.DataSourceWithConfigure = &CheckDataSource{}

type CheckDataSource struct {
	client *client.Client
}

type CheckDataSourceModel struct {
	ID          types.String  `tfsdk:"id"`
	CustomerID  types.String  `tfsdk:"customer_id"`
	Type        types.String  `tfsdk:"type"`
	Target      types.String  `tfsdk:"target"`
	Label       types.String  `tfsdk:"label"`
	Enabled     types.Bool    `tfsdk:"enabled"`
	Public      types.Bool    `tfsdk:"public"`
	Interval    types.Float64 `tfsdk:"interval"`
	Threshold   types.Int64   `tfsdk:"threshold"`
	Sens        types.Int64   `tfsdk:"sens"`
	Dep         types.String  `tfsdk:"dep"`
	State       types.Int64   `tfsdk:"state"`
	Created     types.Int64   `tfsdk:"created"`
	Modified    types.Int64   `tfsdk:"modified"`
	Description types.String  `tfsdk:"description"`
	Tags        types.List    `tfsdk:"tags"`
}

func NewCheckDataSource() datasource.DataSource {
	return &CheckDataSource{}
}

func (d *CheckDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_check"
}

func (d *CheckDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches a NodePing check by ID.",
		MarkdownDescription: `
Fetches a NodePing check by ID.

## Example Usage

` + "```hcl" + `
data "nodeping_check" "example" {
  id = "201205050153W2Q4C-0J2HSIRF"
}

output "check_state" {
  value = data.nodeping_check.example.state
}
` + "```" + `
`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the check.",
				Required:    true,
			},
			"customer_id": schema.StringAttribute{
				Description: "The customer ID (account ID) that owns this check. Set this to look up a " +
					"check that belongs to a subaccount.",
				Optional: true,
				Computed: true,
			},
			"type": schema.StringAttribute{
				Description: "The type of check.",
				Computed:    true,
			},
			"target": schema.StringAttribute{
				Description: "The target URL, hostname, or IP address.",
				Computed:    true,
			},
			"label": schema.StringAttribute{
				Description: "The label for the check.",
				Computed:    true,
			},
			"enabled": schema.BoolAttribute{
				Description: "Whether the check is enabled.",
				Computed:    true,
			},
			"public": schema.BoolAttribute{
				Description: "Whether public reports are enabled.",
				Computed:    true,
			},
			"interval": schema.Float64Attribute{
				Description: "Check interval in minutes.",
				Computed:    true,
			},
			"threshold": schema.Int64Attribute{
				Description: "Timeout in seconds.",
				Computed:    true,
			},
			"sens": schema.Int64Attribute{
				Description: "Sensitivity (rechecks before status change).",
				Computed:    true,
			},
			"dep": schema.StringAttribute{
				Description: "Check ID for notification dependency.",
				Computed:    true,
			},
			"state": schema.Int64Attribute{
				Description: "Current state (0 = failing, 1 = passing).",
				Computed:    true,
			},
			"created": schema.Int64Attribute{
				Description: "Creation timestamp.",
				Computed:    true,
			},
			"modified": schema.Int64Attribute{
				Description: "Last modification timestamp.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "Description of the check.",
				Computed:    true,
			},
			"tags": schema.ListAttribute{
				Description: "Tags for the check.",
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (d *CheckDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *CheckDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config CheckDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading check data source", map[string]interface{}{
		"id": config.ID.ValueString(),
	})

	c := d.client
	if !config.CustomerID.IsNull() && !config.CustomerID.IsUnknown() && config.CustomerID.ValueString() != "" {
		c = c.WithCustomerID(config.CustomerID.ValueString())
	}

	check, err := c.GetCheck(ctx, config.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Check",
			"Could not read check ID "+config.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	config.CustomerID = types.StringValue(check.CustomerID)
	config.Type = types.StringValue(check.Type)
	config.Target = types.StringValue(check.Parameters.Target)
	config.Label = types.StringValue(check.Label)
	config.Enabled = types.BoolValue(check.Enabled == "active" || check.Enabled == "true")
	config.Public = types.BoolValue(check.Public)

	if check.Interval.String() != "" {
		if interval, err := check.Interval.Float64(); err == nil {
			config.Interval = types.Float64Value(interval)
		} else {
			resp.Diagnostics.AddError(
				"Unexpected API Response",
				"Could not parse interval for check "+check.ID+": "+err.Error(),
			)
		}
	}

	config.Threshold = numberOrNull(check.Parameters.Threshold)
	config.Sens = numberOrNull(check.Parameters.Sens)

	config.State = types.Int64Value(int64(check.State))
	config.Created = types.Int64Value(check.Created)
	config.Modified = types.Int64Value(check.Modified)

	if dep, ok := check.Dep.(string); ok && dep != "" {
		config.Dep = types.StringValue(dep)
	} else {
		config.Dep = types.StringNull()
	}

	if check.Description != "" {
		config.Description = types.StringValue(check.Description)
	} else {
		config.Description = types.StringNull()
	}

	if check.Tags != nil {
		tags, diags := types.ListValueFrom(ctx, types.StringType, check.Tags)
		resp.Diagnostics.Append(diags...)
		config.Tags = tags
	} else {
		config.Tags = types.ListNull(types.StringType)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// numberOrNull decodes the numeric representations the NodePing API uses
// interchangeably (JSON numbers, json.Number and decimal strings).
func numberOrNull(v interface{}) types.Int64 {
	switch val := v.(type) {
	case float64:
		return types.Int64Value(int64(val))
	case int:
		return types.Int64Value(int64(val))
	case int64:
		return types.Int64Value(val)
	case json.Number:
		if i, err := val.Int64(); err == nil {
			return types.Int64Value(i)
		}
	case string:
		if i, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64); err == nil {
			return types.Int64Value(i)
		}
	}
	return types.Int64Null()
}
