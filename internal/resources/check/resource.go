package check

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/atlanticbt/terraform-provider-nodeping/internal/client"
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
	defer recordCreatedID(ctx, resp, check.ID, plan.CustomerID)

	result := mapForApply(ctx, check, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Created check", map[string]interface{}{
		"id": check.ID,
	})

	setIdentity(ctx, resp.Identity, result.ID, result.CustomerID, &resp.Diagnostics)

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

	setIdentity(ctx, resp.Identity, newState.ID, newState.CustomerID, &resp.Diagnostics)

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
	setIdentity(ctx, resp.Identity, result.ID, result.CustomerID, &resp.Diagnostics)

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
	var checkID string
	var customerID string

	// An "identity" import block is handled first; anything else is the
	// established string form, which must keep working unchanged.
	if idFromIdentity, customerFromIdentity, ok := identityImportValues(ctx, req, &resp.Diagnostics); ok {
		if resp.Diagnostics.HasError() {
			return
		}
		if idFromIdentity == "" {
			resp.Diagnostics.AddError(
				"Invalid Import Identity",
				"The 'id' attribute of the import identity must be a non-empty check ID.",
			)
			return
		}
		checkID = idFromIdentity
		customerID = customerFromIdentity
	} else {
		if resp.Diagnostics.HasError() {
			return
		}

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

	setIdentity(ctx, resp.Identity, state.ID, state.CustomerID, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// ModifyPlan computes tags_all, the union of the configured tags and the
// provider level default_tags, and it is tags_all that is sent to NodePing.
//
// The merge cannot be written into "tags" itself. Terraform requires the
// planned value of an Optional attribute to equal a known configuration value,
// so planning [team-devops, tf-acc] against a configured [team-devops] is
// rejected as an invalid plan; doing the same merge in Read instead only moves
// the same rule one step later, to the consistency check after apply. Splitting
// the attribute in two is what the AWS provider does for the same reason:
// "tags" holds what the practitioner wrote and always matches the
// configuration, and the computed "tags_all" holds what is actually applied.
//
// The configured tags are read from the config rather than the plan so that a
// removal is visible. That mattered more when "tags" was Optional+Computed and
// the framework copied prior state into the plan for an absent attribute; with
// "tags" now Optional only the plan already tracks config, and reading the
// config keeps the merge correct either way.
func (r *CheckResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Skip if destroying or client not configured.
	if req.Plan.Raw.IsNull() || r.client == nil {
		return
	}

	planDefaultInterval(ctx, req, resp)
	if resp.Diagnostics.HasError() {
		return
	}

	var configTags types.Set
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("tags"), &configTags)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// An unknown config value cannot be merged at plan time. Leaving tags_all
	// unknown lets the apply path fill it in from the API response.
	if configTags.IsUnknown() {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("tags_all"), types.SetUnknown(types.StringType))...)
		return
	}

	var explicitTags []string
	if !configTags.IsNull() {
		resp.Diagnostics.Append(configTags.ElementsAs(ctx, &explicitTags, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	merged := mergeTags(r.client.GetDefaultTags(), explicitTags)

	// NodePing omits the tags key on a check that has never had any, which Read
	// stores as null. An empty set means "send [] to clear every tag", and there
	// is nothing to clear, so planning null keeps an untagged check from planning
	// an update on every run.
	if len(merged) == 0 && !req.State.Raw.IsNull() {
		var priorAll types.Set
		resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("tags_all"), &priorAll)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if priorAll.IsNull() {
			resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("tags_all"), types.SetNull(types.StringType))...)
			if resp.Diagnostics.HasError() {
				return
			}
			collapseNoOpPlan(ctx, req, resp)
			return
		}
	}

	tagsAll, diags := types.SetValueFrom(ctx, types.StringType, merged)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("tags_all"), tagsAll)...)
	if resp.Diagnostics.HasError() {
		return
	}

	collapseNoOpPlan(ctx, req, resp)
}

// collapseNoOpPlan replaces the plan with prior state when nothing the provider
// can see has actually changed.
//
// TargetNormalizationPlanModifier keeps "target" itself out of the diff when
// the configuration spells it differently from the form NodePing stores, but by
// the time any plan modifier runs the damage is done: the framework has already
// compared the proposed plan with prior state and, finding them different,
// marked every null computed attribute unknown so the provider can fill it in.
// A plan carrying unknowns is not an empty plan. After an import, where state
// holds NodePing's canonical target and the configuration holds the
// practitioner's, that left an in-place update planned against a check with
// nothing wrong with it.
//
// Resource level ModifyPlan is the last hook in the sequence and the only one
// that can undo it. Filling the unknowns back from prior state is what decides
// the question: if every value the plan actually knows already matches state
// then the blanked attributes are the whole of the difference, and planning
// prior state verbatim is what Terraform asks a provider to do when it
// considers the configured and prior values equivalent. Any real change leaves
// a known value differing, and the plan is left alone.
func collapseNoOpPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || resp.Plan.Raw.IsNull() || resp.Plan.Raw.Equal(req.State.Raw) {
		return
	}

	filled, err := fillUnknownsFromState(resp.Plan.Raw, req.State.Raw)
	if err != nil || !filled.Equal(req.State.Raw) {
		return
	}

	tflog.Debug(ctx, "Plan differs from state only in values the framework blanked; keeping prior state")

	resp.Plan.Raw = req.State.Raw
}

