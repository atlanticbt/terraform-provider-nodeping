package account

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/client"
)

var (
	_ datasource.DataSource              = &AccountDataSource{}
	_ datasource.DataSourceWithConfigure = &AccountDataSource{}
)

type AccountDataSource struct {
	client *client.Client
}

type AccountDataSourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Type             types.String `tfsdk:"type"`
	Status           types.String `tfsdk:"status"`
	Timezone         types.String `tfsdk:"timezone"`
	CreationDate     types.Int64  `tfsdk:"creation_date"`
	NextBillingDate  types.String `tfsdk:"next_billing_date"`
	DefaultLocations types.List   `tfsdk:"default_locations"`
}

func NewAccountDataSource() datasource.DataSource {
	return &AccountDataSource{}
}

func (d *AccountDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account"
}

func (d *AccountDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches a NodePing account or subaccount.",
		MarkdownDescription: `
Fetches a NodePing account or subaccount.

Omit ` + "`id`" + ` to read the account the configured API token belongs to. Supply an
account ID to read a subaccount.

## Example Usage

` + "```hcl" + `
# The account the API token belongs to.
data "nodeping_account" "primary" {}

# A specific subaccount.
data "nodeping_account" "client" {
  id = "201203232048C76FH"
}

output "primary_timezone" {
  value = data.nodeping_account.primary.timezone
}
` + "```" + `

NodePing does not return ` + "`contactname`" + `, ` + "`email`" + `,
` + "`emailme`" + ` or ` + "`autodiagnotifications`" + ` from any read, so this data
source cannot expose them.
`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The NodePing account ID. Leave unset to read the account the API token belongs to.",
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "The account name, returned by the API as customer_name.",
				Computed:    true,
			},
			"type": schema.StringAttribute{
				Description: "The account type reported by NodePing, for example 'customer' or 'subcustomer'.",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "The account status, 'Active' or 'Suspend'.",
				Computed:    true,
			},
			"timezone": schema.StringAttribute{
				Description: "The account's GMT offset as a string, for example \"-7\".",
				Computed:    true,
			},
			"creation_date": schema.Int64Attribute{
				Description: "The account creation timestamp reported by NodePing.",
				Computed:    true,
			},
			"next_billing_date": schema.StringAttribute{
				Description: "The next billing date reported by NodePing.",
				Computed:    true,
			},
			"default_locations": schema.ListAttribute{
				Description: "The account's default probe regions.",
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (d *AccountDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *AccountDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config AccountDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := ""
	if !config.ID.IsNull() && !config.ID.IsUnknown() {
		id = config.ID.ValueString()
	}

	tflog.Debug(ctx, "Reading account data source", map[string]interface{}{"id": id})

	account, err := d.client.GetAccount(ctx, id)
	if err != nil {
		target := "the primary account"
		if id != "" {
			target = "account ID " + id
		}
		resp.Diagnostics.AddError(
			"Error Reading Account",
			"Could not read "+target+": "+err.Error(),
		)
		return
	}

	state := AccountDataSourceModel{
		ID:              types.StringValue(account.ID),
		Name:            types.StringValue(account.CustomerName),
		Type:            types.StringValue(account.Type),
		Status:          types.StringValue(account.Status),
		Timezone:        types.StringValue(account.Timezone),
		CreationDate:    types.Int64Value(account.CreationDate),
		NextBillingDate: types.StringValue(account.NextBillingDate),
	}

	locations, diags := types.ListValueFrom(ctx, types.StringType, account.DefaultLocations)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.DefaultLocations = locations

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
