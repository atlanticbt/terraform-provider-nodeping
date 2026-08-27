package check

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ValidCheckTypes is the full list of check types supported by the NodePing API.
var ValidCheckTypes = []string{
	"AGENT", "AUDIO", "CLUSTER", "DOHDOT", "DNS", "FTP", "HTTP", "HTTPCONTENT",
	"HTTPPARSE", "HTTPADV", "IMAP4", "MONGODB", "MTR", "MYSQL", "NTP", "PGSQL",
	"PING", "POP3", "PORT", "PUSH", "RBL", "RDAP", "RDP", "REDIS", "SIP", "SMTP",
	"SNMP", "SPEC10DNS", "SPEC10RDDS", "SSH", "SSL", "WEBSOCKET", "WHOIS",
}

// ValidRunLocationRegions are the NodePing region codes accepted by runlocations.
var ValidRunLocationRegions = []string{"nam", "lam", "eur", "eao", "wlw"}

// typesNotRequiringTarget lists check types for which "target" is not required.
var typesNotRequiringTarget = map[string]bool{
	"AGENT":      true,
	"DNS":        true,
	"PUSH":       true,
	"SPEC10DNS":  true,
	"SPEC10RDDS": true,
	"CLUSTER":    true,
}

// contentMatchTypes are check types that support contentstring/regex/invert.
var contentMatchTypes = map[string]bool{
	"HTTPCONTENT": true,
	"HTTPADV":     true,
	"HTTPPARSE":   true,
	"DNS":         true,
	"DOHDOT":      true,
}

// dnsTypes are check types that support dnstype/dnstoresolve/dnssection/dnsrd.
var dnsTypes = map[string]bool{
	"DNS":       true,
	"DOHDOT":    true,
	"SPEC10DNS": true,
}

// warningDaysTypes are check types that support warningdays.
var warningDaysTypes = map[string]bool{
	"SSL":        true,
	"WHOIS":      true,
	"RDAP":       true,
	"SMTP":       true,
	"IMAP4":      true,
	"POP3":       true,
	"DOHDOT":     true,
	"SPEC10RDDS": true,
}

type CheckResourceModel struct {
	ID             types.String        `tfsdk:"id"`
	CustomerID     types.String        `tfsdk:"customer_id"`
	Type           types.String        `tfsdk:"type"`
	Target         types.String        `tfsdk:"target"`
	Label          types.String        `tfsdk:"label"`
	Enabled        types.Bool          `tfsdk:"enabled"`
	Public         types.Bool          `tfsdk:"public"`
	Interval       types.Float64       `tfsdk:"interval"`
	Threshold      types.Int64         `tfsdk:"threshold"`
	Sens           types.Int64         `tfsdk:"sens"`
	Mute           types.Bool          `tfsdk:"mute"`
	Dep            types.String        `tfsdk:"dep"`
	Description    types.String        `tfsdk:"description"`
	RunLocations   types.List          `tfsdk:"runlocations"`
	HomeLoc        types.String        `tfsdk:"homeloc"`
	AutoDiag       types.Bool          `tfsdk:"autodiag"`
	Tags           types.Set           `tfsdk:"tags"`
	Notifications  []NotificationModel `tfsdk:"notifications"`
	State          types.Int64         `tfsdk:"state"`
	Created        types.Int64         `tfsdk:"created"`
	Modified       types.Int64         `tfsdk:"modified"`
	ContentString  types.String        `tfsdk:"contentstring"`
	Regex          types.Bool          `tfsdk:"regex"`
	Invert         types.Bool          `tfsdk:"invert"`
	Follow         types.Bool          `tfsdk:"follow"`
	Method         types.String        `tfsdk:"method"`
	StatusCode     types.Int64         `tfsdk:"statuscode"`
	SendHeaders    types.Map           `tfsdk:"sendheaders"`
	ReceiveHeaders types.Map           `tfsdk:"receiveheaders"`
	PostData       types.String        `tfsdk:"postdata"`
	Port           types.Int64         `tfsdk:"port"`
	Username       types.String        `tfsdk:"username"`
	Password       types.String        `tfsdk:"password"`
	Secure         types.String        `tfsdk:"secure"`
	Verify         types.Bool          `tfsdk:"verify"`
	IPv6           types.Bool          `tfsdk:"ipv6"`
	DNSType        types.String        `tfsdk:"dnstype"`
	DNSToResolve   types.String        `tfsdk:"dnstoresolve"`
	DNSSection     types.String        `tfsdk:"dnssection"`
	DNSRD          types.Bool          `tfsdk:"dnsrd"`
	Transport      types.String        `tfsdk:"transport"`
	WarningDays    types.Int64         `tfsdk:"warningdays"`
	ServerName     types.String        `tfsdk:"servername"`
	Email          types.String        `tfsdk:"email"`
	Database       types.String        `tfsdk:"database"`
	Query          types.String        `tfsdk:"query"`
	Namespace      types.String        `tfsdk:"namespace"`
	SSHKey         types.String        `tfsdk:"sshkey"`
	ClientCert     types.String        `tfsdk:"clientcert"`
	SNMPv          types.String        `tfsdk:"snmpv"`
	SNMPCom        types.String        `tfsdk:"snmpcom"`
}

