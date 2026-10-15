package contactgroups

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/atlanticbt/terraform-provider-nodeping/internal/client"
)

var _ datasource.DataSource = &ContactGroupsDataSource{}
var _ datasource.DataSourceWithConfigure = &ContactGroupsDataSource{}

type ContactGroupsDataSource struct {
	client *client.Client
}

type ContactGroupsDataSourceModel struct {
	CustomerID    types.String        `tfsdk:"customer_id"`
	ContactGroups []ContactGroupModel `tfsdk:"contactgroups"`
}

type ContactGroupModel struct {
	ID         types.String `tfsdk:"id"`
	CustomerID types.String `tfsdk:"customer_id"`
	Name       types.String `tfsdk:"name"`
	Members    types.Set    `tfsdk:"members"`
}

func NewContactGroupsDataSource() datasource.DataSource {
	return &ContactGroupsDataSource{}
}

func (d *ContactGroupsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_contactgroups"
}

func (d *ContactGroupsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches all NodePing contact groups.",
		MarkdownDescription: `
Fetches all NodePing contact groups, ordered by group ID.

Each group's ` + "`members`" + ` list holds contact **address** IDs, not contact IDs.

## Example Usage

` + "```hcl" + `
data "nodeping_contactgroups" "all" {}

output "group_ids" {
  value = [for g in data.nodeping_contactgroups.all.contactgroups : g.id]
}

# Look a group up by name.
locals {
  prod_group = one([
    for g in data.nodeping_contactgroups.all.contactgroups : g
    if g.name == "Production Alerts"
  ])
}
` + "```" + `

To list the groups belonging to a SubAccount:

` + "```hcl" + `
data "nodeping_contactgroups" "sub" {
  customer_id = "201205050153W2Q4C"
}
` + "```" + `
`,
		Attributes: map[string]schema.Attribute{
			"customer_id": schema.StringAttribute{
				Description: "The customer ID (SubAccount ID) to list contact groups for. " +
					"Omit to list the groups of the account the provider is configured with.",
				Optional: true,
			},
			"contactgroups": schema.ListNestedAttribute{
				Description: "List of contact groups, ordered by group ID.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "The unique identifier of the contact group.",
							Computed:    true,
						},
						"customer_id": schema.StringAttribute{
							Description: "The customer ID (account ID) that owns this contact group.",
							Computed:    true,
						},
						"name": schema.StringAttribute{
							Description: "The name of the contact group.",
							Computed:    true,
						},
						"members": schema.SetAttribute{
							Description: "Contact address IDs that belong to this group. These are NOT contact IDs.",
							Computed:    true,
							ElementType: types.StringType,
						},
					},
				},
			},
		},
	}
}

func (d *ContactGroupsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

// clientFor returns a client scoped to the configured SubAccount, or the
// provider-level client when no customer_id was supplied.
func (d *ContactGroupsDataSource) clientFor(customerID types.String) *client.Client {
	if customerID.IsNull() || customerID.IsUnknown() || customerID.ValueString() == "" {
		return d.client
	}
	return d.client.WithCustomerID(customerID.ValueString())
}

func (d *ContactGroupsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ContactGroupsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading contact groups data source")

	groups, err := d.clientFor(config.CustomerID).ListContactGroups(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Contact Groups",
			"Could not list contact groups: "+err.Error(),
		)
		return
	}

	config.ContactGroups = mapContactGroups(ctx, groups, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// mapContactGroups converts the API's group map into a list ordered by group
// ID, with each group's members sorted. Ranging the map directly yields Go's
// randomized iteration order, which makes the data source produce a different
// result on every read.
func mapContactGroups(ctx context.Context, groups map[string]client.ContactGroup, diags *diag.Diagnostics) []ContactGroupModel {
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	result := make([]ContactGroupModel, 0, len(keys))
	for _, key := range keys {
		group := groups[key]

		// The list endpoint keys each entry by its group ID; older responses
		// have been seen without the redundant _id field in the body.
		id := group.ID
		if id == "" {
			id = key
		}

		members := make([]string, 0, len(group.Members))
		members = append(members, group.Members...)
		sort.Strings(members)

		set, d := types.SetValueFrom(ctx, types.StringType, members)
		diags.Append(d...)
		if d.HasError() {
			return nil
		}

		result = append(result, ContactGroupModel{
			ID:         types.StringValue(id),
			CustomerID: types.StringValue(group.CustomerID),
			Name:       types.StringValue(group.Name),
			Members:    set,
		})
	}

	return result
}
