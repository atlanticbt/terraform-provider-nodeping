package notificationprofile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/atlanticbt/terraform-provider-nodeping/internal/resources/identity"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/atlanticbt/terraform-provider-nodeping/internal/client"
)

var (
	_ resource.Resource                = &NotificationProfileResource{}
	_ resource.ResourceWithConfigure   = &NotificationProfileResource{}
	_ resource.ResourceWithImportState = &NotificationProfileResource{}
)

// defaultSchedule is what NodePing falls back to when a notification carries no
// schedule of its own.
const defaultSchedule = "All"

type NotificationProfileResource struct {
	client *client.Client
}

func NewNotificationProfileResource() resource.Resource {
	return &NotificationProfileResource{}
}

func (r *NotificationProfileResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notificationprofile"
}

func (r *NotificationProfileResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = NotificationProfileSchema()
}

func (r *NotificationProfileResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// clientFor returns a client scoped to the subaccount recorded on the resource,
// falling back to the provider-level client when the resource carries no
// customer_id. Every CRUD path must go through it: a request that forgets the
// scope silently targets the root account, where the profile does not exist.
func (r *NotificationProfileResource) clientFor(customerID types.String) *client.Client {
	if !isSet(customerID) || customerID.ValueString() == "" {
		return r.client
	}
	return r.client.WithCustomerID(customerID.ValueString())
}

func (r *NotificationProfileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan NotificationProfileResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating notification profile", map[string]interface{}{
		"name": plan.Name.ValueString(),
	})

	profile, err := r.clientFor(plan.CustomerID).CreateNotificationProfile(ctx, client.NotificationProfileCreateRequest{
		Name:          plan.Name.ValueString(),
		Notifications: notificationsToAPI(plan.Notifications),
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Notification Profile",
			"Could not create notification profile: "+err.Error(),
		)
		return
	}

	state := mapForApply(profile, &plan)

	tflog.Debug(ctx, "Created notification profile", map[string]interface{}{
		"id": state.ID.ValueString(),
	})

	identity.Set(ctx, resp.Identity, state.ID, state.CustomerID, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *NotificationProfileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state NotificationProfileResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading notification profile", map[string]interface{}{
		"id": state.ID.ValueString(),
	})

	profile, err := r.clientFor(state.CustomerID).GetNotificationProfile(ctx, state.ID.ValueString())
	if err != nil {
		var notFound *client.NotFoundError
		if errors.As(err, &notFound) {
			tflog.Debug(ctx, "Notification profile not found, removing from state", map[string]interface{}{
				"id": state.ID.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading Notification Profile",
			"Could not read notification profile ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	refreshed := mapForRead(profile, &state)

	identity.Set(ctx, resp.Identity, refreshed.ID, refreshed.CustomerID, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, refreshed)...)
}

func (r *NotificationProfileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan NotificationProfileResourceModel
	var state NotificationProfileResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating notification profile", map[string]interface{}{
		"id": state.ID.ValueString(),
	})

	// NodePing replaces the notifications array wholesale, so the complete
	// planned list is sent on every update, never a delta.
	profile, err := r.clientFor(state.CustomerID).UpdateNotificationProfile(ctx, state.ID.ValueString(), client.NotificationProfileUpdateRequest{
		ID:            state.ID.ValueString(),
		Name:          plan.Name.ValueString(),
		Notifications: notificationsToAPI(plan.Notifications),
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating Notification Profile",
			"Could not update notification profile ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	updated := mapForApply(profile, &plan)
	if !isSet(updated.ID) {
		updated.ID = state.ID
	}
	if !isSet(updated.CustomerID) {
		updated.CustomerID = state.CustomerID
	}

	tflog.Debug(ctx, "Updated notification profile", map[string]interface{}{
		"id": updated.ID.ValueString(),
	})

	// Update writes the identity too. Both members are immutable, so this
	// re-states the same values -- but on a resource created before the core
	// supported identity there is no stored identity yet, and the framework
	// rejects an update that leaves it null.
	identity.Set(ctx, resp.Identity, updated.ID, updated.CustomerID, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, updated)...)
}

func (r *NotificationProfileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state NotificationProfileResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting notification profile", map[string]interface{}{
		"id": state.ID.ValueString(),
	})

	err := r.clientFor(state.CustomerID).DeleteNotificationProfile(ctx, state.ID.ValueString())
	if err != nil {
		var notFound *client.NotFoundError
		if errors.As(err, &notFound) {
			// Already gone: deletion is idempotent.
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting Notification Profile",
			"Could not delete notification profile ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	tflog.Debug(ctx, "Deleted notification profile", map[string]interface{}{
		"id": state.ID.ValueString(),
	})
}