type NotificationModel struct {
	ContactID types.String `tfsdk:"contact_id"`
	Delay     types.Int64  `tfsdk:"delay"`
	Schedule  types.String `tfsdk:"schedule"`
}

// unknownOnChange marks a computed attribute as unknown whenever any other part
// of the plan differs from prior state. Terraform core copies prior state into
// the plan for computed-only attributes, so UseStateForUnknown is a no-op for
// them; this modifier is the inverse and is what allows the provider to write a
// fresh server-side value (e.g. "modified") after an update without tripping the
// "inconsistent result after apply" check.
type unknownOnChange struct{}

func (m unknownOnChange) Description(_ context.Context) string {
	return "Value is refreshed from the API whenever the resource changes."
}

func (m unknownOnChange) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m unknownOnChange) PlanModifyInt64(_ context.Context, req planmodifier.Int64Request, resp *planmodifier.Int64Response) {
	// Create (no prior state) and destroy (no plan) need no help.
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	if !req.Plan.Raw.Equal(req.State.Raw) {
		resp.PlanValue = types.Int64Unknown()
	}
}

// intervalValidator enforces the NodePing interval domain: 0.25, 0.5 or any
// integer >= 1 (minutes).
type intervalValidator struct{}

func (v intervalValidator) Description(_ context.Context) string {
	return "must be 0.25, 0.5, or a whole number of minutes >= 1"
}

func (v intervalValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v intervalValidator) ValidateFloat64(_ context.Context, req validator.Float64Request, resp *validator.Float64Response) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	f := req.ConfigValue.ValueFloat64()
	if f == 0.25 || f == 0.5 {
		return
	}
	if f >= 1 && f == math.Trunc(f) {
		return
	}
	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Invalid Check Interval",
		fmt.Sprintf("interval must be 0.25, 0.5, or a whole number of minutes >= 1, got: %v", f),
	)
}

// runLocationValidator validates a single runlocations element.
type runLocationValidator struct{}

func (v runLocationValidator) Description(_ context.Context) string {
	return "must be a region code (nam, lam, eur, eao, wlw), a two character probe code, or an AGENT check ID"
}

func (v runLocationValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v runLocationValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if !IsValidRunLocation(req.ConfigValue.ValueString()) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Probe Location",
			fmt.Sprintf(
				"%q is not a valid runlocations entry. Expected one of the regions %s, a two character probe code (e.g. \"nj\"), or an AGENT check ID.",
				req.ConfigValue.ValueString(), strings.Join(ValidRunLocationRegions, ", "),
			),
		)
	}
}

// IsValidRunLocation reports whether s is an acceptable runlocations entry.
func IsValidRunLocation(s string) bool {
	if s == "" {
		return false
	}
	lower := strings.ToLower(s)
	for _, r := range ValidRunLocationRegions {
		if lower == r {
			return true
		}
	}
	// Two character probe codes, e.g. "nj", "ca".
	if len(s) == 2 && isAlpha(s) {
		return true
	}
	// AGENT check IDs look like 201205050153W2Q4C-0J2HSIRF.
	if len(s) >= 10 && strings.Contains(s, "-") {
		return true
	}
	return false
}

// runLocationsValidator validates every element of the runlocations list.
func runLocationsValidator() validator.List {
	return listvalidator.ValueStringsAre(runLocationValidator{})
}

func isAlpha(s string) bool {
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}

