package check

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

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
