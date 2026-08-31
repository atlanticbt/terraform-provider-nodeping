package contactgroup

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/client"
)

var (
	_ resource.Resource                = &ContactGroupResource{}
	_ resource.ResourceWithConfigure   = &ContactGroupResource{}
	_ resource.ResourceWithImportState = &ContactGroupResource{}
)

type ContactGroupResource struct {
	client *client.Client
}

func NewContactGroupResource() resource.Resource {
	return &ContactGroupResource{}
}

func (r *ContactGroupResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_contactgroup"
}

func (r *ContactGroupResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = ContactGroupSchema()
}

func (r *ContactGroupResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = c
}

// scopedClient returns a client scoped to the customer (SubAccount) recorded on
// the resource, falling back to the provider-level client when the resource
// does not carry a customer_id.
func (r *ContactGroupResource) scopedClient(customerID types.String) *client.Client {
	if customerID.IsNull() || customerID.IsUnknown() || customerID.ValueString() == "" {
		return r.client
	}
	return r.client.WithCustomerID(customerID.ValueString())
}

func (r *ContactGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ContactGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	members := membersFromSet(ctx, plan.Members, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating contact group", map[string]interface{}{
		"name":         plan.Name.ValueString(),
		"member_count": len(members),
	})

	group, err := r.scopedClient(plan.CustomerID).CreateContactGroup(ctx, client.ContactGroupCreateRequest{
		Name:    plan.Name.ValueString(),
		Members: members,
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Contact Group",
			"Could not create contact group: "+err.Error(),
		)
		return
	}

	applyGroupToModel(ctx, group, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Created contact group", map[string]interface{}{
		"id": group.ID,
	})

	setIdentity(ctx, resp.Identity, plan.ID, plan.CustomerID, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ContactGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ContactGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading contact group", map[string]interface{}{
		"id": state.ID.ValueString(),
	})

	group, err := r.scopedClient(state.CustomerID).GetContactGroup(ctx, state.ID.ValueString())
	if err != nil {
		var notFound *client.NotFoundError
		if errors.As(err, &notFound) {
			tflog.Debug(ctx, "Contact group not found, removing from state", map[string]interface{}{
				"id": state.ID.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading Contact Group",
			"Could not read contact group ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	applyGroupToModel(ctx, group, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	setIdentity(ctx, resp.Identity, state.ID, state.CustomerID, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ContactGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ContactGroupResourceModel
	var state ContactGroupResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	members := membersFromSet(ctx, plan.Members, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating contact group", map[string]interface{}{
		"id":           state.ID.ValueString(),
		"member_count": len(members),
	})

	group, err := r.scopedClient(state.CustomerID).UpdateContactGroup(ctx, state.ID.ValueString(), client.ContactGroupUpdateRequest{
		ID:      state.ID.ValueString(),
		Name:    plan.Name.ValueString(),
		Members: members,
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating Contact Group",
			"Could not update contact group ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	if plan.CustomerID.IsNull() || plan.CustomerID.IsUnknown() {
		plan.CustomerID = state.CustomerID
	}

	applyGroupToModel(ctx, group, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updated contact group", map[string]interface{}{
		"id": group.ID,
	})

	// Update writes the identity too. Both members are immutable, so this
	// re-states the same values -- but on a resource created before the core
	// supported identity there is no stored identity yet, and the framework
	// rejects an update that leaves it null.
	setIdentity(ctx, resp.Identity, plan.ID, plan.CustomerID, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ContactGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ContactGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting contact group", map[string]interface{}{
		"id": state.ID.ValueString(),
	})

	err := r.scopedClient(state.CustomerID).DeleteContactGroup(ctx, state.ID.ValueString())
	if err != nil {
		var notFound *client.NotFoundError
		if errors.As(err, &notFound) {
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting Contact Group",
			"Could not delete contact group ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	tflog.Debug(ctx, "Deleted contact group", map[string]interface{}{
		"id": state.ID.ValueString(),
	})
}

func (r *ContactGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	var groupID string
	var customerID string

	// An "identity" import block is handled first; anything else is the
	// established string form, which must keep working unchanged.
	if idFromIdentity, customerFromIdentity, fromIdentity := identityImportValues(ctx, req, &resp.Diagnostics); fromIdentity {
		if resp.Diagnostics.HasError() {
			return
		}
		if idFromIdentity == "" {
			resp.Diagnostics.AddError(
				"Invalid Import Identity",
				"The 'id' attribute of the import identity must be a non-empty contact group ID.",
			)
			return
		}
		groupID = idFromIdentity
		customerID = customerFromIdentity
	} else {
		if resp.Diagnostics.HasError() {
			return
		}

		var ok bool
		groupID, customerID, ok = parseImportID(req.ID)
		if !ok {
			resp.Diagnostics.AddError(
				"Invalid Import ID",
				fmt.Sprintf("Expected import ID in format 'contactgroup_id' or 'customer_id:contactgroup_id', got: %s", req.ID),
			)
			return
		}
	}

	tflog.Debug(ctx, "Importing contact group", map[string]interface{}{
		"contactgroup_id": groupID,
		"customer_id":     customerID,
	})

	customerIDValue := types.StringNull()
	if customerID != "" {
		customerIDValue = types.StringValue(customerID)
	}

	group, err := r.scopedClient(customerIDValue).GetContactGroup(ctx, groupID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Importing Contact Group",
			"Could not import contact group: "+err.Error(),
		)
		return
	}

	// The customer_id must be persisted, otherwise every subsequent API call for
	// this resource targets the root account and the group silently disappears
	// from state on the next refresh.
	state := ContactGroupResourceModel{CustomerID: customerIDValue}

	applyGroupToModel(ctx, group, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	setIdentity(ctx, resp.Identity, state.ID, state.CustomerID, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// parseImportID splits an import ID into its group and customer components.
// Both "GROUPID" and "CUSTOMERID:GROUPID" are accepted.
func parseImportID(raw string) (groupID, customerID string, ok bool) {
	parts := strings.Split(raw, ":")

	switch len(parts) {
	case 1:
		groupID = parts[0]
	case 2:
		customerID = parts[0]
		groupID = parts[1]
	default:
		return "", "", false
	}

	if strings.TrimSpace(groupID) == "" {
		return "", "", false
	}

	return groupID, customerID, true
}

// applyGroupToModel writes an API response onto a model, leaving customer_id
// alone when the response does not carry one so a configured SubAccount scope
// is never lost.
func applyGroupToModel(ctx context.Context, group *client.ContactGroup, model *ContactGroupResourceModel, diags *diag.Diagnostics) {
	model.ID = types.StringValue(group.ID)
	model.Name = types.StringValue(group.Name)

	// Only fill customer_id in when the model does not already carry one. An
	// explicitly configured SubAccount scope must survive verbatim, otherwise
	// Terraform reports an inconsistent result after apply.
	if model.CustomerID.IsNull() || model.CustomerID.IsUnknown() {
		if group.CustomerID != "" {
			model.CustomerID = types.StringValue(group.CustomerID)
		} else {
			model.CustomerID = types.StringNull()
		}
	}

	model.Members = membersToSet(ctx, group.Members, diags)
}

// membersFromSet flattens a planned member set into the sorted slice sent to
// the API. Sorting keeps request bodies stable across plans; the set itself
// carries no order.
func membersFromSet(ctx context.Context, set types.Set, diags *diag.Diagnostics) []string {
	if set.IsNull() || set.IsUnknown() {
		return []string{}
	}

	members := make([]string, 0, len(set.Elements()))
	diags.Append(set.ElementsAs(ctx, &members, false)...)
	if diags.HasError() {
		return []string{}
	}

	sort.Strings(members)
	return members
}

// membersToSet converts an API member list into the schema's set value. A group
// with no members is an empty set, never null, so that a Computed value is
// always known after apply.
func membersToSet(ctx context.Context, members []string, diags *diag.Diagnostics) types.Set {
	// The slice must be non-nil: a nil slice converts to a null set, which is an
	// illegal value for a Computed attribute after apply.
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
