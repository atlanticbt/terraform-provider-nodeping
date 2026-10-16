package check

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/resources/identity"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/client"
)

var (
	_ resource.Resource                     = &CheckResource{}
	_ resource.ResourceWithConfigure        = &CheckResource{}
	_ resource.ResourceWithImportState      = &CheckResource{}
	_ resource.ResourceWithModifyPlan       = &CheckResource{}
	_ resource.ResourceWithValidateConfig   = &CheckResource{}
	_ resource.ResourceWithConfigValidators = &CheckResource{}
)

type CheckResource struct {
	client *client.Client
}

func NewCheckResource() resource.Resource {
	return &CheckResource{}
}

func (r *CheckResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_check"
}

func (r *CheckResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = CheckSchema()
}

func (r *CheckResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T.", req.ProviderData),
		)
		return
	}

	r.client = c
}

// clientFor returns a client scoped to the check's customer (subaccount) when
// customer_id is set, and the provider level client otherwise.
func (r *CheckResource) clientFor(customerID types.String) *client.Client {
	if r.client == nil {
		return nil
	}
	if isSet(customerID) && customerID.ValueString() != "" {
		return r.client.WithCustomerID(customerID.ValueString())
	}
	return r.client
}

func (r *CheckResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan CheckResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating check", map[string]interface{}{
		"type":   plan.Type.ValueString(),
		"target": plan.Target.ValueString(),
	})

	// Write-only attributes are always null in req.Plan; the configured value
	// only exists in req.Config.
	passwordWO := writeOnlyPassword(ctx, req.Config, &resp.Diagnostics)
	hostPasswordsWO := writeOnlyHostPasswords(ctx, req.Config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq := r.buildCreateRequest(ctx, &plan, passwordWO, hostPasswordsWO, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	check, err := r.clientFor(plan.CustomerID).CreateCheck(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Check",
			"Could not create check: "+err.Error(),
		)
		return
	}

	// NodePing owns a check from here on. Every step below can still fail and
	// return, so the identifier reaches state before any of them do.
	defer identity.RecordCreatedID(ctx, resp, check.ID, plan.CustomerID)

	result := mapForApply(ctx, check, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Created check", map[string]interface{}{
		"id": check.ID,
	})

	identity.Set(ctx, resp.Identity, result.ID, result.CustomerID, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, result)...)
}

func (r *CheckResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state CheckResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading check", map[string]interface{}{
		"id": state.ID.ValueString(),
	})

	c := r.clientFor(state.CustomerID)

	check, err := c.GetCheck(ctx, state.ID.ValueString())
	if err != nil {
		var nfErr *client.NotFoundError
		if errors.As(err, &nfErr) {
			tflog.Debug(ctx, "Check not found, removing from state", map[string]interface{}{
				"id": state.ID.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading Check",
			"Could not read check ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	newState := mapForRead(ctx, check, &state, c.GetDefaultTags(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	identity.Set(ctx, resp.Identity, newState.ID, newState.CustomerID, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, newState)...)
}

func (r *CheckResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan CheckResourceModel
	var state CheckResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating check", map[string]interface{}{
		"id": state.ID.ValueString(),
	})

	// Write-only attributes are always null in req.Plan and req.State; the
	// configured value only exists in req.Config. A bump of
	// password_wo_version is what caused this update to be planned at all.
	passwordWO := writeOnlyPassword(ctx, req.Config, &resp.Diagnostics)
	hostPasswordsWO := writeOnlyHostPasswords(ctx, req.Config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq := r.buildCreateRequest(ctx, &plan, passwordWO, hostPasswordsWO, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	updateReq := client.CheckUpdateRequest{CheckCreateRequest: createReq}

	check, err := r.clientFor(state.CustomerID).UpdateCheck(ctx, state.ID.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating Check",
			"Could not update check ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	result := mapForApply(ctx, check, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updated check", map[string]interface{}{
		"id": check.ID,
	})

	// Update writes the identity too. Both members are immutable, so this
	// re-states the same values -- but on a resource created before the core
	// supported identity there is no stored identity yet, and the framework
	// rejects an update that leaves it null.
	identity.Set(ctx, resp.Identity, result.ID, result.CustomerID, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, result)...)
}

func (r *CheckResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state CheckResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting check", map[string]interface{}{
		"id": state.ID.ValueString(),
	})

	err := r.clientFor(state.CustomerID).DeleteCheck(ctx, state.ID.ValueString())
	if err != nil {
		var nfErr *client.NotFoundError
		if errors.As(err, &nfErr) {
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting Check",
			"Could not delete check ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	tflog.Debug(ctx, "Deleted check", map[string]interface{}{
		"id": state.ID.ValueString(),
	})
}

func (r *CheckResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// An "identity" import block is handled first; anything else is the
	// established string form, which must keep working unchanged.
	checkID, customerID, fromIdentity := identity.FromImport(ctx, req, &resp.Diagnostics, "check")
	if resp.Diagnostics.HasError() {
		return
	}
	if !fromIdentity {

		idParts := strings.Split(req.ID, ":")

		switch len(idParts) {
		case 1:
			checkID = idParts[0]
		case 2:
			customerID = idParts[0]
			checkID = idParts[1]
		default:
			resp.Diagnostics.AddError(
				"Invalid Import ID",
				fmt.Sprintf("Expected import ID in format 'check_id' or 'customer_id:check_id', got: %s", req.ID),
			)
			return
		}

		if checkID == "" {
			resp.Diagnostics.AddError(
				"Invalid Import ID",
				fmt.Sprintf("Expected import ID in format 'check_id' or 'customer_id:check_id', got: %s", req.ID),
			)
			return
		}
	}

	tflog.Debug(ctx, "Importing check", map[string]interface{}{
		"check_id":    checkID,
		"customer_id": customerID,
	})

	c := r.clientFor(types.StringValue(customerID))

	check, err := c.GetCheck(ctx, checkID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Importing Check",
			"Could not import check: "+err.Error(),
		)
		return
	}

	state := mapForRead(ctx, check, nil, c.GetDefaultTags(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// The customer ID used for the import must be persisted so that subsequent
	// reads keep addressing the same (sub)account.
	if customerID != "" {
		state.CustomerID = types.StringValue(customerID)
	}

	identity.Set(ctx, resp.Identity, state.ID, state.CustomerID, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}