// fillUnknownsFromState returns plan with every unknown value replaced by the
// value at the same path in state. A path that state does not have is left
// unknown, which keeps the caller's comparison unequal rather than silently
// treating a missing value as a match.
func fillUnknownsFromState(plan, state tftypes.Value) (tftypes.Value, error) {
	return tftypes.Transform(plan, func(p *tftypes.AttributePath, v tftypes.Value) (tftypes.Value, error) {
		if v.IsKnown() {
			return v, nil
		}
		found, remaining, err := tftypes.WalkAttributePath(state, p)
		if err != nil || len(remaining.Steps()) > 0 {
			return v, nil
		}
		stateValue, ok := found.(tftypes.Value)
		if !ok {
			return v, nil
		}
		return stateValue, nil
	})
}

// writeOnlyPassword reads password_wo out of the configuration. Write-only
// attributes are stripped from the plan and the state by Terraform core, so
// req.Plan and req.State would both yield a null value here.
func writeOnlyPassword(ctx context.Context, config tfsdk.Config, diags *diag.Diagnostics) types.String {
	var password types.String
	diags.Append(config.GetAttribute(ctx, path.Root("password_wo"), &password)...)
	return password
}

// writeOnlyHostPasswords reads hosts[*].password_wo out of the configuration,
// keyed by the same host identifier the practitioner used. As with the check
// level password_wo, the value exists only in req.Config: Terraform core strips
// write-only attributes from both the plan and the state, so reading them from
// req.Plan would silently yield nulls.
func writeOnlyHostPasswords(ctx context.Context, config tfsdk.Config, diags *diag.Diagnostics) map[string]types.String {
	var hosts types.Map
	diags.Append(config.GetAttribute(ctx, path.Root("hosts"), &hosts)...)
	if diags.HasError() || !isSet(hosts) {
		return nil
	}

	configHosts := make(map[string]RedisHostModel)
	diags.Append(hosts.ElementsAs(ctx, &configHosts, false)...)
	if diags.HasError() {
		return nil
	}

	passwords := make(map[string]types.String, len(configHosts))
	for key, h := range configHosts {
		if isSet(h.PasswordWO) {
			passwords[key] = h.PasswordWO
		}
	}
	if len(passwords) == 0 {
		return nil
	}
	return passwords
}

// ConfigValidators declares the mutually exclusive credential attributes.
func (r *CheckResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.Conflicting(
			path.MatchRoot("password"),
			path.MatchRoot("password_wo"),
		),
	}
}

// ValidateConfig performs cross-attribute validation that a per-attribute
// validator cannot express.
func (r *CheckResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config CheckResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Redis Sentinel needs the master name; the API rejects the check without
	// it. This is checked independently of the check type so that a typo in
	// redistype still produces the more specific message below.
	if isSet(config.RedisType) && config.RedisType.ValueString() == "sentinel" && !isSet(config.SentinelName) {
		resp.Diagnostics.AddAttributeError(
			path.Root("sentinelname"),
			"Missing Required Attribute",
			"sentinelname is required when redistype is \"sentinel\".",
		)
	}

	// A version without a value cannot do anything, and silently ignoring it
	// would look like a password rotation that never happened.
	if isSet(config.PasswordWOVersion) && !isSet(config.PasswordWO) {
		resp.Diagnostics.AddAttributeError(
			path.Root("password_wo_version"),
			"Missing Required Attribute",
			"password_wo_version has no effect without password_wo.",
		)
	}

	validateHostCredentials(ctx, config.Hosts, &resp.Diagnostics)

	if !isSet(config.Type) {
		return
	}
	checkType := config.Type.ValueString()

	if !typesNotRequiringTarget[checkType] && config.Target.IsNull() {
		resp.Diagnostics.AddAttributeError(
			path.Root("target"),
			"Missing Required Attribute",
			fmt.Sprintf("target is required for %s checks.", checkType),
		)
	}

	notApplicable := func(name string, value attr.Value, allowed map[string]bool, kinds string) {
		if value.IsNull() || value.IsUnknown() {
			return
		}
		// NodePing stores these boolean flags as false on check types that do
		// not use them, and returns them on read: of 35 live checks surveyed,
		// invert came back false on PUSH checks as well as HTTP ones. So a
		// refresh legitimately produces invert = false on a PUSH check, and
		// rejecting it would make the provider reject its own output. A flag
		// turned on is still a mistake worth reporting.
		if b, isBool := value.(types.Bool); isBool && !b.ValueBool() {
			return
		}
		if allowed[checkType] {
			return
		}
		// Other values only warn: NodePing keeps them when a check changes type
		// and returns them on read (three live SSL checks carry a contentstring),
		// so an error would make those checks unimportable.
		if _, isBool := value.(types.Bool); !isBool {
			resp.Diagnostics.AddAttributeWarning(
				path.Root(name),
				"Attribute Not Applicable To Check Type",
				fmt.Sprintf("%s is only used by %s checks, but the check type is %s. NodePing stores it but does not act on it.", name, kinds, checkType),
			)
			return
		}
		resp.Diagnostics.AddAttributeError(
			path.Root(name),
			"Attribute Not Applicable To Check Type",
			fmt.Sprintf("%s is only supported for %s checks, but the check type is %s.", name, kinds, checkType),
		)
	}

	const contentKinds = "HTTPCONTENT, HTTPADV, HTTPPARSE, DNS and DOHDOT"
	notApplicable("contentstring", config.ContentString, contentMatchTypes, contentKinds)
	notApplicable("regex", config.Regex, contentMatchTypes, contentKinds)
	notApplicable("invert", config.Invert, contentMatchTypes, contentKinds)

	const dnsKinds = "DNS and DOHDOT"
	notApplicable("dnstype", config.DNSType, dnsTypes, dnsKinds)
	notApplicable("dnstoresolve", config.DNSToResolve, dnsTypes, dnsKinds)
	notApplicable("dnssection", config.DNSSection, dnsTypes, dnsKinds)
	notApplicable("dnsrd", config.DNSRD, dnsTypes, dnsKinds)

	notApplicable("warningdays", config.WarningDays, warningDaysTypes,
		"SSL, WHOIS, RDAP, SMTP, IMAP4, POP3, DOHDOT and SPEC10RDDS")

	if min, ok := minIntervalByType[checkType]; ok && isSet(config.Interval) && config.Interval.ValueFloat64() < min {
		resp.Diagnostics.AddAttributeError(
			path.Root("interval"),
			"Interval Below NodePing Minimum",
			fmt.Sprintf("NodePing replaces a %s interval below %v minutes with %d, so %v would never settle. Use %v or more, or leave interval unset for %d.",
				checkType, min, rewrittenInterval, config.Interval.ValueFloat64(), min, rewrittenInterval),
		)
	}
}

