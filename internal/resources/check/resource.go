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

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/client"
)

var (
	_ resource.Resource                   = &CheckResource{}
	_ resource.ResourceWithConfigure      = &CheckResource{}
	_ resource.ResourceWithImportState    = &CheckResource{}
	_ resource.ResourceWithModifyPlan     = &CheckResource{}
	_ resource.ResourceWithValidateConfig = &CheckResource{}
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

	createReq := r.buildCreateRequest(ctx, &plan, &resp.Diagnostics)
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

	result := mapForApply(ctx, check, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Created check", map[string]interface{}{
		"id": check.ID,
	})

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

	check, err := r.clientFor(state.CustomerID).GetCheck(ctx, state.ID.ValueString())
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

	newState := mapForRead(ctx, check, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

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

	createReq := r.buildCreateRequest(ctx, &plan, &resp.Diagnostics)
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
	idParts := strings.Split(req.ID, ":")

	var checkID string
	var customerID string

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

	state := mapForRead(ctx, check, nil, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// The customer ID used for the import must be persisted so that subsequent
	// reads keep addressing the same (sub)account.
	if customerID != "" {
		state.CustomerID = types.StringValue(customerID)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// ModifyPlan merges the provider level default_tags into the planned tags.
//
// The explicit tags are read from the *config* rather than the plan: because
// "tags" is Optional+Computed, the framework copies prior state into the plan
// when the attribute is absent from config, which would otherwise make tag
// removals invisible.
func (r *CheckResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Skip if destroying or client not configured.
	if req.Plan.Raw.IsNull() || r.client == nil {
		return
	}

	var configTags types.Set
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("tags"), &configTags)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// An unknown config value cannot be merged at plan time; the apply path will
	// fill it in from the API response.
	if configTags.IsUnknown() {
		return
	}

	defaultTags := r.client.GetDefaultTags()

	var explicitTags []string
	if !configTags.IsNull() {
		resp.Diagnostics.Append(configTags.ElementsAs(ctx, &explicitTags, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	} else if len(defaultTags) == 0 {
		// Nothing configured and no defaults: leave whatever the framework
		// planned (prior state on update, unknown on create) alone.
		return
	}

	merged := mergeTags(defaultTags, explicitTags)

	tagsSet, diags := types.SetValueFrom(ctx, types.StringType, merged)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("tags"), tagsSet)...)
}

// ValidateConfig performs cross-attribute validation that a per-attribute
// validator cannot express.
func (r *CheckResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config CheckResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

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
		if !allowed[checkType] {
			resp.Diagnostics.AddAttributeError(
				path.Root(name),
				"Attribute Not Applicable To Check Type",
				fmt.Sprintf("%s is only supported for %s checks, but the check type is %s.", name, kinds, checkType),
			)
		}
	}

	const contentKinds = "HTTPCONTENT, HTTPADV, HTTPPARSE, DNS and DOHDOT"
	notApplicable("contentstring", config.ContentString, contentMatchTypes, contentKinds)
	notApplicable("regex", config.Regex, contentMatchTypes, contentKinds)
	notApplicable("invert", config.Invert, contentMatchTypes, contentKinds)

	const dnsKinds = "DNS, DOHDOT and SPEC10DNS"
	notApplicable("dnstype", config.DNSType, dnsTypes, dnsKinds)
	notApplicable("dnstoresolve", config.DNSToResolve, dnsTypes, dnsKinds)
	notApplicable("dnssection", config.DNSSection, dnsTypes, dnsKinds)
	notApplicable("dnsrd", config.DNSRD, dnsTypes, dnsKinds)

	notApplicable("warningdays", config.WarningDays, warningDaysTypes,
		"SSL, WHOIS, RDAP, SMTP, IMAP4, POP3, DOHDOT and SPEC10RDDS")
}

func (r *CheckResource) buildCreateRequest(ctx context.Context, plan *CheckResourceModel, diags *diag.Diagnostics) client.CheckCreateRequest {
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
		req.Sens = int(plan.Sens.ValueInt64())
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

	// Tags are already merged with default_tags in ModifyPlan.
	// An empty (but non-null) set means "remove every tag", which must be sent
	// as [] rather than omitted — hence the pointer.
	if isSet(plan.Tags) {
		tags := []string{}
		diags.Append(plan.Tags.ElementsAs(ctx, &tags, false)...)
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
		req.StatusCode = int(plan.StatusCode.ValueInt64())
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
		req.Port = int(plan.Port.ValueInt64())
	}

	if isSet(plan.Username) {
		req.Username = plan.Username.ValueString()
	}

	if isSet(plan.Password) {
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
		req.WarningDays = int(plan.WarningDays.ValueInt64())
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

// mapForRead builds a model that reflects the API response for every attribute.
// It is used by Read and ImportState. prior may be nil (import); when it is not
// nil it is used only for values that the API cannot round-trip (password) or
// normalises in a semantically irrelevant way (trailing slash on target) and to
// keep the notification ordering stable.
func mapForRead(ctx context.Context, check *client.Check, prior *CheckResourceModel, diags *diag.Diagnostics) *CheckResourceModel {
	model := &CheckResourceModel{}
	mapCheckToModel(ctx, check, model, diags)

	if prior == nil {
		return model
	}

	// The API never returns the password, so keep whatever we already know.
	if !isSet(model.Password) && isSet(prior.Password) {
		model.Password = prior.Password
	}

	// Ignore purely cosmetic normalisation of the target.
	if isSet(prior.Target) && normalizeURL(prior.Target.ValueString()) == normalizeURL(model.Target.ValueString()) {
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

	if check.Tags != nil {
		tags, d := types.SetValueFrom(ctx, types.StringType, check.Tags)
		diags.Append(d...)
		model.Tags = tags
	} else {
		model.Tags = types.SetNull(types.StringType)
	}

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
	model.Password = stringOrNull(p.Password)
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

	model.Notifications = mapNotifications(check.Notifications)
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

func normalizeURL(u string) string {
	return strings.TrimSuffix(u, "/")
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
