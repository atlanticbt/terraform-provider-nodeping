package check

import (
	"context"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/atlanticbt/terraform-provider-nodeping/internal/client"
)

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

func (r *CheckResource) buildCreateRequest(ctx context.Context, plan *CheckResourceModel, passwordWO types.String, hostPasswordsWO map[string]types.String, diags *diag.Diagnostics) client.CheckCreateRequest {
	req := client.CheckCreateRequest{
		Type: plan.Type.ValueString(),
	}

	setCommonFields(ctx, plan, &req, diags)
	setContentFields(ctx, plan, &req, diags)
	setConnectionFields(plan, passwordWO, &req)
	setDNSFields(ctx, plan, &req, diags)
	setServiceFields(plan, &req)
	req.Fields = fieldsRequest(ctx, plan, diags)
	req.Hosts = hostsRequest(ctx, plan, hostPasswordsWO, diags)
	req.Notifications = notificationsRequest(plan.Notifications)

	return req
}

// setCommonFields sets the attributes every check type shares: identity, scheduling, location and tags.
func setCommonFields(ctx context.Context, plan *CheckResourceModel, req *client.CheckCreateRequest, diags *diag.Diagnostics) {
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

	if isSet(plan.OldResultFail) {
		req.OldResultFail = plan.OldResultFail.ValueBool()
	}
}

// setContentFields sets the HTTP request and response matching attributes.
func setContentFields(ctx context.Context, plan *CheckResourceModel, req *client.CheckCreateRequest, diags *diag.Diagnostics) {
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

	if isSet(plan.Data) {
		data := make(map[string]string)
		diags.Append(plan.Data.ElementsAs(ctx, &data, false)...)
		req.Data = data
	}
}

// setConnectionFields sets the connection and credential attributes.
func setConnectionFields(plan *CheckResourceModel, passwordWO types.String, req *client.CheckCreateRequest) {
	if isSet(plan.Port) {
		req.Port = apiInt(plan.Port.ValueInt64())
	}

	if isSet(plan.Username) {
		req.Username = plan.Username.ValueString()
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

	if isSet(plan.ServerName) {
		req.ServerName = plan.ServerName.ValueString()
	}

	if isSet(plan.Transport) {
		req.Transport = plan.Transport.ValueString()
	}

	if isSet(plan.SSHKey) {
		req.SSHKey = plan.SSHKey.ValueString()
	}

	if isSet(plan.ClientCert) {
		req.ClientCert = plan.ClientCert.ValueString()
	}

	// password_wo wins over the deprecated password. The two are mutually
	// exclusive at config validation time, so at most one of these fires.
	if isSet(passwordWO) {
		req.Password = passwordWO.ValueString()
	} else if isSet(plan.Password) {
		req.Password = plan.Password.ValueString()
	}
}

// setDNSFields sets the DNS and DNS-over-HTTPS/TLS attributes.
func setDNSFields(ctx context.Context, plan *CheckResourceModel, req *client.CheckCreateRequest, diags *diag.Diagnostics) {
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

	if isSet(plan.DoHDoT) && plan.Type.ValueString() == "DOHDOT" {
		req.DoHDoT = plan.DoHDoT.ValueString()
	}

	if isSet(plan.EDNS) {
		edns := make(map[string]string)
		diags.Append(plan.EDNS.ElementsAs(ctx, &edns, false)...)
		req.EDNS = edns
	}
}

// setServiceFields sets the attributes of the service-specific check types: certificates and domains, databases, SNMP, Redis, RDAP/WHOIS and audio.
func setServiceFields(plan *CheckResourceModel, req *client.CheckCreateRequest) {
	if isSet(plan.WarningDays) {
		req.WarningDays = apiInt(plan.WarningDays.ValueInt64())
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

	if isSet(plan.SNMPv) {
		req.SNMPv = plan.SNMPv.ValueString()
	}

	if isSet(plan.SNMPCom) {
		req.SNMPCom = plan.SNMPCom.ValueString()
	}

	// redistype and dohdot carry schema defaults, so they are set for every
	// check type. Only send them where NodePing understands them.
	if isSet(plan.RedisType) && plan.Type.ValueString() == "REDIS" {
		req.RedisType = plan.RedisType.ValueString()
	}

	if isSet(plan.SentinelName) {
		req.SentinelName = plan.SentinelName.ValueString()
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
}

// fieldsRequest converts the planned PUSH fields into their API form, or nil
// when none are configured.
func fieldsRequest(ctx context.Context, plan *CheckResourceModel, diags *diag.Diagnostics) map[string]client.CheckField {
	if !isSet(plan.Fields) {
		return nil
	}
	fields := make(map[string]CheckFieldModel)
	diags.Append(plan.Fields.ElementsAs(ctx, &fields, false)...)
	if len(fields) == 0 {
		return nil
	}
	out := make(map[string]client.CheckField, len(fields))
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
		out[key] = field
	}
	return out
}

// hostsRequest converts the planned REDIS hosts into their API form, or nil
// when none are configured. A host's password_wo wins over the deprecated
// password, exactly as it does for the check level credential; the two are
// mutually exclusive per host at config validation time.
func hostsRequest(ctx context.Context, plan *CheckResourceModel, hostPasswordsWO map[string]types.String, diags *diag.Diagnostics) map[string]client.RedisHost {
	if !isSet(plan.Hosts) {
		return nil
	}
	hosts := make(map[string]RedisHostModel)
	diags.Append(plan.Hosts.ElementsAs(ctx, &hosts, false)...)
	if len(hosts) == 0 {
		return nil
	}
	out := make(map[string]client.RedisHost, len(hosts))
	for key, h := range hosts {
		password := h.Password.ValueString()
		if wo, ok := hostPasswordsWO[key]; ok && isSet(wo) {
			password = wo.ValueString()
		}
		out[key] = client.RedisHost{
			Host:     h.Host.ValueString(),
			Port:     int(h.Port.ValueInt64()),
			Password: password,
		}
	}
	return out
}

// notificationsRequest converts the planned notifications into the API's
// list of single-key maps, keyed by contact address ID.
func notificationsRequest(notifications []NotificationModel) []map[string]interface{} {
	var out []map[string]interface{}
	for _, n := range notifications {
		out = append(out, map[string]interface{}{
			n.ContactID.ValueString(): map[string]interface{}{
				"delay":    int(n.Delay.ValueInt64()),
				"schedule": n.Schedule.ValueString(),
			},
		})
	}
	return out
}