func (r *NotificationProfileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// An "identity" import block is handled first; anything else is the
	// established string form, which must keep working unchanged.
	profileID, customerID, fromIdentity := identity.FromImport(ctx, req, &resp.Diagnostics, "notification profile")
	if resp.Diagnostics.HasError() {
		return
	}
	// FromImport rejects an empty id; a whitespace-only one is rejected here too.
	if fromIdentity && strings.TrimSpace(profileID) == "" {
		resp.Diagnostics.AddError(
			"Invalid Import Identity",
			"The 'id' attribute of the import identity must be a non-empty notification profile ID.",
		)
		return
	}
	if !fromIdentity {

		idParts := strings.Split(req.ID, ":")

		switch len(idParts) {
		case 1:
			profileID = idParts[0]
		case 2:
			customerID = idParts[0]
			profileID = idParts[1]
		default:
			resp.Diagnostics.AddError(
				"Invalid Import ID",
				fmt.Sprintf("Expected import ID in format 'profile_id' or 'customer_id:profile_id', got: %s", req.ID),
			)
			return
		}

		if strings.TrimSpace(profileID) == "" {
			resp.Diagnostics.AddError(
				"Invalid Import ID",
				fmt.Sprintf("Expected import ID in format 'profile_id' or 'customer_id:profile_id', got: %s", req.ID),
			)
			return
		}
	}

	tflog.Debug(ctx, "Importing notification profile", map[string]interface{}{
		"profile_id":  profileID,
		"customer_id": customerID,
	})

	customerIDValue := types.StringNull()
	if customerID != "" {
		customerIDValue = types.StringValue(customerID)
	}

	profile, err := r.clientFor(customerIDValue).GetNotificationProfile(ctx, profileID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Importing Notification Profile",
			"Could not import notification profile: "+err.Error(),
		)
		return
	}

	state := mapForRead(profile, nil)

	// The customer_id must be persisted, otherwise every subsequent API call for
	// this resource targets the root account and the profile silently disappears
	// from state on the next refresh.
	if customerID != "" {
		state.CustomerID = customerIDValue
	}

	identity.Set(ctx, resp.Identity, state.ID, state.CustomerID, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// notificationsToAPI converts the configured blocks into the wire shape: an
// array of single-key objects, one per block, in the configured order. Blocks
// are never merged or de-duplicated - a profile may legitimately notify the
// same contact twice with different delays.
func notificationsToAPI(notifications []NotificationModel) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(notifications))
	for _, n := range notifications {
		schedule := defaultSchedule
		if isSet(n.Schedule) {
			schedule = n.Schedule.ValueString()
		}
		result = append(result, map[string]interface{}{
			n.ContactID.ValueString(): map[string]interface{}{
				"delay":    n.Delay.ValueInt64(),
				"schedule": schedule,
			},
		})
	}
	return result
}

// mapProfileToModel produces a faithful representation of the remote object.
func mapProfileToModel(profile *client.NotificationProfile) *NotificationProfileResourceModel {
	return &NotificationProfileResourceModel{
		ID:            types.StringValue(profile.ID),
		CustomerID:    types.StringValue(profile.CustomerID),
		Name:          types.StringValue(profile.Name),
		Notifications: mapNotifications(profile.Notifications),
	}
}

// mapForRead builds a model that reflects the API response for every attribute.
// It is used by Read and ImportState. prior may be nil (import); when it is not
// nil it is used only to keep the notification ordering stable, so that a
// refresh does not manufacture a diff out of NodePing's array ordering.
func mapForRead(profile *client.NotificationProfile, prior *NotificationProfileResourceModel) *NotificationProfileResourceModel {
	model := mapProfileToModel(profile)

	if prior == nil {
		return model
	}

	if profile.CustomerID == "" && isSet(prior.CustomerID) {
		model.CustomerID = prior.CustomerID
	}
	model.Notifications = alignNotifications(prior.Notifications, model.Notifications)

	return model
}

