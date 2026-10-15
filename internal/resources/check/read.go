package check

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/atlanticbt/terraform-provider-nodeping/internal/client"
)

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

	model.Fields = fieldsToMap(p.Fields, check.ID, diags)
	model.Hosts = hostsToMap(p.Hosts, diags)

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
func fieldsToMap(fields map[string]client.CheckField, checkID string, diags *diag.Diagnostics) types.Map {
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
func hostsToMap(hosts map[string]client.RedisHost, diags *diag.Diagnostics) types.Map {
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
