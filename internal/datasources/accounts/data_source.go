package accounts

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
	_ datasource.DataSource              = &AccountsDataSource{}
	_ datasource.DataSourceWithConfigure = &AccountsDataSource{}
)

type AccountsDataSource struct {
	client *client.Client
}

type AccountsDataSourceModel struct {
	Accounts []AccountModel `tfsdk:"accounts"`
}

type AccountModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	Status          types.String `tfsdk:"status"`
	ActiveChecks    types.Int64  `tfsdk:"active_checks"`
	SubaccountCount types.Int64  `tfsdk:"subaccount_count"`
}

func NewAccountsDataSource() datasource.DataSource {
	return &AccountsDataSource{}
}

func (d *AccountsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_accounts"
}

func (d *AccountsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches the NodePing account and all of its subaccounts.",
		MarkdownDescription: `
Fetches the NodePing account the API token belongs to along with all of its subaccounts.

Entries are sorted by account ID so the list is stable between reads. NodePing returns the
listing as a JSON object keyed by account ID, which has no inherent order.

## Example Usage

` + "```hcl" + `
data "nodeping_accounts" "all" {}

output "subaccount_ids" {
  value = [
    for a in data.nodeping_accounts.all.accounts : a.id
    if a.subaccount_count == null
  ]
}
` + "```" + `

When the provider is configured with ` + "`customer_id`" + `, NodePing scopes the endpoint to that
account and returns it alone. The list then holds one entry, with ` + "`active_checks`" + ` at 0 and
` + "`subaccount_count`" + ` null, because the scoped response carries neither figure.

The listing carries only summary fields. Use the ` + "`nodeping_account`" + ` data source
for an account's timezone, default locations and billing date.

A subaccount destroyed with ` + "`nodeping_subaccount`" + ` may still appear here briefly:
NodePing queues deletions rather than performing them inline.
`,
		Attributes: map[string]schema.Attribute{
			"accounts": schema.ListNestedAttribute{
				Description: "Accounts visible to the configured API token, sorted by account ID.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "The NodePing account ID.",
							Computed:    true,
						},
						"name": schema.StringAttribute{
							Description: "The account name.",
							Computed:    true,
						},
						"status": schema.StringAttribute{
							Description: "The account status, 'Active' or 'Suspend'.",
							Computed:    true,
						},
						"active_checks": schema.Int64Attribute{
							Description: "The number of active checks in the account.",
							Computed:    true,
						},
						"subaccount_count": schema.Int64Attribute{
							Description: "The number of subaccounts. NodePing reports this only for the primary account; " +
								"it is null for every subaccount.",
							Computed: true,
						},
					},
				},
			},
		},
	}
}

func (d *AccountsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *AccountsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	tflog.Debug(ctx, "Reading accounts data source")

	listing, err := d.client.ListAccounts(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Accounts",
			"Could not list accounts: "+err.Error(),
		)
		return
	}

	state := AccountsDataSourceModel{Accounts: mapAccounts(listing)}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// mapAccounts converts the API's account map into a list ordered by account ID.
// Ranging the map directly yields Go's randomized iteration order, which would
// make the data source produce a different result on every read.
func mapAccounts(listing map[string]client.AccountSummary) []AccountModel {
	ids := make([]string, 0, len(listing))
	for id := range listing {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	result := make([]AccountModel, 0, len(ids))
	for _, id := range ids {
		summary := listing[id]

		model := AccountModel{
			ID:              types.StringValue(id),
			Name:            types.StringValue(summary.Name),
			Status:          types.StringValue(summary.Status),
			ActiveChecks:    types.Int64Value(int64(summary.Count)),
			SubaccountCount: types.Int64Null(),
		}
		if summary.Parent != nil {
			model.SubaccountCount = types.Int64Value(int64(*summary.Parent))
		}

		result = append(result, model)
	}

	return result
}