func CheckSchema() schema.Schema {
	return schema.Schema{
		Description: "Manages a NodePing check.",
		MarkdownDescription: `
Manages a NodePing check.

Checks monitor your services and send notifications when they fail or recover.

## Example Usage

### HTTP Check

` + "```hcl" + `
resource "nodeping_check" "http" {
  type    = "HTTP"
  target  = "https://example.com"
  label   = "Example Website"
  enabled = true

  interval  = 5
  threshold = 10
  sens      = 2

  runlocations = ["nam"]
}
` + "```" + `

### DNS Check

` + "```hcl" + `
resource "nodeping_check" "dns" {
  type          = "DNS"
  target        = "8.8.8.8"
  label         = "DNS Check"
  enabled       = true

  dnstype       = "A"
  dnstoresolve  = "example.com"
  contentstring = "93.184.216.34"
}
` + "```" + `

### SSL Certificate Check

` + "```hcl" + `
resource "nodeping_check" "ssl" {
  type        = "SSL"
  target      = "example.com"
  label       = "SSL Certificate"
  enabled     = true

  warningdays = 30
  servername  = "example.com"
}
` + "```" + `

## Import

Checks can be imported using the check ID:

` + "```shell" + `
terraform import nodeping_check.example 201205050153W2Q4C-0J2HSIRF
` + "```" + `

Checks belonging to a subaccount are imported with the subaccount (customer) ID
prefixed to the check ID:

` + "```shell" + `
terraform import nodeping_check.example SUBACCT-1:201205050153W2Q4C-0J2HSIRF
` + "```" + `
`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the check.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"customer_id": schema.StringAttribute{
				Description: "The customer ID (account ID) that owns this check. Set this to manage a check " +
					"in a subaccount. Changing it forces a new check to be created.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"type": schema.StringAttribute{
				Description: "The type of check. See NodePing documentation for available types.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.OneOf(ValidCheckTypes...),
				},
			},
			"target": schema.StringAttribute{
				Description: "The target URL, hostname, or IP address to check.",
				Optional:    true,
				Computed:    true,
			},
			"label": schema.StringAttribute{
				Description: "A label for the check. If not provided, the target will be used.",
				Optional:    true,
				Computed:    true,
			},
			"enabled": schema.BoolAttribute{
				Description: "Whether the check is enabled.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"public": schema.BoolAttribute{
				Description: "Whether public reports are enabled for this check.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"interval": schema.Float64Attribute{
				Description: "How often the check runs in minutes. Can be 0.25, 0.5, or any integer >= 1.",
				Optional:    true,
				Computed:    true,
				Default:     float64default.StaticFloat64(15),
				Validators: []validator.Float64{
					intervalValidator{},
				},
			},
			"threshold": schema.Int64Attribute{
				Description: "Timeout in seconds for the check.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(5),
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"sens": schema.Int64Attribute{
				Description: "Number of rechecks before status change.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(2),
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"mute": schema.BoolAttribute{
				Description: "Mute all notifications for this check.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"dep": schema.StringAttribute{
				Description: "Check ID for notification dependency.",
				Optional:    true,
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "Description or notes for the check (max 1000 characters).",
				Optional:    true,
				Computed:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(1000),
				},
			},
			"runlocations": schema.ListAttribute{
				Description: "Probe locations to run the check from.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Validators: []validator.List{
					runLocationsValidator(),
				},
			},
			"homeloc": schema.StringAttribute{
				Description: "Preferred probe location for the check.",
				Optional:    true,
				Computed:    true,
			},
			"autodiag": schema.BoolAttribute{
				Description: "Enable automated diagnostics.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"tags": schema.SetAttribute{
				Description:         "Tags for grouping checks. Provider default_tags are automatically merged with these tags.",
				MarkdownDescription: "Tags for grouping checks. Provider `default_tags` are automatically merged with these tags.",
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
				},
			},
			"state": schema.Int64Attribute{
				Description: "Current state of the check (0 = failing, 1 = passing).",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					unknownOnChange{},
				},
			},
			"created": schema.Int64Attribute{
				Description: "Timestamp when the check was created.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"modified": schema.Int64Attribute{
				Description: "Timestamp when the check was last modified.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					unknownOnChange{},
				},
			},
			"contentstring": schema.StringAttribute{
				Description: "String to match in the response.",
				Optional:    true,
				Computed:    true,
			},
			"regex": schema.BoolAttribute{
				Description: "Treat contentstring as a regular expression.",
				Optional:    true,
				Computed:    true,
			},
			"invert": schema.BoolAttribute{
				Description: "Invert the content match (does not contain).",
				Optional:    true,
				Computed:    true,
			},
			"follow": schema.BoolAttribute{
				Description: "Follow redirects (HTTP checks).",
				Optional:    true,
				Computed:    true,
			},
			"method": schema.StringAttribute{
				Description: "HTTP method for HTTPADV checks.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("GET", "POST", "PUT", "HEAD", "TRACE", "CONNECT"),
				},
			},
			"statuscode": schema.Int64Attribute{
				Description: "Expected HTTP status code.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.Int64{
					int64validator.Between(100, 599),
				},
			},
			"sendheaders": schema.MapAttribute{
				Description: "HTTP headers to send with the request.",
				Optional:    true,
				Computed:    true,
				Sensitive:   true,
				ElementType: types.StringType,
			},
			"receiveheaders": schema.MapAttribute{
				Description: "Expected HTTP headers in the response.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"postdata": schema.StringAttribute{
				Description: "POST data for HTTPADV checks.",
				Optional:    true,
				Computed:    true,
			},
			"port": schema.Int64Attribute{
				Description: "Port number for the check.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.Int64{
					int64validator.Between(1, 65535),
				},
			},
			"username": schema.StringAttribute{
				Description: "Username for authentication.",
				Optional:    true,
				Computed:    true,
			},
			"password": schema.StringAttribute{
				Description: "Password for authentication.",
				Optional:    true,
				Computed:    true,
				Sensitive:   true,
			},
			"secure": schema.StringAttribute{
				Description: "SSL/TLS mode: 'false', 'ssl', or 'starttls'.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("false", "ssl", "starttls"),
				},
			},
			"verify": schema.BoolAttribute{
				Description: "Verify SSL certificate or DNSSEC.",
				Optional:    true,
				Computed:    true,
			},
			"ipv6": schema.BoolAttribute{
				Description: "Use IPv6 for the check.",
				Optional:    true,
				Computed:    true,
			},
			"dnstype": schema.StringAttribute{
				Description: "DNS query type.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("ANY", "A", "AAAA", "CNAME", "MX", "NS", "PTR", "SOA", "SRV", "TXT"),
				},
			},
			"dnstoresolve": schema.StringAttribute{
				Description: "FQDN to resolve in DNS checks.",
				Optional:    true,
				Computed:    true,
			},
			"dnssection": schema.StringAttribute{
				Description: "DNS reply section to check.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("answer", "authority", "additional", "edns_options"),
				},
			},
			"dnsrd": schema.BoolAttribute{
				Description: "DNS Recursion Desired bit.",
				Optional:    true,
				Computed:    true,
			},
			"transport": schema.StringAttribute{
				Description: "Transport protocol for DNS/SIP checks.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("udp", "tcp", "tls", "ws", "wss"),
				},
			},
			"warningdays": schema.Int64Attribute{
				Description: "Days before certificate/domain expiry to fail.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"servername": schema.StringAttribute{
				Description: "Server name for SNI in SSL checks.",
				Optional:    true,
				Computed:    true,
			},
			"email": schema.StringAttribute{
				Description: "Email address for SMTP checks.",
				Optional:    true,
				Computed:    true,
			},
			"database": schema.StringAttribute{
				Description: "Database name for database checks.",
				Optional:    true,
				Computed:    true,
			},
			"query": schema.StringAttribute{
				Description: "Query for database checks.",
				Optional:    true,
				Computed:    true,
			},
			"namespace": schema.StringAttribute{
				Description: "MongoDB collection namespace.",
				Optional:    true,
				Computed:    true,
			},
			"sshkey": schema.StringAttribute{
				Description: "SSH private key ID for SSH checks.",
				Optional:    true,
				Computed:    true,
			},
			"clientcert": schema.StringAttribute{
				Description: "Client certificate ID for HTTPADV/DOHDOT checks.",
				Optional:    true,
				Computed:    true,
			},
			"snmpv": schema.StringAttribute{
				Description: "SNMP version ('1' or '2c').",
				Optional:    true,
				Computed:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("1", "2c"),
				},
			},
			"snmpcom": schema.StringAttribute{
				Description: "SNMP community string.",
				Optional:    true,
				Computed:    true,
				Sensitive:   true,
			},
		},
		Blocks: map[string]schema.Block{
			"notifications": schema.ListNestedBlock{
				Description: "Notification configuration for the check.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"contact_id": schema.StringAttribute{
							Description: "Contact or contact group ID to notify.",
							Required:    true,
						},
						"delay": schema.Int64Attribute{
							Description: "Delay in minutes before sending notification.",
							Optional:    true,
							Computed:    true,
							Default:     int64default.StaticInt64(0),
						},
						"schedule": schema.StringAttribute{
							Description: "Notification schedule name.",
							Optional:    true,
							Computed:    true,
						},
					},
				},
			},
		},
	}
}
