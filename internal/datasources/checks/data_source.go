package checks

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/atlanticbt/terraform-provider-nodeping/internal/client"
	checkresource "github.com/atlanticbt/terraform-provider-nodeping/internal/resources/check"
)

var _ datasource.DataSource = &ChecksDataSource{}
var _ datasource.DataSourceWithConfigure = &ChecksDataSource{}

type ChecksDataSource struct {
	client *client.Client
}

type ChecksDataSourceModel struct {
	CustomerID types.String `tfsdk:"customer_id"`
	Type       types.String `tfsdk:"type"`
	Label      types.String `tfsdk:"label"`
	Tags       types.List   `tfsdk:"tags"`
	Checks     []CheckModel `tfsdk:"checks"`
}

type CheckModel struct {
	ID         types.String  `tfsdk:"id"`
	CustomerID types.String  `tfsdk:"customer_id"`
	Type       types.String  `tfsdk:"type"`
	Target     types.String  `tfsdk:"target"`
	Label      types.String  `tfsdk:"label"`
	Enabled    types.Bool    `tfsdk:"enabled"`
	Interval   types.Float64 `tfsdk:"interval"`
	Dep        types.String  `tfsdk:"dep"`
	State      types.Int64   `tfsdk:"state"`
	Tags       types.List    `tfsdk:"tags"`
}

func NewChecksDataSource() datasource.DataSource {
	return &ChecksDataSource{}
}

func (d *ChecksDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_checks"
}

func (d *ChecksDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches all NodePing checks with optional filtering.",
		MarkdownDescription: `
Fetches all NodePing checks with optional filtering.

## Example Usage

` + "```hcl" + `
data "nodeping_checks" "all" {}

data "nodeping_checks" "http_only" {
  type = "HTTP"
}

output "check_ids" {
  value = [for c in data.nodeping_checks.all.checks : c.id]
}
` + "```" + `
`,
		Attributes: map[string]schema.Attribute{
			"customer_id": schema.StringAttribute{
				Description: "The customer ID (account ID) to list checks for. Set this to list the " +
					"checks of a subaccount.",
				Optional: true,
			},
			"type": schema.StringAttribute{
				Description: "Filter checks by type.",
				Optional:    true,
				Validators: []validator.String{
					stringvalidator.OneOf(checkresource.ValidCheckTypes...),
				},
			},
			"label": schema.StringAttribute{
				Description: "Filter checks by exact label.",
				Optional:    true,
			},
			"tags": schema.ListAttribute{
				Description: "Only return checks carrying all of these tags.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"checks": schema.ListNestedAttribute{
				Description: "List of checks.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "The unique identifier of the check.",
							Computed:    true,
						},
						"customer_id": schema.StringAttribute{
							Description: "The customer ID (account ID) that owns this check.",
							Computed:    true,
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
						"interval": schema.Float64Attribute{
							Description: "Check interval in minutes.",
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
						"tags": schema.ListAttribute{
							Description: "Tags for the check.",
							Computed:    true,
							ElementType: types.StringType,
						},
					},
				},
			},
		},
	}
}

func (d *ChecksDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ChecksDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ChecksDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading checks data source")

	c := d.client
	if !config.CustomerID.IsNull() && !config.CustomerID.IsUnknown() && config.CustomerID.ValueString() != "" {
		c = c.WithCustomerID(config.CustomerID.ValueString())
	}

	checks, err := c.ListChecks(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Checks",
			"Could not list checks: "+err.Error(),
		)
		return
	}

	typeFilter := config.Type.ValueString()
	labelFilter := config.Label.ValueString()

	var tagFilter []string
	if !config.Tags.IsNull() && !config.Tags.IsUnknown() {
		resp.Diagnostics.Append(config.Tags.ElementsAs(ctx, &tagFilter, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// The API returns a map, so iterate in a deterministic order to keep the
	// resulting list stable between plans.
	ids := make([]string, 0, len(checks))
	for id := range checks {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	config.Checks = make([]CheckModel, 0, len(checks))
	for _, id := range ids {
		check := checks[id]
		if check.ID == "" {
			// The list endpoint keys checks by ID; not every response repeats
			// it inside the object.
			check.ID = id
		}
		if typeFilter != "" && !strings.EqualFold(check.Type, typeFilter) {
			continue
		}
		if labelFilter != "" && check.Label != labelFilter {
			continue
		}
		if !hasAllTags(check.Tags, tagFilter) {
			continue
		}

		checkModel := CheckModel{
			ID:         types.StringValue(check.ID),
			CustomerID: types.StringValue(check.CustomerID),
			Type:       types.StringValue(check.Type),
			Target:     types.StringValue(check.Parameters.Target),
			Label:      types.StringValue(check.Label),
			Enabled:    types.BoolValue(check.Enabled == "active" || check.Enabled == "true"),
			State:      types.Int64Value(int64(check.State)),
		}

		if check.Interval.String() != "" {
			if interval, err := check.Interval.Float64(); err == nil {
				checkModel.Interval = types.Float64Value(interval)
			} else {
				resp.Diagnostics.AddError(
					"Unexpected API Response",
					"Could not parse interval for check "+check.ID+": "+err.Error(),
				)
			}
		}

		if dep, ok := check.Dep.(string); ok && dep != "" {
			checkModel.Dep = types.StringValue(dep)
		} else {
			checkModel.Dep = types.StringNull()
		}

		if check.Tags != nil {
			tags, diags := types.ListValueFrom(ctx, types.StringType, check.Tags)
			resp.Diagnostics.Append(diags...)
			checkModel.Tags = tags
		} else {
			checkModel.Tags = types.ListNull(types.StringType)
		}

		config.Checks = append(config.Checks, checkModel)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// hasAllTags reports whether every wanted tag is present on the check.
func hasAllTags(tags []string, wanted []string) bool {
	if len(wanted) == 0 {
		return true
	}
	present := make(map[string]bool, len(tags))
	for _, t := range tags {
		present[t] = true
	}
	for _, w := range wanted {
		if !present[w] {
			return false
		}
	}
	return true
}
