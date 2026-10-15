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
