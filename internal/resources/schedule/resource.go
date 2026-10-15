package schedule

import (
	"context"
	"errors"
	"fmt"
	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/resources/identity"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/client"
)

var (
	_ resource.Resource                = &ScheduleResource{}
	_ resource.ResourceWithConfigure   = &ScheduleResource{}
	_ resource.ResourceWithImportState = &ScheduleResource{}
)

type ScheduleResource struct {
	client *client.Client
}

func NewScheduleResource() resource.Resource {
	return &ScheduleResource{}
}

func (r *ScheduleResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_schedule"
}

func (r *ScheduleResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = ScheduleSchema()
}

func (r *ScheduleResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// clientFor returns a client scoped to the customer (SubAccount) recorded on
// the resource, falling back to the provider-level client when the resource
// does not carry a customer_id.
func (r *ScheduleResource) clientFor(customerID types.String) *client.Client {
	if customerID.IsNull() || customerID.IsUnknown() || customerID.ValueString() == "" {
		return r.client
	}
	return r.client.WithCustomerID(customerID.ValueString())
}

func (r *ScheduleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ScheduleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating schedule", map[string]interface{}{
		"id": plan.ID.ValueString(),
	})

	sent := r.put(ctx, &plan, "Creating", &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		if sent {
			// The ID is the one the practitioner chose and the endpoint is a
			// PUT, so it is known even when the call failed. If NodePing stored
			// the schedule the next plan can now reconcile or destroy it; if the
			// request never landed, the next Read finds nothing and drops it.
			identity.RecordCreatedID(ctx, resp, plan.ID.ValueString(), plan.CustomerID)
		}
		return
	}

	identity.Set(ctx, resp.Identity, plan.ID, plan.CustomerID, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ScheduleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ScheduleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading schedule", map[string]interface{}{
		"id": state.ID.ValueString(),
	})

	schedule, err := r.clientFor(state.CustomerID).GetSchedule(ctx, state.ID.ValueString())
	if err != nil {
		var notFound *client.NotFoundError
		if errors.As(err, &notFound) {
			tflog.Debug(ctx, "Schedule not found, removing from state", map[string]interface{}{
				"id": state.ID.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading Schedule",
			"Could not read schedule ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	state.ID = types.StringValue(schedule.ID)
	state.CustomerID = resolveCustomerID(state.CustomerID, schedule.CustomerID)
	mapDaysForRead(&state, schedule.Data)

	identity.Set(ctx, resp.Identity, state.ID, state.CustomerID, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ScheduleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ScheduleResourceModel
	var state ScheduleResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Both `id` and `customer_id` force replacement, so the planned values are
	// always the ones already in state here.
	if plan.CustomerID.IsNull() || plan.CustomerID.IsUnknown() {
		plan.CustomerID = state.CustomerID
	}

	tflog.Debug(ctx, "Updating schedule", map[string]interface{}{
		"id": plan.ID.ValueString(),
	})

	r.put(ctx, &plan, "Updating", &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Update writes the identity too. Both members are immutable, so this
	// re-states the same values -- but on a resource created before the core
	// supported identity there is no stored identity yet, and the framework
	// rejects an update that leaves it null.
	identity.Set(ctx, resp.Identity, plan.ID, plan.CustomerID, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ScheduleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ScheduleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting schedule", map[string]interface{}{
		"id": state.ID.ValueString(),
	})

	err := r.clientFor(state.CustomerID).DeleteSchedule(ctx, state.ID.ValueString())
	if err != nil {
		var notFound *client.NotFoundError
		if errors.As(err, &notFound) {
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting Schedule",
			"Could not delete schedule ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	tflog.Debug(ctx, "Deleted schedule", map[string]interface{}{
		"id": state.ID.ValueString(),
	})
}

func (r *ScheduleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// An "identity" import block is handled first; anything else is the
	// established string form, which must keep working unchanged.
	scheduleID, customerID, fromIdentity := identity.FromImport(ctx, req, &resp.Diagnostics, "schedule")
	if resp.Diagnostics.HasError() {
		return
	}
	if !fromIdentity {

		idParts := strings.Split(req.ID, ":")

		switch len(idParts) {
		case 1:
			scheduleID = idParts[0]
		case 2:
			customerID = idParts[0]
			scheduleID = idParts[1]
		default:
			resp.Diagnostics.AddError(
				"Invalid Import ID",
				fmt.Sprintf("Expected import ID in format 'schedule_id' or 'customer_id:schedule_id', got: %s", req.ID),
			)
			return
		}

		if scheduleID == "" || (len(idParts) == 2 && customerID == "") {
			resp.Diagnostics.AddError(
				"Invalid Import ID",
				fmt.Sprintf("Expected import ID in format 'schedule_id' or 'customer_id:schedule_id', got: %s", req.ID),
			)
			return
		}
	}

	tflog.Debug(ctx, "Importing schedule", map[string]interface{}{
		"schedule_id": scheduleID,
		"customer_id": customerID,
	})

	customerIDValue := types.StringNull()
	if customerID != "" {
		customerIDValue = types.StringValue(customerID)
	}

	schedule, err := r.clientFor(customerIDValue).GetSchedule(ctx, scheduleID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Importing Schedule",
			"Could not import schedule: "+err.Error(),
		)
		return
	}

	// The customer_id must be persisted, otherwise every subsequent API call
	// for this resource targets the root account and the schedule silently
	// disappears from state on the next refresh.
	state := ScheduleResourceModel{
		ID:         types.StringValue(schedule.ID),
		CustomerID: resolveCustomerID(customerIDValue, schedule.CustomerID),
	}
	mapDaysForRead(&state, schedule.Data)

	identity.Set(ctx, resp.Identity, state.ID, state.CustomerID, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// put issues the create-or-update PUT and folds the response back into the
// plan. The schedules endpoint has no POST, so Create and Update are the same
// call.
//
// sent reports whether the request was handed to the client, which Create needs
// in order to decide whether NodePing may now be holding a schedule it has to
// record. A rejection before that point leaves nothing behind.
func (r *ScheduleResource) put(ctx context.Context, plan *ScheduleResourceModel, action string, diags *diag.Diagnostics) (sent bool) {
	putReq := client.SchedulePutRequest{
		ID:   plan.ID.ValueString(),
		Data: daysFromModel(plan),
	}

	if len(putReq.Data) == 0 {
		diags.AddError(
			"Error "+action+" Schedule",
			"A schedule must define at least one day (monday through sunday).",
		)
		return false
	}

	schedule, err := r.clientFor(plan.CustomerID).PutSchedule(ctx, plan.ID.ValueString(), putReq)
	if err != nil {
		diags.AddError(
			"Error "+action+" Schedule",
			"Could not save schedule ID "+plan.ID.ValueString()+": "+err.Error(),
		)
		return true
	}

	plan.ID = types.StringValue(schedule.ID)
	plan.CustomerID = resolveCustomerID(plan.CustomerID, schedule.CustomerID)
	mapDaysForApply(plan, schedule.Data)

	tflog.Debug(ctx, "Saved schedule", map[string]interface{}{
		"id": schedule.ID,
	})

	return true
}

// resolveCustomerID keeps an explicitly configured customer_id and otherwise
// takes whatever the API reported. The attribute is Optional+Computed, so it
// must always end up known — never unknown — by the time state is written.
func resolveCustomerID(configured types.String, fromAPI string) types.String {
	if !configured.IsNull() && !configured.IsUnknown() && configured.ValueString() != "" {
		return configured
	}
	if fromAPI != "" {
		return types.StringValue(fromAPI)
	}
	return types.StringNull()
}

// daysFromModel converts the configured days into the API's `data` object.
// Days the practitioner left out are omitted entirely.
func daysFromModel(model *ScheduleResourceModel) map[string]client.ScheduleDay {
	data := make(map[string]client.ScheduleDay, len(client.ScheduleDayNames))
	for _, name := range client.ScheduleDayNames {
		day := model.day(name)
		if day == nil {
			continue
		}
		data[name] = client.ScheduleDay{
			Time1:    client.ScheduleTime(day.Time1.ValueString()),
			Time2:    client.ScheduleTime(day.Time2.ValueString()),
			Exclude:  client.ScheduleFlexBool(day.Exclude.ValueBool()),
			Disabled: client.ScheduleFlexBool(day.Disabled.ValueBool()),
			AllDay:   client.ScheduleFlexBool(day.AllDay.ValueBool()),
		}
	}
	if len(data) == 0 {
		return nil
	}
	return data
}

// mapDaysForApply folds a create/update response into the plan.
//
// It differs from mapDaysForRead in one way that matters: when the API echoes
// nothing back (NodePing's PUT sometimes answers with a bare acknowledgement)
// or omits a field, the planned value is preserved. Overwriting it would make
// the applied state disagree with the plan and produce an
// "inconsistent result after apply" error rather than a useful diff.
func mapDaysForApply(model *ScheduleResourceModel, apiDays map[string]client.ScheduleDay) {
	if len(apiDays) == 0 {
		normalizeDays(model)
		return
	}

	for _, name := range client.ScheduleDayNames {
		planned := model.day(name)
		apiDay, ok := apiDays[name]
		if !ok {
			// The API dropped a configured day. Keep the plan so the apply
			// stays consistent; the next refresh surfaces it as real drift.
			model.setDay(name, normalizeDay(planned))
			continue
		}
		model.setDay(name, dayToModel(apiDay, planned))
	}
}

// mapDaysForRead replaces the day models with exactly what the API reports.
// Refresh must show drift, so nothing from prior state is preserved here.
func mapDaysForRead(model *ScheduleResourceModel, apiDays map[string]client.ScheduleDay) {
	for _, name := range client.ScheduleDayNames {
		apiDay, ok := apiDays[name]
		if !ok {
			model.setDay(name, nil)
			continue
		}
		model.setDay(name, dayToModel(apiDay, nil))
	}
}

// dayToModel converts one API day into its model form. When planned is
// non-nil, a value the API omitted falls back to the planned value so the
// practitioner's configuration is not silently erased by a terse response.
func dayToModel(apiDay client.ScheduleDay, planned *DayModel) *DayModel {
	model := &DayModel{
		Time1:    stringOrPlanned(string(apiDay.Time1), plannedTime1(planned)),
		Time2:    stringOrPlanned(string(apiDay.Time2), plannedTime2(planned)),
		Exclude:  types.BoolValue(apiDay.Exclude.Bool()),
		Disabled: types.BoolValue(apiDay.Disabled.Bool()),
		AllDay:   types.BoolValue(apiDay.AllDay.Bool()),
	}
	return model
}

func plannedTime1(planned *DayModel) types.String {
	if planned == nil {
		return types.StringNull()
	}
	return planned.Time1
}

func plannedTime2(planned *DayModel) types.String {
	if planned == nil {
		return types.StringNull()
	}
	return planned.Time2
}

func stringOrPlanned(apiValue string, planned types.String) types.String {
	if apiValue != "" {
		return types.StringValue(apiValue)
	}
	if !planned.IsNull() && !planned.IsUnknown() {
		return planned
	}
	return types.StringNull()
}

// normalizeDays resolves every unknown value left in the plan. Optional +
// Computed attributes arrive unknown when the practitioner omitted them, and an
// unknown value must never reach state.
func normalizeDays(model *ScheduleResourceModel) {
	for _, name := range client.ScheduleDayNames {
		model.setDay(name, normalizeDay(model.day(name)))
	}
}

func normalizeDay(day *DayModel) *DayModel {
	if day == nil {
		return nil
	}
	return &DayModel{
		Time1:    knownOrNull(day.Time1),
		Time2:    knownOrNull(day.Time2),
		Exclude:  knownBool(day.Exclude),
		Disabled: knownBool(day.Disabled),
		AllDay:   knownBool(day.AllDay),
	}
}

func knownOrNull(value types.String) types.String {
	if value.IsUnknown() {
		return types.StringNull()
	}
	return value
}

func knownBool(value types.Bool) types.Bool {
	if value.IsUnknown() || value.IsNull() {
		return types.BoolValue(false)
	}
	return value
}