// validateHostCredentials applies the check level credential rules to every
// REDIS host entry. resourcevalidator.Conflicting cannot express a path through
// a map, so the same two rules are enforced here.
func validateHostCredentials(ctx context.Context, hosts types.Map, diags *diag.Diagnostics) {
	if !isSet(hosts) {
		return
	}

	configHosts := make(map[string]RedisHostModel)
	diags.Append(hosts.ElementsAs(ctx, &configHosts, false)...)
	if diags.HasError() {
		return
	}

	// Map iteration order is randomised; sort so repeated runs report the same
	// diagnostics in the same order.
	keys := make([]string, 0, len(configHosts))
	for key := range configHosts {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		h := configHosts[key]
		entry := path.Root("hosts").AtMapKey(key)

		if isSet(h.Password) && isSet(h.PasswordWO) {
			diags.AddAttributeError(
				entry.AtName("password_wo"),
				"Invalid Attribute Combination",
				fmt.Sprintf("hosts[%q] sets both password and password_wo. Only one may be configured.", key),
			)
		}

		if isSet(h.PasswordWOVersion) && !isSet(h.PasswordWO) {
			diags.AddAttributeError(
				entry.AtName("password_wo_version"),
				"Missing Required Attribute",
				fmt.Sprintf("hosts[%q] sets password_wo_version, which has no effect without password_wo.", key),
			)
		}
	}
}