// mapForApply builds the post-apply state. Values that were known in the plan
// are preserved verbatim - Terraform requires the applied state to match the
// plan exactly - and only values that were unknown in the plan are filled in
// from the API response.
func mapForApply(profile *client.NotificationProfile, plan *NotificationProfileResourceModel) *NotificationProfileResourceModel {
	apiModel := mapProfileToModel(profile)

	result := *plan
	if !isSet(result.ID) {
		result.ID = apiModel.ID
	}
	if !isSet(result.CustomerID) {
		result.CustomerID = apiModel.CustomerID
	}
	if !isSet(result.Name) {
		result.Name = apiModel.Name
	}

	result.Notifications = reconcileNotifications(plan.Notifications, apiModel.Notifications)

	return &result
}

// mapNotifications flattens the API representation
// ([]{contact_id: {delay, schedule}}) into the model. Contact IDs inside a
// single element are visited in sorted order so the result is deterministic
// regardless of Go's randomized map iteration, and duplicates are preserved
// because the configuration may legitimately contain repeated blocks.
func mapNotifications(notifications []map[string]interface{}) []NotificationModel {
	if len(notifications) == 0 {
		return nil
	}

	result := make([]NotificationModel, 0, len(notifications))
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
			if d, ok := toInt64(configMap["delay"]); ok {
				delay = d
			}
			schedule := defaultSchedule
			if s, ok := configMap["schedule"].(string); ok && s != "" {
				schedule = s
			}
			result = append(result, NotificationModel{
				ContactID: types.StringValue(contactID),
				Delay:     types.Int64Value(delay),
				Schedule:  types.StringValue(schedule),
			})
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// reconcileNotifications returns exactly one element per planned notification
// block, in the planned order, with unknown attributes filled from the matching
// notification in the API response. Notification blocks are a list, so the
// element count and ordering must match the plan exactly - collapsing repeated
// blocks or reordering them produces "block count changed from N to M".
func reconcileNotifications(planned, api []NotificationModel) []NotificationModel {
	if planned == nil {
		return nil
	}

	used := make([]bool, len(api))
	matches := make([]int, len(planned))

	// First pass: exact matches on every attribute the plan knows about, so
	// that repeated blocks for the same contact are paired up correctly.
	for pi, p := range planned {
		matches[pi] = -1
		for i, a := range api {
			if used[i] || !a.ContactID.Equal(p.ContactID) {
				continue
			}
			if isSet(p.Delay) && !a.Delay.Equal(p.Delay) {
				continue
			}
			if isSet(p.Schedule) && !a.Schedule.Equal(p.Schedule) {
				continue
			}
			used[i] = true
			matches[pi] = i
			break
		}
	}

	// Second pass: anything still unpaired matches on contact ID alone.
	for pi, p := range planned {
		if matches[pi] >= 0 {
			continue
		}
		for i, a := range api {
			if used[i] || !a.ContactID.Equal(p.ContactID) {
				continue
			}
			used[i] = true
			matches[pi] = i
			break
		}
	}

	result := make([]NotificationModel, 0, len(planned))
	for pi, p := range planned {
		n := p
		if match := matches[pi]; match >= 0 {
			if !isSet(n.Delay) {
				n.Delay = api[match].Delay
			}
			if !isSet(n.Schedule) {
				n.Schedule = api[match].Schedule
			}
		} else {
			if n.Delay.IsUnknown() {
				n.Delay = types.Int64Value(0)
			}
			if n.Schedule.IsUnknown() {
				n.Schedule = types.StringValue(defaultSchedule)
			}
		}
		result = append(result, n)
	}

	return result
}

// alignNotifications orders the notifications reported by the API so that they
// match the ordering already recorded in state wherever possible. Entries state
// does not know about are appended, entries that disappeared are dropped: the
// result always reflects the API, only the ordering is stabilised.
func alignNotifications(prior, api []NotificationModel) []NotificationModel {
	if len(api) == 0 || len(prior) == 0 {
		return api
	}

	used := make([]bool, len(api))
	result := make([]NotificationModel, 0, len(api))

	for _, p := range prior {
		for i, a := range api {
			if used[i] {
				continue
			}
			if a.ContactID.Equal(p.ContactID) {
				used[i] = true
				result = append(result, a)
				break
			}
		}
	}
	for i, a := range api {
		if !used[i] {
			result = append(result, a)
		}
	}

	return result
}

// isSet reports whether a configured value carries data, i.e. it is neither
// null nor unknown.
func isSet(v attr.Value) bool {
	return !v.IsNull() && !v.IsUnknown()
}

// toInt64 decodes the numeric representations NodePing uses interchangeably:
// JSON numbers, json.Number and decimal strings.
func toInt64(v interface{}) (int64, bool) {
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
