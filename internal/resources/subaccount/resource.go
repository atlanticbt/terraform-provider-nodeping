package subaccount

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/atlanticbt/terraform-provider-nodeping/internal/client"
)

var (
	_ resource.Resource                = &SubaccountResource{}
	_ resource.ResourceWithConfigure   = &SubaccountResource{}
	_ resource.ResourceWithImportState = &SubaccountResource{}
	_ resource.ResourceWithModifyPlan  = &SubaccountResource{}
)

type SubaccountResource struct {
	client *client.Client
}

func NewSubaccountResource() resource.Resource {
	return &SubaccountResource{}
}

func (r *SubaccountResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_subaccount"
}

func (r *SubaccountResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = SubaccountSchema()
}

func (r *SubaccountResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SubaccountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SubaccountResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating subaccount", map[string]interface{}{
		"name":     plan.Name.ValueString(),
		"location": plan.Location.ValueString(),
	})

	createReq := client.SubaccountCreateRequest{
		Name:        plan.Name.ValueString(),
		ContactName: plan.ContactName.ValueString(),
		Email:       plan.Email.ValueString(),
		Timezone:    plan.Timezone.ValueString(),
		Location:    plan.Location.ValueString(),
		EmailMe:     stringOrEmpty(plan.EmailMe),
	}

	id, err := r.client.CreateSubaccount(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Subaccount",
			"Could not create subaccount: "+err.Error(),
		)
		return
	}

	tflog.Debug(ctx, "Created subaccount", map[string]interface{}{"id": id})

	// A billable account now exists. Everything below can still fail and
	// return, so its ID reaches state before any of it runs.
	defer recordCreatedID(ctx, resp, id)

	// POST does not accept `status` or `autodiagnotifications`, so anything the
	// practitioner configured for those has to be applied with a follow-up PUT.
	postCreate := client.AccountUpdateRequest{
		AutodiagNotifications: nonDefaultString(plan.AutodiagNotifications, "false"),
		Status:                nonDefaultString(plan.Status, "Active"),
	}
	if !postCreate.IsEmpty() {
		if err := r.client.UpdateAccount(ctx, id, postCreate); err != nil {
			resp.Diagnostics.AddError(
				"Error Configuring Subaccount",
				"The subaccount "+id+" was created but could not be configured: "+err.Error()+
					". It has been recorded in state under its ID; the next plan will either finish "+
					"configuring it or destroy it, depending on the configuration.",
			)
			return
		}
	}

	account, err := r.client.GetAccount(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Created Subaccount",
			"The subaccount "+id+" was created but could not be read back: "+err.Error()+
				". It has been recorded in state under its ID; run the plan again to refresh it.",
		)
		return
	}

	applyAccount(ctx, &plan, account, false, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	setIdentity(ctx, resp.Identity, plan.ID, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SubaccountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SubaccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading subaccount", map[string]interface{}{"id": state.ID.ValueString()})

	account, err := r.client.GetAccount(ctx, state.ID.ValueString())
	if err != nil {
		var notFound *client.NotFoundError
		if errors.As(err, &notFound) {
			tflog.Debug(ctx, "Subaccount not found, removing from state", map[string]interface{}{
				"id": state.ID.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading Subaccount",
			"Could not read subaccount ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	applyAccount(ctx, &state, account, true, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// An imported subaccount has no value for the attributes NodePing never
	// returns. Seed them with their documented defaults so the next plan is not
	// a diff against null.
	if state.EmailMe.IsNull() || state.EmailMe.IsUnknown() {
		state.EmailMe = types.StringValue("no")
	}
	if state.AutodiagNotifications.IsNull() || state.AutodiagNotifications.IsUnknown() {
		state.AutodiagNotifications = types.StringValue("false")
	}

	setIdentity(ctx, resp.Identity, state.ID, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SubaccountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan SubaccountResourceModel
	var state SubaccountResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	tflog.Debug(ctx, "Updating subaccount", map[string]interface{}{"id": id})

	updateReq := client.AccountUpdateRequest{
		Name:                  plan.Name.ValueString(),
		Timezone:              plan.Timezone.ValueString(),
		Location:              plan.Location.ValueString(),
		EmailMe:               stringOrEmpty(plan.EmailMe),
		AutodiagNotifications: stringOrEmpty(plan.AutodiagNotifications),
		Status:                stringOrEmpty(plan.Status),
	}

	if err := r.client.UpdateAccount(ctx, id, updateReq); err != nil {
		var notFound *client.NotFoundError
		if errors.As(err, &notFound) {
			resp.Diagnostics.AddError(
				"Subaccount No Longer Exists",
				"Subaccount ID "+id+" was not found. It may have been deleted outside Terraform. "+
					"Run 'terraform refresh' and apply again.",
			)
			return
		}
		resp.Diagnostics.AddError(
			"Error Updating Subaccount",
			"Could not update subaccount ID "+id+": "+err.Error(),
		)
		return
	}

	account, err := r.client.GetAccount(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Updated Subaccount",
			"Could not read subaccount ID "+id+" after updating it: "+err.Error(),
		)
		return
	}

	plan.ID = types.StringValue(id)
	applyAccount(ctx, &plan, account, false, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Update writes the identity too. Both members are immutable, so this
	// re-states the same values -- but on a resource created before the core
	// supported identity there is no stored identity yet, and the framework
	// rejects an update that leaves it null.
	setIdentity(ctx, resp.Identity, plan.ID, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SubaccountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SubaccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	tflog.Warn(ctx, "Deleting subaccount and every check, contact and result it owns", map[string]interface{}{
		"id": id,
	})

	err := r.client.DeleteAccount(ctx, id)
	if err != nil {
		var notFound *client.NotFoundError
		if errors.As(err, &notFound) {
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting Subaccount",
			"Could not delete subaccount ID "+id+": "+err.Error(),
		)
		return
	}

	// NodePing queues the removal rather than performing it inline. The
	// acknowledgement is the only signal available; polling for disappearance
	// would block the apply for an unbounded time with nothing to act on.
	tflog.Debug(ctx, "Subaccount queued for deletion", map[string]interface{}{"id": id})
}

func (r *SubaccountResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// An "identity" import block is handled first; anything else is the
	// established bare string ID, which must keep working unchanged.
	id, fromIdentity := identityImportValues(ctx, req, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if !fromIdentity {
		id = req.ID
	}

	id = strings.TrimSpace(id)
	if id == "" {
		if fromIdentity {
			resp.Diagnostics.AddError(
				"Invalid Import Identity",
				"The 'id' attribute of the import identity must be a non-empty NodePing account ID such as '201203232048C76FH'.",
			)
			return
		}
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			"Expected a NodePing account ID such as '201203232048C76FH', got an empty string.",
		)
		return
	}

	tflog.Debug(ctx, "Importing subaccount", map[string]interface{}{"id": id})

	setIdentity(ctx, resp.Identity, types.StringValue(id), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resource.ImportStatePassthroughID(ctx, path.Root("id"), resource.ImportStateRequest{ID: id}, resp)
}

// applyAccount copies the fields NodePing actually returns onto the model.
//
// contactname, email and autodiagnotifications are deliberately not touched:
// the API accepts them but never reports them back, so overwriting the
// configured values from a response would replace them with empty strings and
// produce permanent drift. emailme is returned, as a boolean.
// adoptRemote selects whether values the practitioner also configures are taken
// from the response. During a refresh they must be, otherwise drift is invisible.
// During create and update they must not be: a value NodePing normalises
// differently from the plan (a timezone of "-7" echoed as -7, say) would fail
// Terraform's "provider produced inconsistent result after apply" check. The
// following refresh reconciles any such difference.
func applyAccount(ctx context.Context, model *SubaccountResourceModel, account *client.Account, adoptRemote bool, diags *diag.Diagnostics) {
	model.ID = types.StringValue(account.ID)
	model.Type = types.StringValue(account.Type)
	model.CreationDate = types.Int64Value(account.CreationDate)
	model.NextBillingDate = types.StringValue(account.NextBillingDate)

	if account.CustomerName != "" && (adoptRemote || isUnset(model.Name)) {
		model.Name = types.StringValue(account.CustomerName)
	}
	if account.Timezone != "" && (adoptRemote || isUnset(model.Timezone)) {
		model.Timezone = types.StringValue(account.Timezone)
	}
	if account.Status != "" && (adoptRemote || isUnset(model.Status)) {
		model.Status = types.StringValue(account.Status)
	} else if isUnset(model.Status) {
		// NodePing creates every subaccount active.
		model.Status = types.StringValue("Active")
	}

	if account.EmailMe != "" && (adoptRemote || isUnset(model.EmailMe)) {
		model.EmailMe = types.StringValue(account.EmailMe)
	}
	// location is never returned. defaultlocations is derived from it, so an
	// imported subaccount takes it from there when there is exactly one.
	if isUnset(model.Location) && len(account.DefaultLocations) == 1 {
		model.Location = types.StringValue(account.DefaultLocations[0])
	}

	locations, d := types.ListValueFrom(ctx, types.StringType, account.DefaultLocations)
	diags.Append(d...)
	if diags.HasError() {
		return
	}
	model.DefaultLocations = locations
}

func isUnset(v types.String) bool {
	return v.IsNull() || v.IsUnknown()
}

// stringOrEmpty renders a possibly-null attribute for an omitempty request
// field: an unset attribute must not be sent, so it becomes "".
func stringOrEmpty(v types.String) string {
	if v.IsNull() || v.IsUnknown() {
		return ""
	}
	return v.ValueString()
}

// nonDefaultString returns the value only when it differs from the value the
// API already applies on its own, so a create does not issue a pointless PUT.
func nonDefaultString(v types.String, apiDefault string) string {
	value := stringOrEmpty(v)
	if value == apiDefault {
		return ""
	}
	return value
}

// ModifyPlan requires the create-only fields when a subaccount is being created.
// They are Optional in the schema because NodePing never returns them, so an
// imported subaccount has none, but NodePing cannot create one without them.
func (r *SubaccountResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || !req.State.Raw.IsNull() {
		return
	}
	var plan SubaccountResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for _, f := range []struct {
		name  string
		value types.String
	}{{"contactname", plan.ContactName}, {"email", plan.Email}, {"location", plan.Location}} {
		if f.value.IsNull() {
			resp.Diagnostics.AddAttributeError(
				path.Root(f.name),
				"Missing Attribute For New Subaccount",
				f.name+" is required to create a subaccount. It may be left unset only on a subaccount that already exists.",
			)
		}
	}
}
