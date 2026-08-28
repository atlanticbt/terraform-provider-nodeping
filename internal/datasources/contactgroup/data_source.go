package contactgroup

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/client"
)

var _ datasource.DataSource = &ContactGroupDataSource{}
var _ datasource.DataSourceWithConfigure = &ContactGroupDataSource{}

type ContactGroupDataSource struct {
	client *client.Client
}

type ContactGroupDataSourceModel struct {
	ID         types.String `tfsdk:"id"`
	CustomerID types.String `tfsdk:"customer_id"`
	Name       types.String `tfsdk:"name"`
	Members    types.Set    `tfsdk:"members"`
}

func NewContactGroupDataSource() datasource.DataSource {
	return &ContactGroupDataSource{}
}

func (d *ContactGroupDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_contactgroup"
}

func (d *ContactGroupDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches a NodePing contact group by ID.",
		MarkdownDescription: `
Fetches a NodePing contact group by ID.

` + "`members`" + ` holds contact **address** IDs, not contact IDs. Each NodePing
contact owns one or more addresses, each with its own 8-character ID, and it is
the address that joins a group.

## Example Usage

` + "```hcl" + `
data "nodeping_contactgroup" "alerts" {
  id = "201205050153W2Q4C-G-ISYB7"
}

output "alert_group_name" {
  value = data.nodeping_contactgroup.alerts.name
}

# Address IDs, ready to reuse in another group.
output "alert_group_address_ids" {
  value = data.nodeping_contactgroup.alerts.members
}
` + "```" + `

For a group that belongs to a SubAccount, set ` + "`customer_id`" + `:

` + "```hcl" + `
data "nodeping_contactgroup" "sub" {
  id          = "201205050153W2Q4C-G-ISYB7"
  customer_id = "201205050153W2Q4C"
}
` + "```" + `
`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the contact group.",
				Required:    true,
			},
			"customer_id": schema.StringAttribute{
				Description: "The customer ID (SubAccount ID) that owns this contact group. " +
					"Set this to look up a group that belongs to a SubAccount.",
				Optional: true,
				Computed: true,
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
	}
}

func (d *ContactGroupDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *ContactGroupDataSource) clientFor(customerID types.String) *client.Client {
	if customerID.IsNull() || customerID.IsUnknown() || customerID.ValueString() == "" {
		return d.client
	}
	return d.client.WithCustomerID(customerID.ValueString())
}

func (d *ContactGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ContactGroupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading contact group data source", map[string]interface{}{
		"id": config.ID.ValueString(),
	})

	group, err := d.clientFor(config.CustomerID).GetContactGroup(ctx, config.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Contact Group",
			"Could not read contact group ID "+config.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	config.Name = types.StringValue(group.Name)
	if group.CustomerID != "" {
		config.CustomerID = types.StringValue(group.CustomerID)
	} else if config.CustomerID.IsNull() || config.CustomerID.IsUnknown() {
		config.CustomerID = types.StringNull()
	}
	config.Members = membersToSet(ctx, group.Members, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// membersToSet converts the API's member list into a set value. The slice is
// sorted first so the value is byte-for-byte stable between reads, and it is
// never nil so the Computed attribute always resolves to a known value.
func membersToSet(ctx context.Context, members []string, diags *diag.Diagnostics) types.Set {
	sorted := make([]string, 0, len(members))
	sorted = append(sorted, members...)
	sort.Strings(sorted)

	set, d := types.SetValueFrom(ctx, types.StringType, sorted)
	diags.Append(d...)
	if d.HasError() {
		return types.SetNull(types.StringType)
	}
	return set
}
