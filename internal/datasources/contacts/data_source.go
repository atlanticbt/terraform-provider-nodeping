package contacts

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/atlanticbt/terraform-provider-nodeping/internal/client"
)

var _ datasource.DataSource = &ContactsDataSource{}
var _ datasource.DataSourceWithConfigure = &ContactsDataSource{}

type ContactsDataSource struct {
	client *client.Client
}

type ContactsDataSourceModel struct {
	Contacts []ContactModel `tfsdk:"contacts"`
}

type ContactModel struct {
	ID         types.String   `tfsdk:"id"`
	CustomerID types.String   `tfsdk:"customer_id"`
	Name       types.String   `tfsdk:"name"`
	CustRole   types.String   `tfsdk:"custrole"`
	Addresses  []AddressModel `tfsdk:"addresses"`
}

type AddressModel struct {
	ID      types.String `tfsdk:"id"`
	Type    types.String `tfsdk:"type"`
	Address types.String `tfsdk:"address"`
}

func NewContactsDataSource() datasource.DataSource {
	return &ContactsDataSource{}
}

func (d *ContactsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_contacts"
}

func (d *ContactsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches all NodePing contacts.",
		MarkdownDescription: `
Fetches all NodePing contacts.

## Example Usage

` + "```hcl" + `
data "nodeping_contacts" "all" {}

output "contact_ids" {
  value = [for c in data.nodeping_contacts.all.contacts : c.id]
}
` + "```" + `
`,
		Attributes: map[string]schema.Attribute{
			"contacts": schema.ListNestedAttribute{
				Description: "List of contacts.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "The unique identifier of the contact.",
							Computed:    true,
						},
						"customer_id": schema.StringAttribute{
							Description: "The customer ID (account ID) that owns this contact.",
							Computed:    true,
						},
						"name": schema.StringAttribute{
							Description: "The name of the contact.",
							Computed:    true,
						},
						"custrole": schema.StringAttribute{
							Description: "The permission role for this contact.",
							Computed:    true,
						},
						"addresses": schema.ListNestedAttribute{
							Description: "Contact addresses.",
							Computed:    true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"id": schema.StringAttribute{
										Description: "The unique identifier of the address.",
										Computed:    true,
									},
									"type": schema.StringAttribute{
										Description: "The type of address.",
										Computed:    true,
									},
									"address": schema.StringAttribute{
										Description: "The address value.",
										Computed:    true,
										Sensitive:   true,
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

func (d *ContactsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ContactsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	tflog.Debug(ctx, "Reading contacts data source")

	contacts, err := d.client.ListContacts(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Contacts",
			"Could not list contacts: "+err.Error(),
		)
		return
	}

	var state ContactsDataSourceModel
	state.Contacts = mapContacts(contacts)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// mapContacts converts the API's contact map into a list ordered by contact ID,
// with each contact's addresses ordered by address ID. Ranging the maps directly
// yields Go's randomized iteration order, which makes the data source produce a
// different result on every read.
func mapContacts(contacts map[string]client.Contact) []ContactModel {
	keys := make([]string, 0, len(contacts))
	for key := range contacts {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	result := make([]ContactModel, 0, len(keys))
	for _, key := range keys {
		contact := contacts[key]

		id := contact.ID
		if id == "" {
			id = key
		}

		contactModel := ContactModel{
			ID:         types.StringValue(id),
			CustomerID: types.StringValue(contact.CustomerID),
			Name:       types.StringValue(contact.Name),
			CustRole:   types.StringValue(contact.CustRole),
		}

		addressIDs := make([]string, 0, len(contact.Addresses))
		for addrID := range contact.Addresses {
			addressIDs = append(addressIDs, addrID)
		}
		sort.Strings(addressIDs)

		contactModel.Addresses = make([]AddressModel, 0, len(addressIDs))
		for _, addrID := range addressIDs {
			addr := contact.Addresses[addrID]
			contactModel.Addresses = append(contactModel.Addresses, AddressModel{
				ID:      types.StringValue(addrID),
				Type:    types.StringValue(addr.Type),
				Address: types.StringValue(addr.Address),
			})
		}

		result = append(result, contactModel)
	}

	return result
}