func (r *CheckResource) buildCreateRequest(ctx context.Context, plan *CheckResourceModel, passwordWO types.String, hostPasswordsWO map[string]types.String, diags *diag.Diagnostics) client.CheckCreateRequest {
	req := client.CheckCreateRequest{
		Type: plan.Type.ValueString(),
	}

	if isSet(plan.Target) {
		req.Target = plan.Target.ValueString()
	}

	if isSet(plan.Label) {
		req.Label = plan.Label.ValueString()
	}

	if isSet(plan.Enabled) {
		if plan.Enabled.ValueBool() {
			req.Enabled = "active"
		} else {
			req.Enabled = "false"
		}
	}

	if isSet(plan.Public) {
		req.Public = plan.Public.ValueBool()
	}

	if isSet(plan.Interval) {
		req.Interval = plan.Interval.ValueFloat64()
	}

	if isSet(plan.Threshold) {
		req.Threshold = int(plan.Threshold.ValueInt64())
	}

	if isSet(plan.Sens) {
		req.Sens = apiInt(plan.Sens.ValueInt64())
	}

	if isSet(plan.Mute) {
		req.Mute = plan.Mute.ValueBool()
	}

	if isSet(plan.Dep) {
		req.Dep = plan.Dep.ValueString()
	}

	if isSet(plan.Description) {
		req.Description = plan.Description.ValueString()
	}

	if isSet(plan.AutoDiag) {
		req.AutoDiag = plan.AutoDiag.ValueBool()
	}

	if isSet(plan.RunLocations) {
		var locations []string
		diags.Append(plan.RunLocations.ElementsAs(ctx, &locations, false)...)
		if len(locations) > 0 {
			req.RunLocations = locations
		}
	}

	if isSet(plan.HomeLoc) {
		req.HomeLoc = plan.HomeLoc.ValueString()
	}

	// tags_all is the merged set ModifyPlan computed; "tags" holds only what the
	// practitioner wrote and is never sent.
	// An empty (but non-null) set means "remove every tag", which must be sent
	// as [] rather than omitted — hence the pointer.
	if isSet(plan.TagsAll) {
		tags := []string{}
		diags.Append(plan.TagsAll.ElementsAs(ctx, &tags, false)...)
		sort.Strings(tags)
		req.Tags = &tags
	}

	if isSet(plan.ContentString) {
		req.ContentString = plan.ContentString.ValueString()
	}

	if isSet(plan.Regex) {
		req.Regex = plan.Regex.ValueBool()
	}

	if isSet(plan.Invert) {
		req.Invert = plan.Invert.ValueBool()
	}

	if isSet(plan.Follow) {
		req.Follow = plan.Follow.ValueBool()
	}

	if isSet(plan.Method) {
		req.Method = plan.Method.ValueString()
	}

	if isSet(plan.StatusCode) {
		req.StatusCode = apiInt(plan.StatusCode.ValueInt64())
	}

	if isSet(plan.SendHeaders) {
		headers := make(map[string]string)
		diags.Append(plan.SendHeaders.ElementsAs(ctx, &headers, false)...)
		req.SendHeaders = headers
	}

	if isSet(plan.ReceiveHeaders) {
		headers := make(map[string]string)
		diags.Append(plan.ReceiveHeaders.ElementsAs(ctx, &headers, false)...)
		req.ReceiveHeaders = headers
	}

	if isSet(plan.PostData) {
		req.PostData = plan.PostData.ValueString()
	}

	if isSet(plan.Port) {
		req.Port = apiInt(plan.Port.ValueInt64())
	}

	if isSet(plan.Username) {
		req.Username = plan.Username.ValueString()
	}

	// password_wo wins over the deprecated password. The two are mutually
	// exclusive at config validation time, so at most one of these fires.
	if isSet(passwordWO) {
		req.Password = passwordWO.ValueString()
	} else if isSet(plan.Password) {
		req.Password = plan.Password.ValueString()
	}

	if isSet(plan.Secure) {
		req.Secure = plan.Secure.ValueString()
	}

	if isSet(plan.Verify) {
		req.Verify = plan.Verify.ValueBool()
	}

	if isSet(plan.IPv6) {
		req.IPv6 = plan.IPv6.ValueBool()
	}

	if isSet(plan.DNSType) {
		req.DNSType = plan.DNSType.ValueString()
	}

	if isSet(plan.DNSToResolve) {
		req.DNSToResolve = plan.DNSToResolve.ValueString()
	}

	if isSet(plan.DNSSection) {
		req.DNSSection = plan.DNSSection.ValueString()
	}

	if isSet(plan.DNSRD) {
		req.DNSRD = plan.DNSRD.ValueBool()
	}

	if isSet(plan.Transport) {
		req.Transport = plan.Transport.ValueString()
	}

	if isSet(plan.WarningDays) {
		req.WarningDays = apiInt(plan.WarningDays.ValueInt64())
	}

	if isSet(plan.ServerName) {
		req.ServerName = plan.ServerName.ValueString()
	}

	if isSet(plan.Email) {
		req.Email = plan.Email.ValueString()
	}

	if isSet(plan.Database) {
		req.Database = plan.Database.ValueString()
	}

	if isSet(plan.Query) {
		req.Query = plan.Query.ValueString()
	}

	if isSet(plan.Namespace) {
		req.Namespace = plan.Namespace.ValueString()
	}

	if isSet(plan.SSHKey) {
		req.SSHKey = plan.SSHKey.ValueString()
	}

	if isSet(plan.ClientCert) {
		req.ClientCert = plan.ClientCert.ValueString()
	}

	if isSet(plan.SNMPv) {
		req.SNMPv = plan.SNMPv.ValueString()
	}

	if isSet(plan.SNMPCom) {
		req.SNMPCom = plan.SNMPCom.ValueString()
	}

	if isSet(plan.Data) {
		data := make(map[string]string)
		diags.Append(plan.Data.ElementsAs(ctx, &data, false)...)
		req.Data = data
	}

	if isSet(plan.Fields) {
		fields := make(map[string]CheckFieldModel)
		diags.Append(plan.Fields.ElementsAs(ctx, &fields, false)...)
		if len(fields) > 0 {
			req.Fields = make(map[string]client.CheckField, len(fields))
			for key, f := range fields {
				field := client.CheckField{Name: f.Name.ValueString()}
				if isSet(f.Min) {
					field.Min = f.Min.ValueInt64()
				}
				if isSet(f.Max) {
					field.Max = f.Max.ValueInt64()
				}
				if isSet(f.Match) {
					field.Match = f.Match.ValueString()
				}
				req.Fields[key] = field
			}
		}
	}

	if isSet(plan.Hosts) {
		hosts := make(map[string]RedisHostModel)
		diags.Append(plan.Hosts.ElementsAs(ctx, &hosts, false)...)
		if len(hosts) > 0 {
			req.Hosts = make(map[string]client.RedisHost, len(hosts))
			for key, h := range hosts {
				// password_wo wins over the deprecated password, exactly as it
				// does for the check level credential. The two are mutually
				// exclusive per host at config validation time.
				password := h.Password.ValueString()
				if wo, ok := hostPasswordsWO[key]; ok && isSet(wo) {
					password = wo.ValueString()
				}
				req.Hosts[key] = client.RedisHost{
					Host:     h.Host.ValueString(),
					Port:     int(h.Port.ValueInt64()),
					Password: password,
				}
			}
		}
	}

	// redistype and dohdot carry schema defaults, so they are set for every
	// check type. Only send them where NodePing understands them.
	if isSet(plan.RedisType) && plan.Type.ValueString() == "REDIS" {
		req.RedisType = plan.RedisType.ValueString()
	}

	if isSet(plan.SentinelName) {
		req.SentinelName = plan.SentinelName.ValueString()
	}

	if isSet(plan.DoHDoT) && plan.Type.ValueString() == "DOHDOT" {
		req.DoHDoT = plan.DoHDoT.ValueString()
	}

	if isSet(plan.OldResultFail) {
		req.OldResultFail = plan.OldResultFail.ValueBool()
	}

	if isSet(plan.EDNS) {
		edns := make(map[string]string)
		diags.Append(plan.EDNS.ElementsAs(ctx, &edns, false)...)
		req.EDNS = edns
	}

	if isSet(plan.WhoisServer) {
		req.WhoisServer = plan.WhoisServer.ValueString()
	}

	if isSet(plan.RDAPUrl) {
		req.RDAPUrl = plan.RDAPUrl.ValueString()
	}

	if isSet(plan.Ignore) {
		req.Ignore = plan.Ignore.ValueString()
	}

	if isSet(plan.VerifyVolume) {
		req.VerifyVolume = plan.VerifyVolume.ValueBool()
	}

	if isSet(plan.VolumeMin) {
		req.VolumeMin = apiInt(plan.VolumeMin.ValueInt64())
	}

	for _, n := range plan.Notifications {
		notif := map[string]interface{}{
			n.ContactID.ValueString(): map[string]interface{}{
				"delay":    int(n.Delay.ValueInt64()),
				"schedule": n.Schedule.ValueString(),
			},
		}
		req.Notifications = append(req.Notifications, notif)
	}

	return req
}

