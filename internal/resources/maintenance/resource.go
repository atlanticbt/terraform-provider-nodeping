package maintenance

import (
	"context"
	"errors"
	"fmt"
	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/resources/identity"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/client"
)

var (
	_ resource.Resource                = &MaintenanceResource{}
	_ resource.ResourceWithConfigure   = &MaintenanceResource{}
	_ resource.ResourceWithImportState = &MaintenanceResource{}
)

type MaintenanceResource struct {
	client *client.Client
}

func NewMaintenanceResource() resource.Resource {
	return &MaintenanceResource{}
}

func (r *MaintenanceResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_maintenance"
}

func (r *MaintenanceResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = MaintenanceSchema()
}

func (r *MaintenanceResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// clientFor returns a client scoped to the SubAccount recorded on the resource,
// falling back to the provider-level client when no customer_id is set. Every
// CRUD path must go through it, otherwise a SubAccount window is looked up
// against the root account and silently appears to have been deleted.
func (r *MaintenanceResource) clientFor(customerID types.String) *client.Client {
	if customerID.IsNull() || customerID.IsUnknown() || customerID.ValueString() == "" {
		return r.client
	}
	return r.client.WithCustomerID(customerID.ValueString())
}

func (r *MaintenanceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan MaintenanceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	checklist := checklistFromModel(ctx, plan.Checklist, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	enabled := plan.Enabled.ValueBool()
	createReq := client.MaintenanceCreateRequest{
		Name:      plan.Name.ValueString(),
		Enabled:   &enabled,
		Cron:      plan.Cron.ValueString(),
		Duration:  int(plan.Duration.ValueInt64()),
		Checklist: checklist,
	}

	tflog.Debug(ctx, "Creating maintenance window", map[string]interface{}{
		"name":     createReq.Name,
		"cron":     createReq.Cron,
		"duration": createReq.Duration,
		"checks":   len(checklist),
	})

	maint, err := r.clientFor(plan.CustomerID).CreateMaintenance(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Maintenance Window",
			"Could not create maintenance window: "+err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(applyAPIToModel(ctx, maint, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Created maintenance window", map[string]interface{}{
		"id": maint.Identifier(),
	})

	identity.Set(ctx, resp.Identity, plan.ID, plan.CustomerID, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *MaintenanceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state MaintenanceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	tflog.Debug(ctx, "Reading maintenance window", map[string]interface{}{"id": id})

	maint, err := r.clientFor(state.CustomerID).GetMaintenance(ctx, id)
	if err != nil {
		var notFound *client.NotFoundError
		if errors.As(err, &notFound) {
			// A scheduled window only disappears when someone deletes it out of
			// band, so removing it from state is the correct drift response.
			// Ad-hoc windows, which delete themselves, are rejected at create
			// and import time precisely so this path can stay simple.
			tflog.Debug(ctx, "Maintenance window not found, removing from state", map[string]interface{}{"id": id})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading Maintenance Window",
			"Could not read maintenance window ID "+id+": "+err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(applyAPIToModel(ctx, maint, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	identity.Set(ctx, resp.Identity, state.ID, state.CustomerID, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *MaintenanceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan MaintenanceResourceModel
	var state MaintenanceResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	checklist := checklistFromModel(ctx, plan.Checklist, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	enabled := plan.Enabled.ValueBool()

	tflog.Debug(ctx, "Updating maintenance window", map[string]interface{}{"id": id})

	// duration and checklist are required by the API on every PUT, so the full
	// desired value is always sent rather than a partial patch.
	maint, err := r.clientFor(state.CustomerID).UpdateMaintenance(ctx, id, client.MaintenanceUpdateRequest{
		ID:        id,
		Name:      plan.Name.ValueString(),
		Enabled:   &enabled,
		Cron:      plan.Cron.ValueString(),
		Duration:  int(plan.Duration.ValueInt64()),
		Checklist: checklist,
	})
	if err != nil {
		var notFound *client.NotFoundError
		if errors.As(err, &notFound) {
			resp.Diagnostics.AddError(
				"Maintenance Window No Longer Exists",
				"Maintenance window ID "+id+" was deleted outside of OpenTofu and cannot be updated. "+
					"Remove it from state with `tofu state rm` or run a refresh, then apply again.",
			)
			return
		}
		resp.Diagnostics.AddError(
			"Error Updating Maintenance Window",
			"Could not update maintenance window ID "+id+": "+err.Error(),
		)
		return
	}

	plan.CustomerID = state.CustomerID
	resp.Diagnostics.Append(applyAPIToModel(ctx, maint, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updated maintenance window", map[string]interface{}{"id": maint.Identifier()})

	// Update writes the identity too. Both members are immutable, so this
	// re-states the same values -- but on a resource created before the core
	// supported identity there is no stored identity yet, and the framework
	// rejects an update that leaves it null.
	identity.Set(ctx, resp.Identity, plan.ID, plan.CustomerID, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *MaintenanceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state MaintenanceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	tflog.Debug(ctx, "Deleting maintenance window", map[string]interface{}{"id": id})

	if err := r.clientFor(state.CustomerID).DeleteMaintenance(ctx, id); err != nil {
		var notFound *client.NotFoundError
		if errors.As(err, &notFound) {
			// Already gone: a delete that finds nothing has achieved its goal.
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting Maintenance Window",
			"Could not delete maintenance window ID "+id+": "+err.Error(),
		)
		return
	}

	tflog.Debug(ctx, "Deleted maintenance window", map[string]interface{}{"id": id})
}

func (r *MaintenanceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// An "identity" import block is handled first; anything else is the
	// established string form, which must keep working unchanged.
	maintenanceID, customerID, fromIdentity := identity.FromImport(ctx, req, &resp.Diagnostics, "maintenance window")
	if resp.Diagnostics.HasError() {
		return
	}
	if !fromIdentity {

		var parseErr error
		maintenanceID, customerID, parseErr = parseImportID(req.ID)
		if parseErr != nil {
			resp.Diagnostics.AddError("Invalid Import ID", parseErr.Error())
			return
		}
	}

	tflog.Debug(ctx, "Importing maintenance window", map[string]interface{}{
		"maintenance_id": maintenanceID,
		"customer_id":    customerID,
	})

	customerIDValue := types.StringNull()
	if customerID != "" {
		customerIDValue = types.StringValue(customerID)
	}

	maint, err := r.clientFor(customerIDValue).GetMaintenance(ctx, maintenanceID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Importing Maintenance Window",
			"Could not import maintenance window: "+err.Error(),
		)
		return
	}

	if maint.IsAdHoc() {
		resp.Diagnostics.AddError(
			"Ad-hoc Maintenance Cannot Be Imported",
			"Maintenance window "+maintenanceID+" is an ad-hoc window. NodePing deletes ad-hoc windows as soon "+
				"as they finish running, so importing one would leave a resource that vanishes on the next "+
				"refresh and is then recreated on every apply, repeatedly disabling monitoring. "+
				"Use a scheduled window with a `cron` expression instead.",
		)
		return
	}

	if maint.Cron == "" {
		resp.Diagnostics.AddError(
			"Maintenance Window Has No Schedule",
			"Maintenance window "+maintenanceID+" has no `cron` expression. This resource manages scheduled "+
				"maintenance only. Add a schedule in the NodePing UI before importing it.",
		)
		return
	}

	// customer_id must be persisted, otherwise every subsequent API call for
	// this resource targets the root account and the window silently disappears
	// from state on the next refresh.
	if customerID == "" && maint.CustomerID != "" {
		customerIDValue = types.StringValue(maint.CustomerID)
	}

	state := MaintenanceResourceModel{CustomerID: customerIDValue}
	resp.Diagnostics.Append(applyAPIToModel(ctx, maint, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	identity.Set(ctx, resp.Identity, state.ID, state.CustomerID, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// parseImportID accepts "MAINTID" and "CUSTOMERID:MAINTID".
func parseImportID(raw string) (maintenanceID, customerID string, err error) {
	const usage = "expected import ID in format 'maintenance_id' or 'customer_id:maintenance_id', got: %q"

	parts := strings.Split(raw, ":")
	switch len(parts) {
	case 1:
		maintenanceID = parts[0]
	case 2:
		customerID = parts[0]
		maintenanceID = parts[1]
	default:
		return "", "", fmt.Errorf(usage, raw)
	}

	if strings.TrimSpace(maintenanceID) == "" {
		return "", "", fmt.Errorf(usage, raw)
	}
	if len(parts) == 2 && strings.TrimSpace(customerID) == "" {
		return "", "", fmt.Errorf(usage, raw)
	}
	if strings.EqualFold(maintenanceID, client.AdHocMaintenanceID) {
		return "", "", fmt.Errorf("%q is not an importable maintenance ID: ad-hoc windows are deleted by NodePing when they finish", raw)
	}

	return maintenanceID, customerID, nil
}

// checklistFromModel converts the checklist set into a sorted, de-duplicated
// slice. Sorting is what makes the request body — and therefore any diff of it
// — stable; a set's element order in the framework is not guaranteed.
func checklistFromModel(ctx context.Context, set types.Set, diags *diag.Diagnostics) []string {
	if set.IsNull() || set.IsUnknown() {
		return nil
	}

	var out []string
	diags.Append(set.ElementsAs(ctx, &out, false)...)
	if diags.HasError() {
		return nil
	}

	sort.Strings(out)
	return out
}

// applyAPIToModel copies an API response onto the model, preserving the
// caller-supplied customer_id when the API does not echo one back.
func applyAPIToModel(ctx context.Context, maint *client.Maintenance, model *MaintenanceResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	model.ID = types.StringValue(maint.Identifier())
	if maint.CustomerID != "" {
		model.CustomerID = types.StringValue(maint.CustomerID)
	} else if model.CustomerID.IsUnknown() {
		model.CustomerID = types.StringNull()
	}

	model.Name = types.StringValue(maint.Name)
	model.Enabled = types.BoolValue(maint.Enabled)
	model.Cron = types.StringValue(maint.Cron)
	model.Duration = types.Int64Value(int64(maint.Duration))
	model.Processing = types.BoolValue(maint.Processing)

	if maint.NextRun != nil {
		model.NextRun = types.Int64Value(*maint.NextRun)
	} else {
		model.NextRun = types.Int64Null()
	}

	checklist := append([]string(nil), maint.Checklist...)
	sort.Strings(checklist)
	set, d := types.SetValueFrom(ctx, types.StringType, checklist)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	model.Checklist = set

	return diags
}
