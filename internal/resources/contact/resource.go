package contact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/resources/identity"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/client"
)

var (
	_ resource.Resource                = &ContactResource{}
	_ resource.ResourceWithConfigure   = &ContactResource{}
	_ resource.ResourceWithImportState = &ContactResource{}
)

type ContactResource struct {
	client *client.Client
}

func NewContactResource() resource.Resource {
	return &ContactResource{}
}

func (r *ContactResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_contact"
}

func (r *ContactResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = ContactSchema()
}

func (r *ContactResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
// the resource, falling back to the provider-level client when the resource does
// not carry a customer_id.
func (r *ContactResource) scopedClient(customerID types.String) *client.Client {
	if customerID.IsNull() || customerID.IsUnknown() || customerID.ValueString() == "" {
		return r.client
	}
	return r.client.WithCustomerID(customerID.ValueString())
}

func (r *ContactResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ContactResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating contact", map[string]interface{}{
		"name": plan.Name.ValueString(),
	})

	createReq := client.ContactCreateRequest{
		Name:     plan.Name.ValueString(),
		CustRole: plan.CustRole.ValueString(),
	}

	for _, addr := range plan.Addresses {
		createReq.NewAddresses = append(createReq.NewAddresses, newAddressFromModel(ctx, addr, &resp.Diagnostics))
		if resp.Diagnostics.HasError() {
			return
		}
	}

	contact, err := r.scopedClient(plan.CustomerID).CreateContact(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Contact",
			"Could not create contact: "+err.Error(),
		)
		return
	}

	plan.ID = types.StringValue(contact.ID)
	if plan.CustomerID.IsNull() || plan.CustomerID.IsUnknown() {
		plan.CustomerID = types.StringValue(contact.CustomerID)
	}

	plan.Addresses = mapAddressesToModel(ctx, contact.Addresses, plan.Addresses, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Created contact", map[string]interface{}{
		"id": contact.ID,
	})

	identity.Set(ctx, resp.Identity, plan.ID, plan.CustomerID, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ContactResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ContactResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading contact", map[string]interface{}{
		"id": state.ID.ValueString(),
	})

	contact, err := r.scopedClient(state.CustomerID).GetContact(ctx, state.ID.ValueString())
	if err != nil {
		var notFound *client.NotFoundError
		if errors.As(err, &notFound) {
			tflog.Debug(ctx, "Contact not found, removing from state", map[string]interface{}{
				"id": state.ID.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading Contact",
			"Could not read contact ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	state.ID = types.StringValue(contact.ID)
	if contact.CustomerID != "" {
		state.CustomerID = types.StringValue(contact.CustomerID)
	}
	state.Name = contactName(contact.Name)
	state.CustRole = types.StringValue(contact.CustRole)

	state.Addresses = mapAddressesToModel(ctx, contact.Addresses, state.Addresses, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	identity.Set(ctx, resp.Identity, state.ID, state.CustomerID, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ContactResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ContactResourceModel
	var state ContactResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating contact", map[string]interface{}{
		"id": state.ID.ValueString(),
	})

	updateReq := client.ContactUpdateRequest{
		Name:     plan.Name.ValueString(),
		CustRole: plan.CustRole.ValueString(),
	}

	// Resolve the NodePing address ID for every planned address by matching on
	// content (type + address), never by list position. Matching by position is
	// data destructive: it lets an edit to one address overwrite a different
	// address in NodePing.
	addressIDs := matchAddresses(plan.Addresses, candidatesFromModels(state.Addresses))

	updateReq.Addresses = make(map[string]client.ContactAddress)
	for i, addr := range plan.Addresses {
		if id := addressIDs[i]; id != "" {
			updateReq.Addresses[id] = contactAddressFromModel(ctx, addr, &resp.Diagnostics)
		} else {
			updateReq.NewAddresses = append(updateReq.NewAddresses, newAddressFromModel(ctx, addr, &resp.Diagnostics))
		}
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// NodePing rejects an update that would leave a contact with no addresses
	// at all, with "Account must have at least one 'owner' contact". Left to
	// the API the request is refused and the addresses stay put, so say what
	// happened here instead of letting the practitioner read a message about
	// account owners.
	if len(plan.Addresses) == 0 {
		if len(state.Addresses) > 0 {
			resp.Diagnostics.AddError(
				"Cannot Remove Every Contact Address",
				"NodePing refuses an update that leaves contact ID "+state.ID.ValueString()+
					" with no addresses. Keep at least one address, or destroy the contact.",
			)
			return
		}
		// A contact that had no addresses and is gaining none must not send
		// `addresses` at all; nil marshals to null, which NodePing ignores.
		updateReq.Addresses = nil
	}

	contact, err := r.scopedClient(state.CustomerID).UpdateContact(ctx, state.ID.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating Contact",
			"Could not update contact ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	plan.ID = types.StringValue(contact.ID)
	if plan.CustomerID.IsNull() || plan.CustomerID.IsUnknown() {
		plan.CustomerID = state.CustomerID
	}

	plan.Addresses = mapAddressesToModel(ctx, contact.Addresses, plan.Addresses, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updated contact", map[string]interface{}{
		"id": contact.ID,
	})

	// Update writes the identity too. Both members are immutable, so this
	// re-states the same values -- but on a resource created before the core
	// supported identity there is no stored identity yet, and the framework
	// rejects an update that leaves it null.
	identity.Set(ctx, resp.Identity, plan.ID, plan.CustomerID, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ContactResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ContactResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting contact", map[string]interface{}{
		"id": state.ID.ValueString(),
	})

	err := r.scopedClient(state.CustomerID).DeleteContact(ctx, state.ID.ValueString())
	if err != nil {
		var notFound *client.NotFoundError
		if errors.As(err, &notFound) {
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting Contact",
			"Could not delete contact ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	tflog.Debug(ctx, "Deleted contact", map[string]interface{}{
		"id": state.ID.ValueString(),
	})
}

func (r *ContactResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// An "identity" import block is handled first; anything else is the
	// established string form, which must keep working unchanged.
	contactID, customerID, fromIdentity := identity.FromImport(ctx, req, &resp.Diagnostics, "contact")
	if resp.Diagnostics.HasError() {
		return
	}
	if !fromIdentity {

		idParts := strings.Split(req.ID, ":")

		if len(idParts) == 2 {
			customerID = idParts[0]
			contactID = idParts[1]
		} else if len(idParts) == 1 {
			contactID = idParts[0]
		} else {
			resp.Diagnostics.AddError(
				"Invalid Import ID",
				fmt.Sprintf("Expected import ID in format 'contact_id' or 'customer_id:contact_id', got: %s", req.ID),
			)
			return
		}

		if contactID == "" {
			resp.Diagnostics.AddError(
				"Invalid Import ID",
				fmt.Sprintf("Expected import ID in format 'contact_id' or 'customer_id:contact_id', got: %s", req.ID),
			)
			return
		}
	}

	tflog.Debug(ctx, "Importing contact", map[string]interface{}{
		"contact_id":  contactID,
		"customer_id": customerID,
	})

	customerIDValue := types.StringNull()
	if customerID != "" {
		customerIDValue = types.StringValue(customerID)
	}

	contact, err := r.scopedClient(customerIDValue).GetContact(ctx, contactID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Importing Contact",
			"Could not import contact: "+err.Error(),
		)
		return
	}

	// The customer_id must be persisted, otherwise every subsequent API call for
	// this resource targets the root account and the contact silently disappears
	// from state on the next refresh.
	if customerID == "" && contact.CustomerID != "" {
		customerIDValue = types.StringValue(contact.CustomerID)
	}

	state := ContactResourceModel{
		ID:         types.StringValue(contact.ID),
		CustomerID: customerIDValue,
		Name:       contactName(contact.Name),
		CustRole:   types.StringValue(contact.CustRole),
	}

	state.Addresses = mapAddressesToModel(ctx, contact.Addresses, nil, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	identity.Set(ctx, resp.Identity, state.ID, state.CustomerID, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// contactName maps the API's name field onto the optional `name` attribute.
//
// A contact created without a name comes back with no `name` key at all, and
// writing the resulting empty string into state made every refresh plan an
// update from null to "" that no apply could settle. `name` is Optional and not
// Computed, so absent has to stay null.
func contactName(name string) types.String {
	if name == "" {
		return types.StringNull()
	}
	return types.StringValue(name)
}

// newAddressFromModel converts a planned address block into a NodePing
// "newaddresses" entry.
func newAddressFromModel(ctx context.Context, addr AddressModel, diags *diag.Diagnostics) client.NewAddress {
	// Existing owner and login addresses have no type, so the attribute is
	// Optional, but NodePing cannot create an address without one.
	if addr.Type.IsNull() || addr.Type.ValueString() == "" {
		diags.AddError(
			"Missing Address Type",
			fmt.Sprintf("The new address %q has no type. Set type on every address block that NodePing has not created yet.", addr.Address.ValueString()),
		)
	}
	newAddr := client.NewAddress{
		Address:       addr.Address.ValueString(),
		Type:          addr.Type.ValueString(),
		SuppressUp:    addr.SuppressUp.ValueBool(),
		SuppressDown:  addr.SuppressDown.ValueBool(),
		SuppressFirst: addr.SuppressFirst.ValueBool(),
		SuppressDiag:  addr.SuppressDiag.ValueBool(),
		SuppressAll:   addr.SuppressAll.ValueBool(),
		Mute:          addr.Mute.ValueBool(),
	}

	if !addr.Action.IsNull() {
		newAddr.Action = addr.Action.ValueString()
	}
	if !addr.Data.IsNull() {
		newAddr.Data = addr.Data.ValueString()
	}
	if !addr.Priority.IsNull() {
		priority := int(addr.Priority.ValueInt64())
		newAddr.Priority = &priority
	}
	newAddr.Headers = stringMapFromModel(ctx, addr.Headers, diags)
	newAddr.QueryStrings = stringMapFromModel(ctx, addr.QueryStrings, diags)

	return newAddr
}

// contactAddressFromModel converts a planned address block into a NodePing
// "addresses" entry (an update of an existing address).
func contactAddressFromModel(ctx context.Context, addr AddressModel, diags *diag.Diagnostics) client.ContactAddress {
	addrUpdate := client.ContactAddress{
		Address:       addr.Address.ValueString(),
		Type:          addr.Type.ValueString(),
		SuppressUp:    addr.SuppressUp.ValueBool(),
		SuppressDown:  addr.SuppressDown.ValueBool(),
		SuppressFirst: addr.SuppressFirst.ValueBool(),
		SuppressDiag:  addr.SuppressDiag.ValueBool(),
		SuppressAll:   addr.SuppressAll.ValueBool(),
	}

	// Always send an explicit boolean. Omitting `mute` when it is false means an
	// un-mute never reaches the API and drifts forever.
	if addr.Mute.ValueBool() {
		addrUpdate.Mute = json.RawMessage("true")
	} else {
		addrUpdate.Mute = json.RawMessage("false")
	}

	if !addr.Action.IsNull() {
		addrUpdate.Action = addr.Action.ValueString()
	}
	if !addr.Data.IsNull() {
		addrUpdate.Data = addr.Data.ValueString()
	}
	if !addr.Priority.IsNull() {
		priority := int(addr.Priority.ValueInt64())
		addrUpdate.Priority = &priority
	}
	addrUpdate.Headers = stringMapFromModel(ctx, addr.Headers, diags)
	addrUpdate.QueryStrings = stringMapFromModel(ctx, addr.QueryStrings, diags)

	return addrUpdate
}

func stringMapFromModel(ctx context.Context, m types.Map, diags *diag.Diagnostics) map[string]string {
	if m.IsNull() || m.IsUnknown() {
		return nil
	}
	out := make(map[string]string, len(m.Elements()))
	diags.Append(m.ElementsAs(ctx, &out, false)...)
	return out
}

// addressCandidate is an existing address (in state or returned by the API)
// that a planned address block can be matched against.
type addressCandidate struct {
	ID      string
	Type    string
	Address string
}

func candidatesFromModels(addresses []AddressModel) []addressCandidate {
	candidates := make([]addressCandidate, 0, len(addresses))
	for _, addr := range addresses {
		if addr.ID.IsNull() || addr.ID.IsUnknown() || addr.ID.ValueString() == "" {
			continue
		}
		candidates = append(candidates, addressCandidate{
			ID:      addr.ID.ValueString(),
			Type:    addr.Type.ValueString(),
			Address: addr.Address.ValueString(),
		})
	}
	return candidates
}

// candidatesFromAPI flattens the API's address map into a slice ordered by
// address ID so that every derived ordering is deterministic regardless of Go's
// randomized map iteration order.
func candidatesFromAPI(apiAddresses map[string]client.ContactAddress) []addressCandidate {
	ids := make([]string, 0, len(apiAddresses))
	for id := range apiAddresses {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	candidates := make([]addressCandidate, 0, len(ids))
	for _, id := range ids {
		candidates = append(candidates, addressCandidate{
			ID:      id,
			Type:    apiAddresses[id].Type,
			Address: apiAddresses[id].Address,
		})
	}
	return candidates
}

func addressKey(typ, address string) string {
	return typ + ":" + address
}

// matchAddresses pairs each planned address with an existing address and returns
// the matched ID per plan index ("" when no match, i.e. the address is new).
//
// Matching is done in three passes, strongest signal first:
//  1. content (type + address) — an address that did not change keeps its ID
//  2. explicit ID — a planned ID that still exists among the candidates
//  3. positional fallback restricted to the same type — this recovers the ID of
//     an address whose value was edited in place
//
// Each candidate is consumed at most once.
func matchAddresses(plan []AddressModel, candidates []addressCandidate) []string {
	result := make([]string, len(plan))
	used := make([]bool, len(candidates))

	byKey := make(map[string][]int, len(candidates))
	byID := make(map[string]int, len(candidates))
	for i, c := range candidates {
		byKey[addressKey(c.Type, c.Address)] = append(byKey[addressKey(c.Type, c.Address)], i)
		byID[c.ID] = i
	}

	// Pass 1: exact content match.
	for i, addr := range plan {
		if addr.Type.IsUnknown() || addr.Address.IsUnknown() {
			continue
		}
		key := addressKey(addr.Type.ValueString(), addr.Address.ValueString())
		for _, ci := range byKey[key] {
			if used[ci] {
				continue
			}
			used[ci] = true
			result[i] = candidates[ci].ID
			break
		}
	}

	// Pass 2: planned ID still present among the candidates.
	for i, addr := range plan {
		if result[i] != "" {
			continue
		}
		if addr.ID.IsNull() || addr.ID.IsUnknown() || addr.ID.ValueString() == "" {
			continue
		}
		ci, ok := byID[addr.ID.ValueString()]
		if !ok || used[ci] {
			continue
		}
		used[ci] = true
		result[i] = candidates[ci].ID
	}

	// Pass 3: in-place edits — pair the remaining plan entries with the remaining
	// candidates of the same type, in order.
	for i, addr := range plan {
		if result[i] != "" {
			continue
		}
		if addr.Type.IsUnknown() {
			continue
		}
		for ci, c := range candidates {
			if used[ci] || c.Type != addr.Type.ValueString() {
				continue
			}
			used[ci] = true
			result[i] = c.ID
			break
		}
	}

	return result
}

// mapAddressesToModel converts the API's address map into the ordered list the
// schema requires. The result follows the order of planAddresses (the plan or
// prior state); addresses that only exist in the API are appended afterwards
// sorted by ID. Ranging the API map directly would produce a randomized order
// and therefore random "inconsistent result after apply" failures.
func mapAddressesToModel(ctx context.Context, apiAddresses map[string]client.ContactAddress, planAddresses []AddressModel, diags *diag.Diagnostics) []AddressModel {
	if len(apiAddresses) == 0 {
		return nil
	}

	candidates := candidatesFromAPI(apiAddresses)
	matched := matchAddresses(planAddresses, candidates)

	result := make([]AddressModel, 0, len(apiAddresses))
	emitted := make(map[string]bool, len(apiAddresses))

	// matched has one entry per planned address, so ranging over the plan keeps
	// both indexes in bounds.
	for i := range planAddresses {
		id := matched[i]
		if id == "" || emitted[id] {
			continue
		}
		emitted[id] = true
		result = append(result, addressToModel(ctx, id, apiAddresses[id], &planAddresses[i], diags))
	}

	for _, c := range candidates {
		if emitted[c.ID] {
			continue
		}
		emitted[c.ID] = true
		result = append(result, addressToModel(ctx, c.ID, apiAddresses[c.ID], nil, diags))
	}

	return result
}

// addressToModel converts a single API address into its model representation.
// When planned is non-nil, values that are semantically equal to the planned
// value are preserved verbatim so that Terraform's consistency check passes and
// no phantom diff is produced.
func addressToModel(ctx context.Context, id string, addr client.ContactAddress, planned *AddressModel, diags *diag.Diagnostics) AddressModel {
	model := AddressModel{
		ID:            types.StringValue(id),
		Type:          stringOrNull(addr.Type),
		Address:       types.StringValue(addr.Address),
		SuppressUp:    types.BoolValue(addr.SuppressUp),
		SuppressDown:  types.BoolValue(addr.SuppressDown),
		SuppressFirst: types.BoolValue(addr.SuppressFirst),
		SuppressDiag:  types.BoolValue(addr.SuppressDiag),
		SuppressAll:   types.BoolValue(addr.SuppressAll),
		Mute:          types.BoolValue(false),
	}

	if addr.Mute != nil {
		var muteVal interface{}
		if err := json.Unmarshal(addr.Mute, &muteVal); err == nil {
			switch v := muteVal.(type) {
			case bool:
				model.Mute = types.BoolValue(v)
			case float64:
				model.Mute = types.BoolValue(v > 0)
			}
		}
	}

	if addr.Action != "" {
		model.Action = types.StringValue(addr.Action)
	} else {
		model.Action = types.StringNull()
	}

	model.Data = dataToModel(addr.Data, planned)

	if addr.Priority != nil {
		model.Priority = types.Int64Value(int64(*addr.Priority))
	} else {
		model.Priority = types.Int64Null()
	}

	if len(addr.Headers) > 0 {
		headers, d := types.MapValueFrom(ctx, types.StringType, addr.Headers)
		diags.Append(d...)
		model.Headers = headers
	} else {
		model.Headers = types.MapNull(types.StringType)
	}

	if len(addr.QueryStrings) > 0 {
		qs, d := types.MapValueFrom(ctx, types.StringType, addr.QueryStrings)
		diags.Append(d...)
		model.QueryStrings = qs
	} else {
		model.QueryStrings = types.MapNull(types.StringType)
	}

	return model
}

// dataToModel renders the API's `data` value as a string, preserving the exact
// string the practitioner configured whenever it is semantically equivalent.
// The API returns `data` either as the original string or as a decoded object;
// re-encoding it unconditionally would rewrite the practitioner's formatting and
// fail the apply consistency check.
func dataToModel(apiData interface{}, planned *AddressModel) JSONString {
	var apiString string
	var hasAPIValue bool

	switch v := apiData.(type) {
	case nil:
		hasAPIValue = false
	case string:
		if v != "" {
			apiString = v
			hasAPIValue = true
		}
	default:
		if jsonBytes, err := json.Marshal(v); err == nil {
			apiString = string(jsonBytes)
			hasAPIValue = true
		}
	}

	if planned != nil && !planned.Data.IsNull() && !planned.Data.IsUnknown() {
		plannedString := planned.Data.ValueString()
		if !hasAPIValue || jsonSemanticEqual(plannedString, apiString) {
			return NewJSONStringValue(plannedString)
		}
	}

	if !hasAPIValue {
		return NewJSONStringNull()
	}
	return NewJSONStringValue(apiString)
}

// addressIDPlanModifier fixes up the `id` of each planned address block.
//
// Terraform core proposes computed attribute values for list elements *by
// position*, so inserting, removing or reordering an address block makes a
// planned address inherit an unrelated address's ID. This modifier re-derives
// each ID by matching on content, and marks genuinely new addresses unknown.
type addressIDPlanModifier struct{}

// AddressIDPlanModifier returns the plan modifier that keeps address IDs paired
// with the right address block.
func AddressIDPlanModifier() planmodifier.List {
	return addressIDPlanModifier{}
}

func (m addressIDPlanModifier) Description(_ context.Context) string {
	return "Matches address IDs to address blocks by content instead of list position"
}

func (m addressIDPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m addressIDPlanModifier) PlanModifyList(ctx context.Context, req planmodifier.ListRequest, resp *planmodifier.ListResponse) {
	// Destroy plan, or nothing planned yet.
	if req.PlanValue.IsNull() || req.PlanValue.IsUnknown() {
		return
	}

	var planAddresses []AddressModel
	resp.Diagnostics.Append(req.PlanValue.ElementsAs(ctx, &planAddresses, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(planAddresses) == 0 {
		return
	}

	var stateAddresses []AddressModel
	if !req.StateValue.IsNull() && !req.StateValue.IsUnknown() {
		resp.Diagnostics.Append(req.StateValue.ElementsAs(ctx, &stateAddresses, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	matched := matchAddresses(planAddresses, candidatesFromModels(stateAddresses))
	for i, id := range matched {
		if id == "" {
			planAddresses[i].ID = types.StringUnknown()
		} else {
			planAddresses[i].ID = types.StringValue(id)
		}
	}

	list, d := types.ListValueFrom(ctx, req.PlanValue.ElementType(ctx), planAddresses)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.PlanValue = list
}

func stringOrNull(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}