// isSet reports whether a configured value carries data, i.e. it is neither
// null nor unknown.
func isSet(v attr.Value) bool {
	return !v.IsNull() && !v.IsUnknown()
}

// mergeTags returns the union of defaults and explicit tags, defaults first,
// with duplicates removed and stable ordering.
func mergeTags(defaults, explicit []string) []string {
	merged := make([]string, 0, len(defaults)+len(explicit))
	seen := make(map[string]bool, len(defaults)+len(explicit))
	for _, tag := range append(append([]string{}, defaults...), explicit...) {
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		merged = append(merged, tag)
	}
	return merged
}

// deriveConfiguredTags splits the practitioner's own tags back out of the
// merged set the API returned, by removing the provider default_tags.
//
// A default the practitioner also listed explicitly must survive, or the next
// plan would show it being removed from "tags" on a check that is correct. The
// prior state is the only record of which tags were written by hand, so a tag
// that is both a default and present in prior "tags" is kept.
//
// Import has no prior state and therefore no way to tell the two apart; there
// the defaults are removed unconditionally, which is the reading that produces
// a configuration the practitioner would have written.
func deriveConfiguredTags(ctx context.Context, all types.Set, defaultTags []string, prior *CheckResourceModel, diags *diag.Diagnostics) types.Set {
	if all.IsNull() || all.IsUnknown() {
		return all
	}

	var merged []string
	diags.Append(all.ElementsAs(ctx, &merged, false)...)
	if diags.HasError() {
		return all
	}

	priorTags := make(map[string]bool)
	havePrior := prior != nil && isSet(prior.Tags)
	if havePrior {
		var written []string
		diags.Append(prior.Tags.ElementsAs(ctx, &written, false)...)
		if diags.HasError() {
			return all
		}
		for _, tag := range written {
			priorTags[tag] = true
		}
	}

	isDefault := make(map[string]bool, len(defaultTags))
	for _, tag := range defaultTags {
		isDefault[tag] = true
	}

	configured := make([]string, 0, len(merged))
	for _, tag := range merged {
		if isDefault[tag] && !priorTags[tag] {
			continue
		}
		configured = append(configured, tag)
	}

	// An absent "tags" argument and an empty one are different configurations,
	// and state has to match whichever the practitioner wrote. With nothing left
	// after the defaults are removed, prior state is what distinguishes them.
	if len(configured) == 0 && !havePrior {
		return types.SetNull(types.StringType)
	}

	result, d := types.SetValueFrom(ctx, types.StringType, configured)
	diags.Append(d...)
	return result
}

// mapForRead builds a model that reflects the API response for every attribute.
// It is used by Read and ImportState. prior may be nil (import); when it is not
// nil it is used only for values that the API cannot round-trip (password) or
// normalises in a way the practitioner did not ask for (the canonical form of
// the target, and the default_tags folded into the check's tag set) and to keep
// the notification ordering stable.
func mapForRead(ctx context.Context, check *client.Check, prior *CheckResourceModel, defaultTags []string, diags *diag.Diagnostics) *CheckResourceModel {
	model := &CheckResourceModel{}
	mapCheckToModel(ctx, check, model, diags)

	model.Tags = deriveConfiguredTags(ctx, model.TagsAll, defaultTags, prior, diags)

	if prior == nil {
		return model
	}

	// The API's plaintext password is never mapped, so the only value the
	// deprecated password attribute can hold is the one already in state.
	// password_wo stays null by construction.
	if isSet(prior.Password) {
		model.Password = prior.Password
	}
	if isSet(prior.PasswordWOVersion) {
		model.PasswordWOVersion = prior.PasswordWOVersion
	}

	model.Hosts = restoreHostPasswords(ctx, prior.Hosts, model.Hosts, diags)

	// Keep the practitioner's spelling when the API only rewrote the target
	// into its canonical form. normalizeTarget is scoped to the check types the
	// API was observed to rewrite, so a difference in case or a trailing slash
	// on a type it leaves alone is still reported as drift.
	if isSet(prior.Target) && normalizeTarget(check.Type, prior.Target.ValueString()) == normalizeTarget(check.Type, model.Target.ValueString()) {
		model.Target = prior.Target
	}

	model.Notifications = alignNotifications(prior.Notifications, model.Notifications)

	return model
}

// mapForApply builds the post-apply state. Values that were known in the plan
// are preserved verbatim - Terraform requires the applied state to match the
// plan exactly - and only values that were unknown in the plan are filled in
// from the API response.
func mapForApply(ctx context.Context, check *client.Check, plan *CheckResourceModel, diags *diag.Diagnostics) *CheckResourceModel {
	apiModel := &CheckResourceModel{}
	mapCheckToModel(ctx, check, apiModel, diags)

	result := *plan
	fillUnknownAttributes(&result, apiModel)

	// Terraform core strips write-only values from the plan before it reaches
	// the provider, and the framework strips them again from the response. Doing
	// it here as well costs nothing and keeps the credential out of state even
	// when the plan was produced by something that does not honour write-only
	// (opentofu#3964, mocked providers).
	result.PasswordWO = types.StringNull()
	result.Hosts = clearHostWriteOnlyPasswords(ctx, result.Hosts, diags)

	// The check ID is only ever produced by the API.
	if !isSet(result.ID) {
		result.ID = apiModel.ID
	}

	result.Notifications = reconcileNotifications(plan.Notifications, apiModel.Notifications)

	return &result
}

var attrValueType = reflect.TypeOf((*attr.Value)(nil)).Elem()

// fillUnknownAttributes copies every attr.Value field that is unknown in dst
// from src. Non attr.Value fields (the notification blocks) are left alone.
func fillUnknownAttributes(dst, src *CheckResourceModel) {
	dstValue := reflect.ValueOf(dst).Elem()
	srcValue := reflect.ValueOf(src).Elem()

	for i := 0; i < dstValue.NumField(); i++ {
		field := dstValue.Field(i)
		if !field.Type().Implements(attrValueType) {
			continue
		}
		current, ok := field.Interface().(attr.Value)
		if !ok || !current.IsUnknown() {
			continue
		}
		field.Set(srcValue.Field(i))
	}
}

// reconcileNotifications returns exactly one element per planned notification
// block, in the planned order, with unknown attributes filled from the matching
// notification in the API response. Notification blocks are a list, so the
// element count and ordering must match the plan exactly.
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
// match the ordering already recorded in state wherever possible. Entries that
// state does not know about are appended, entries that disappeared are dropped:
// the result always reflects the API, only the ordering is stabilised.
func alignNotifications(prior, api []NotificationModel) []NotificationModel {
	if len(api) == 0 {
		return api
	}
	if len(prior) == 0 {
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

const defaultSchedule = "All"

// mapCheckToModel maps an API check onto the resource model. Every attribute of
// the model is assigned, so the result is a faithful representation of the
// remote object suitable for refresh and import.
func mapCheckToModel(ctx context.Context, check *client.Check, model *CheckResourceModel, diags *diag.Diagnostics) {
	model.ID = types.StringValue(check.ID)
	model.CustomerID = types.StringValue(check.CustomerID)
	model.Type = types.StringValue(check.Type)
	model.Label = types.StringValue(check.Label)

	model.Enabled = types.BoolValue(check.Enabled == "active" || check.Enabled == "true")
	model.Public = types.BoolValue(check.Public)
	model.AutoDiag = types.BoolValue(check.AutoDiag)
	model.Mute = types.BoolValue(parseBoolInterface(check.Mute))

	if check.Interval.String() != "" {
		interval, err := check.Interval.Float64()
		if err != nil {
			diags.AddError(
				"Unexpected Check Interval",
				fmt.Sprintf("Could not parse interval %q returned for check %s: %s", check.Interval.String(), check.ID, err),
			)
		} else {
			model.Interval = types.Float64Value(interval)
		}
	}

	model.State = types.Int64Value(int64(check.State))
	model.Created = types.Int64Value(check.Created)
	model.Modified = types.Int64Value(check.Modified)

	model.Description = stringOrNull(check.Description)
	model.Dep = interfaceStringOrNull(check.Dep)
	model.HomeLoc = interfaceStringOrNull(check.HomeLoc)

	// The API only knows the merged set, so it lands in tags_all. Splitting the
	// practitioner's own tags back out needs the provider defaults and the
	// prior state, which mapForRead has and this function does not.
	if check.Tags != nil {
		tags, d := types.SetValueFrom(ctx, types.StringType, check.Tags)
		diags.Append(d...)
		model.TagsAll = tags
	} else {
		model.TagsAll = types.SetNull(types.StringType)
	}
	model.Tags = model.TagsAll

	model.RunLocations = runLocationsToList(ctx, check.RunLocations, diags)

	p := check.Parameters

	model.Target = types.StringValue(p.Target)
	model.Threshold = int64OrNull(p.Threshold, "threshold", check.ID, diags)
	model.Sens = int64OrNull(p.Sens, "sens", check.ID, diags)
	model.StatusCode = int64OrNull(p.StatusCode, "statuscode", check.ID, diags)
	model.Port = int64OrNull(p.Port, "port", check.ID, diags)
	model.WarningDays = int64OrNull(p.WarningDays, "warningdays", check.ID, diags)

	model.ContentString = stringOrNull(p.ContentString)
	model.Method = stringOrNull(p.Method)
	model.PostData = stringOrNull(p.PostData)
	model.Username = stringOrNull(p.Username)
	// NodePing returns the password in plaintext on every check response. It is
	// deliberately not mapped: copying it here would put the credential into
	// state, which is exactly what password_wo exists to prevent. mapForRead
	// carries the prior value forward for the deprecated password attribute.
	model.Password = types.StringNull()
	model.PasswordWO = types.StringNull()
	model.PasswordWOVersion = types.Int64Null()
	model.DNSType = stringOrNull(p.DNSType)
	model.DNSToResolve = stringOrNull(p.DNSToResolve)
	model.DNSSection = stringOrNull(p.DNSSection)
	model.Transport = stringOrNull(p.Transport)
	model.ServerName = stringOrNull(p.ServerName)
	model.Email = stringOrNull(p.Email)
	model.Database = stringOrNull(p.Database)
	model.Query = stringOrNull(p.Query)
	model.Namespace = stringOrNull(p.Namespace)
	model.SNMPv = stringOrNull(p.SNMPv)
	model.SNMPCom = stringOrNull(p.SNMPCom)
	model.Secure = interfaceStringOrNull(p.Secure)
	model.SSHKey = interfaceStringOrNull(p.SSHKey)
	model.ClientCert = interfaceStringOrNull(p.ClientCert)

	model.Regex = boolOrNull(p.Regex)
	model.Invert = boolOrNull(p.Invert)
	model.Follow = boolOrNull(p.Follow)
	model.IPv6 = boolOrNull(p.IPv6)
	model.Verify = boolOrNull(p.Verify)
	model.DNSRD = boolOrNull(p.DNSRD)

	if len(p.SendHeaders) > 0 {
		headers, d := types.MapValueFrom(ctx, types.StringType, p.SendHeaders)
		diags.Append(d...)
		model.SendHeaders = headers
	} else {
		model.SendHeaders = types.MapNull(types.StringType)
	}

	if len(p.ReceiveHeaders) > 0 {
		headers, d := types.MapValueFrom(ctx, types.StringType, p.ReceiveHeaders)
		diags.Append(d...)
		model.ReceiveHeaders = headers
	} else {
		model.ReceiveHeaders = types.MapNull(types.StringType)
	}

	model.WhoisServer = stringOrNull(p.WhoisServer)
	model.RDAPUrl = stringOrNull(p.RDAPUrl)
	model.Ignore = stringOrNull(p.Ignore)
	model.SentinelName = stringOrNull(p.SentinelName)
	model.CheckToken = stringOrNull(p.CheckToken)

	// redistype and dohdot carry schema defaults. The API omits them for check
	// types that do not use them, so falling back to the default here is what
	// keeps a refresh from producing a permanent diff.
	model.RedisType = stringOrDefault(p.RedisType, "standalone")
	model.DoHDoT = stringOrDefault(p.DoHDoT, "doh")

	model.OldResultFail = boolOrNull(p.OldResultFail)
	model.VerifyVolume = boolOrNull(p.VerifyVolume)
	model.VolumeMin = int64OrNull(p.VolumeMin, "volumemin", check.ID, diags)

	model.Data = dataToMap(ctx, p.Data, diags)

	if len(p.EDNS) > 0 {
		edns, d := types.MapValueFrom(ctx, types.StringType, p.EDNS)
		diags.Append(d...)
		model.EDNS = edns
	} else {
		model.EDNS = types.MapNull(types.StringType)
	}

	model.Fields = fieldsToMap(ctx, p.Fields, check.ID, diags)
	model.Hosts = hostsToMap(ctx, p.Hosts, diags)

	model.Notifications = mapNotifications(check.Notifications)
}

// stringOrDefault renders an API string, substituting a default when the API
// left the field out entirely.
func stringOrDefault(s, fallback string) types.String {
	if s == "" {
		return types.StringValue(fallback)
	}
	return types.StringValue(s)
}

// dataToMap renders the loosely typed "data" parameter as a map of strings.
// NodePing returns it as a JSON object whose values may be strings or numbers.
func dataToMap(ctx context.Context, raw interface{}, diags *diag.Diagnostics) types.Map {
	obj, ok := raw.(map[string]interface{})
	if !ok || len(obj) == 0 {
		return types.MapNull(types.StringType)
	}

	data := make(map[string]string, len(obj))
	for key, value := range obj {
		switch v := value.(type) {
		case string:
			data[key] = v
		case bool:
			data[key] = strconv.FormatBool(v)
		case nil:
			data[key] = ""
		default:
			if i, ok := toInt64(v); ok {
				data[key] = strconv.FormatInt(i, 10)
			} else {
				data[key] = fmt.Sprintf("%v", v)
			}
		}
	}

	result, d := types.MapValueFrom(ctx, types.StringType, data)
	diags.Append(d...)
	return result
}

// fieldsToMap renders the "fields" parameter onto the model.
func fieldsToMap(ctx context.Context, fields map[string]client.CheckField, checkID string, diags *diag.Diagnostics) types.Map {
	if len(fields) == 0 {
		return types.MapNull(CheckFieldObjectType())
	}

	// Map iteration order is randomised; sort so the diagnostics a malformed
	// response produces are stable.
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	elements := make(map[string]attr.Value, len(fields))
	for _, key := range keys {
		f := fields[key]
		object, d := types.ObjectValue(checkFieldAttrTypes, map[string]attr.Value{
			"name":  types.StringValue(f.Name),
			"min":   int64OrNull(f.Min, "fields."+key+".min", checkID, diags),
			"max":   int64OrNull(f.Max, "fields."+key+".max", checkID, diags),
			"match": stringOrNull(f.Match),
		})
		diags.Append(d...)
		elements[key] = object
	}

	result, d := types.MapValue(CheckFieldObjectType(), elements)
	diags.Append(d...)
	return result
}

// hostsToMap renders the REDIS "hosts" parameter onto the model. The per-host
// password is deliberately left null: NodePing echoes it back in plaintext and
// persisting it would leak the credential into state. mapForRead restores the
// value already known from the configuration.
func hostsToMap(ctx context.Context, hosts map[string]client.RedisHost, diags *diag.Diagnostics) types.Map {
	if len(hosts) == 0 {
		return types.MapNull(RedisHostObjectType())
	}

	elements := make(map[string]attr.Value, len(hosts))
	for key, h := range hosts {
		port := types.Int64Null()
		if h.Port != 0 {
			port = types.Int64Value(int64(h.Port))
		}
		object, d := types.ObjectValue(redisHostAttrTypes, map[string]attr.Value{
			"host":                types.StringValue(h.Host),
			"port":                port,
			"password":            types.StringNull(),
			"password_wo":         types.StringNull(),
			"password_wo_version": types.Int64Null(),
		})
		diags.Append(d...)
		elements[key] = object
	}

	result, d := types.MapValue(RedisHostObjectType(), elements)
	diags.Append(d...)
	return result
}

// clearHostWriteOnlyPasswords nulls hosts[*].password_wo on a hosts map,
// leaving every other member untouched.
func clearHostWriteOnlyPasswords(ctx context.Context, hosts types.Map, diags *diag.Diagnostics) types.Map {
	if !isSet(hosts) {
		return hosts
	}

	current := make(map[string]RedisHostModel)
	diags.Append(hosts.ElementsAs(ctx, &current, false)...)
	if diags.HasError() {
		return hosts
	}

	elements := make(map[string]attr.Value, len(current))
	for key, h := range current {
		object, d := types.ObjectValue(redisHostAttrTypes, map[string]attr.Value{
			"host":                h.Host,
			"port":                h.Port,
			"password":            h.Password,
			"password_wo":         types.StringNull(),
			"password_wo_version": h.PasswordWOVersion,
		})
		diags.Append(d...)
		elements[key] = object
	}

	result, d := types.MapValue(RedisHostObjectType(), elements)
	diags.Append(d...)
	return result
}

// restoreHostPasswords copies the per-host credential bookkeeping recorded in
// prior back onto the freshly read hosts map, keyed by host identifier: the
// deprecated plaintext password and the password_wo_version counter. password_wo
// itself is write-only and stays null by construction. Hosts the API no longer
// reports are dropped, so real drift in host or port still surfaces.
func restoreHostPasswords(ctx context.Context, prior, current types.Map, diags *diag.Diagnostics) types.Map {
	if !isSet(prior) || !isSet(current) {
		return current
	}

	priorHosts := make(map[string]RedisHostModel)
	diags.Append(prior.ElementsAs(ctx, &priorHosts, false)...)
	currentHosts := make(map[string]RedisHostModel)
	diags.Append(current.ElementsAs(ctx, &currentHosts, false)...)
	if diags.HasError() {
		return current
	}

	elements := make(map[string]attr.Value, len(currentHosts))
	for key, h := range currentHosts {
		password := h.Password
		version := h.PasswordWOVersion
		if p, ok := priorHosts[key]; ok {
			if isSet(p.Password) {
				password = p.Password
			}
			if isSet(p.PasswordWOVersion) {
				version = p.PasswordWOVersion
			}
		}
		object, d := types.ObjectValue(redisHostAttrTypes, map[string]attr.Value{
			"host":                h.Host,
			"port":                h.Port,
			"password":            password,
			"password_wo":         types.StringNull(),
			"password_wo_version": version,
		})
		diags.Append(d...)
		elements[key] = object
	}

	result, d := types.MapValue(RedisHostObjectType(), elements)
	diags.Append(d...)
	return result
}

// mapNotifications flattens the API notification representation
// ([]{contact_id: {delay, schedule}}) into the model. Contact IDs inside a
// single element are visited in sorted order so the result is deterministic,
// and duplicates are preserved because the configuration may legitimately
// contain repeated blocks.
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

func runLocationsToList(ctx context.Context, raw interface{}, diags *diag.Diagnostics) types.List {
	var locations []string

	switch rl := raw.(type) {
	case []interface{}:
		for _, loc := range rl {
			if s, ok := loc.(string); ok {
				locations = append(locations, s)
			}
		}
	case []string:
		locations = rl
	case string:
		if rl != "" {
			locations = []string{rl}
		}
	}

	if len(locations) == 0 {
		return types.ListNull(types.StringType)
	}

	list, d := types.ListValueFrom(ctx, types.StringType, locations)
	diags.Append(d...)
	return list
}

func stringOrNull(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

// interfaceStringOrNull renders a loosely typed API field as a string. NodePing
// returns "false" (the boolean) for a number of optional string fields when
// they are unset.
func interfaceStringOrNull(v interface{}) types.String {
	switch val := v.(type) {
	case string:
		return stringOrNull(val)
	case bool:
		if !val {
			return types.StringNull()
		}
		return types.StringValue(strconv.FormatBool(val))
	default:
		return types.StringNull()
	}
}

func boolOrNull(v interface{}) types.Bool {
	if v == nil {
		return types.BoolNull()
	}
	return types.BoolValue(parseBoolInterface(v))
}

func int64OrNull(v interface{}, name, checkID string, diags *diag.Diagnostics) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	if i, ok := toInt64(v); ok {
		return types.Int64Value(i)
	}
	if s, isString := v.(string); isString && s == "" {
		return types.Int64Null()
	}
	if b, isBool := v.(bool); isBool && !b {
		// NodePing uses false to mean "unset" for several numeric fields.
		return types.Int64Null()
	}
	diags.AddError(
		"Unexpected API Response",
		fmt.Sprintf("Could not decode %s value %#v returned for check %s as a number.", name, v, checkID),
	)
	return types.Int64Null()
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

// parseBoolInterface converts various interface{} types to bool.
// NodePing API returns booleans as bool, string ("true"/"false"), or numbers (0/1).
func parseBoolInterface(v interface{}) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val == "true" || val == "1"
	case float64:
		return val != 0
	case int:
		return val != 0
	case json.Number:
		return val.String() != "0" && val.String() != ""
	default:
		return false
	}
}

// apiInt renders a number for a check request. NodePing treats a JSON number 0
// as empty: sens 0 fails the create, and statuscode or warningdays 0 is stored
// as "". The string "0" is kept, so zero is sent as a string.
func apiInt(v int64) interface{} {
	if v == 0 {
		return "0"
	}
	return v
}

// planDefaultInterval replaces the schema's default interval of 15 on check
// types whose minimum is higher. NodePing would store rewrittenInterval, so
// planning 15 would show a change on every run.
func planDefaultInterval(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	var checkType types.String
	var configInterval types.Float64
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("type"), &checkType)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("interval"), &configInterval)...)
	if resp.Diagnostics.HasError() || !configInterval.IsNull() || !isSet(checkType) {
		return
	}
	if _, ok := minIntervalByType[checkType.ValueString()]; ok {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("interval"), types.Float64Value(rewrittenInterval))...)
	}
}
