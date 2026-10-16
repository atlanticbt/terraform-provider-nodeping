package provider

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/client"
	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/datasources/account"
	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/datasources/accounts"
	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/datasources/check"
	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/datasources/checks"
	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/datasources/contact"
	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/datasources/contactgroup"
	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/datasources/contactgroups"
	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/datasources/contacts"
	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/datasources/maintenance"
	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/datasources/maintenances"
	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/datasources/notificationprofile"
	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/datasources/notificationprofiles"
	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/datasources/schedule"
	"github.com/atlanticbt/terraform-provider-nodeping/v2/internal/datasources/schedules"
	checkresource "github.com/atlanticbt/terraform-provider-nodeping/v2/internal/resources/check"
	contactresource "github.com/atlanticbt/terraform-provider-nodeping/v2/internal/resources/contact"
	contactgroupresource "github.com/atlanticbt/terraform-provider-nodeping/v2/internal/resources/contactgroup"
	maintenanceresource "github.com/atlanticbt/terraform-provider-nodeping/v2/internal/resources/maintenance"
	notificationprofileresource "github.com/atlanticbt/terraform-provider-nodeping/v2/internal/resources/notificationprofile"
	scheduleresource "github.com/atlanticbt/terraform-provider-nodeping/v2/internal/resources/schedule"
	subaccountresource "github.com/atlanticbt/terraform-provider-nodeping/v2/internal/resources/subaccount"
)

var (
	_ provider.Provider                   = &NodePingProvider{}
	_ provider.ProviderWithValidateConfig = &NodePingProvider{}
)

type NodePingProvider struct {
	version string
}

type NodePingProviderModel struct {
	APIToken     types.String  `tfsdk:"api_token"`
	CustomerID   types.String  `tfsdk:"customer_id"`
	APIURL       types.String  `tfsdk:"api_url"`
	RateLimit    types.Float64 `tfsdk:"rate_limit"`
	MaxRetries   types.Int64   `tfsdk:"max_retries"`
	RetryWaitMin types.Int64   `tfsdk:"retry_wait_min"`
	RetryWaitMax types.Int64   `tfsdk:"retry_wait_max"`
	DefaultTags  types.List    `tfsdk:"default_tags"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &NodePingProvider{
			version: version,
		}
	}
}

func (p *NodePingProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "nodeping"
	resp.Version = p.version
}

func (p *NodePingProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The NodePing provider allows you to manage NodePing monitoring resources.",
		MarkdownDescription: `
The NodePing provider allows you to manage NodePing monitoring resources including contacts and checks.

## Authentication

The provider requires an API token for authentication. You can configure it in the provider block or via environment variables.

### Provider Configuration

` + "```hcl" + `
provider "nodeping" {
  api_token = "your-api-token"
}
` + "```" + `

### Environment Variables

- ` + "`NODEPING_API_TOKEN`" + ` - API token for authentication
- ` + "`NODEPING_CUSTOMER_ID`" + ` - Default SubAccount customer ID
- ` + "`NODEPING_API_URL`" + ` - API base URL (for testing)

## Multi-Account Usage

Use provider aliases to manage multiple accounts:

` + "```hcl" + `
provider "nodeping" {
  alias     = "primary"
  api_token = var.primary_token
}

provider "nodeping" {
  alias       = "subaccount"
  api_token   = var.primary_token
  customer_id = "SUBACCOUNT_ID"
}
` + "```" + `

## Default Tags

You can define default tags at the provider level that will be automatically applied to all resources that support tags (e.g., checks):

` + "```hcl" + `
provider "nodeping" {
  api_token    = var.nodeping_token
  default_tags = ["managed-by-terraform", "team-devops"]
}

resource "nodeping_check" "example" {
  type   = "HTTP"
  target = "https://example.com"
  label  = "Example Check"
  tags   = ["production"]
}
` + "```" + `

The check is created with ` + "`[\"managed-by-terraform\", \"team-devops\", \"production\"]`" + `. Duplicates are removed.

The merged set is exposed as the check's computed ` + "`tags_all`" + `. Its ` + "`tags`" + ` argument keeps exactly what the configuration declares, because Terraform requires an optional argument's planned value to match the configuration. Reference ` + "`tags_all`" + ` when you need the set NodePing actually stores.
`,
		Attributes: map[string]schema.Attribute{
			"api_token": schema.StringAttribute{
				Description:         "NodePing API token. Can also be set via NODEPING_API_TOKEN environment variable.",
				MarkdownDescription: "NodePing API token. Can also be set via `NODEPING_API_TOKEN` environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
			"customer_id": schema.StringAttribute{
				Description:         "SubAccount customer ID for managing SubAccount resources. Can also be set via NODEPING_CUSTOMER_ID environment variable.",
				MarkdownDescription: "SubAccount customer ID for managing SubAccount resources. Can also be set via `NODEPING_CUSTOMER_ID` environment variable.",
				Optional:            true,
			},
			"api_url": schema.StringAttribute{
				Description:         "NodePing API base URL. Must use https unless the host is a loopback address. Defaults to https://api.nodeping.com/api/1. Can also be set via NODEPING_API_URL environment variable.",
				MarkdownDescription: "NodePing API base URL. Must use `https` unless the host is a loopback address (`localhost`, `127.0.0.1`, `::1`), which stays available for local mock servers. Defaults to `https://api.nodeping.com/api/1`. Can also be set via `NODEPING_API_URL` environment variable.",
				Optional:            true,
				Validators: []validator.String{
					apiURLValidator{},
				},
			},
			"rate_limit": schema.Float64Attribute{
				Description: "Maximum requests per second to the NodePing API. Must be greater than 0. Defaults to 10.",
				Optional:    true,
				Validators: []validator.Float64{
					positiveFloat64Validator{},
				},
			},
			"max_retries": schema.Int64Attribute{
				Description: "Maximum number of retries for failed requests. Must be 0 or greater; 0 disables retries. Defaults to 3.",
				Optional:    true,
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"retry_wait_min": schema.Int64Attribute{
				Description: "Minimum wait time in seconds between retries. Must be 0 or greater and must not exceed retry_wait_max. Defaults to 1.",
				Optional:    true,
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"retry_wait_max": schema.Int64Attribute{
				Description: "Maximum wait time in seconds between retries. Must be 0 or greater. Defaults to 30.",
				Optional:    true,
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"default_tags": schema.ListAttribute{
				Description:         "Default tags to apply to all resources that support tags (e.g., checks). These tags are merged with resource-specific tags. Tags must not be empty strings.",
				MarkdownDescription: "Default tags to apply to all resources that support tags (e.g., checks). These tags are merged with resource-specific tags. Tags must not be empty strings.",
				Optional:            true,
				ElementType:         types.StringType,
				Validators: []validator.List{
					listvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
				},
			},
		},
	}
}

// ValidateConfig carries the rules that span more than one attribute, which an
// attribute validator cannot express.
func (p *NodePingProvider) ValidateConfig(ctx context.Context, req provider.ValidateConfigRequest, resp *provider.ValidateConfigResponse) {
	var config NodePingProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Both bounds have to be known and set for the comparison to mean
	// anything. When either is omitted, Configure substitutes the client
	// default, which is already a consistent pair.
	minSet := !config.RetryWaitMin.IsNull() && !config.RetryWaitMin.IsUnknown()
	maxSet := !config.RetryWaitMax.IsNull() && !config.RetryWaitMax.IsUnknown()

	if minSet && maxSet && config.RetryWaitMin.ValueInt64() > config.RetryWaitMax.ValueInt64() {
		resp.Diagnostics.AddAttributeError(
			path.Root("retry_wait_min"),
			"Invalid Retry Backoff Window",
			fmt.Sprintf(
				"retry_wait_min (%d) must not exceed retry_wait_max (%d). Swap the values or widen retry_wait_max.",
				config.RetryWaitMin.ValueInt64(),
				config.RetryWaitMax.ValueInt64(),
			),
		)
	}
}

func (p *NodePingProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	tflog.Info(ctx, "Configuring NodePing provider")

	var config NodePingProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rejectUnknownConnectionSettings(config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	conn, ok := resolveConnection(config, &resp.Diagnostics)
	if !ok {
		return
	}

	rateLimit := rateLimitFrom(config)
	retry := retryPolicyFrom(config)

	defaultTags, ok := defaultTagsFrom(ctx, config, &resp.Diagnostics)
	if !ok {
		return
	}

	c := client.NewClient(client.ClientConfig{
		APIToken:    conn.apiToken,
		CustomerID:  conn.customerID,
		BaseURL:     conn.apiURL,
		RateLimit:   rateLimit,
		Retry:       &retry,
		UserAgent:   "terraform-provider-nodeping/" + p.version,
		DefaultTags: defaultTags,
	})

	tflog.Debug(ctx, "NodePing client configured", map[string]interface{}{
		"api_url":     conn.apiURL,
		"customer_id": conn.customerID,
		"rate_limit":  rateLimit,
		"max_retries": retry.MaxRetries,
	})

	resp.DataSourceData = c
	resp.ResourceData = c
}

// connectionSettings are the effective token, customer ID and API URL after
// configuration values override the environment.
type connectionSettings struct {
	apiToken   string
	customerID string
	apiURL     string
}

// rejectUnknownConnectionSettings reports an error for each connection
// attribute whose value is unknown.
//
// Unknown values reach Configure when an attribute is derived from another
// resource's not-yet-applied output. Treating them as empty would either
// raise a misleading "missing token" error or silently fall back to an
// environment variable the practitioner did not intend to use.
func rejectUnknownConnectionSettings(config NodePingProviderModel, diags *diag.Diagnostics) {
	if config.APIToken.IsUnknown() {
		diags.AddAttributeError(
			path.Root("api_token"),
			"Unknown NodePing API Token",
			"The provider cannot create the NodePing API client as there is an unknown configuration value for the NodePing API token. "+
				"Either apply the source of the value first, set the value statically in the configuration, or use the NODEPING_API_TOKEN environment variable.",
		)
	}

	if config.CustomerID.IsUnknown() {
		diags.AddAttributeError(
			path.Root("customer_id"),
			"Unknown NodePing Customer ID",
			"The provider cannot create the NodePing API client as there is an unknown configuration value for the NodePing customer ID. "+
				"Either apply the source of the value first, set the value statically in the configuration, or use the NODEPING_CUSTOMER_ID environment variable.",
		)
	}

	if config.APIURL.IsUnknown() {
		diags.AddAttributeError(
			path.Root("api_url"),
			"Unknown NodePing API URL",
			"The provider cannot create the NodePing API client as there is an unknown configuration value for the NodePing API URL. "+
				"Either apply the source of the value first, set the value statically in the configuration, or use the NODEPING_API_URL environment variable.",
		)
	}
}

// resolveConnection applies configuration over the environment and validates
// the result. It returns false after recording an error.
func resolveConnection(config NodePingProviderModel, diags *diag.Diagnostics) (connectionSettings, bool) {
	conn := connectionSettings{
		apiToken:   stringFromConfigOrEnv(config.APIToken, "NODEPING_API_TOKEN"),
		customerID: stringFromConfigOrEnv(config.CustomerID, "NODEPING_CUSTOMER_ID"),
		apiURL:     stringFromConfigOrEnv(config.APIURL, "NODEPING_API_URL"),
	}

	if conn.apiToken == "" {
		diags.AddAttributeError(
			path.Root("api_token"),
			"Missing NodePing API Token",
			"The provider cannot create the NodePing API client as there is a missing or empty value for the NodePing API token. "+
				"Set the api_token value in the configuration or use the NODEPING_API_TOKEN environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
		return conn, false
	}

	// Repeated from the attribute validator because the effective URL can come
	// from NODEPING_API_URL, which no attribute validator ever sees.
	if conn.apiURL != "" {
		if err := validateAPIURL(conn.apiURL); err != nil {
			diags.AddAttributeError(
				path.Root("api_url"),
				"Invalid NodePing API URL",
				err.Error()+". "+insecureURLDetail,
			)
			return conn, false
		}
	}

	return conn, true
}

// stringFromConfigOrEnv returns the configured value when it is set, and the
// environment variable otherwise.
func stringFromConfigOrEnv(v types.String, envVar string) string {
	if !v.IsNull() {
		return v.ValueString()
	}
	return os.Getenv(envVar)
}

func rateLimitFrom(config NodePingProviderModel) float64 {
	if !config.RateLimit.IsNull() && !config.RateLimit.IsUnknown() {
		return config.RateLimit.ValueFloat64()
	}
	return client.DefaultRateLimit
}

// retryPolicyFrom builds the retry policy. It is passed to the client as a
// RetryPolicy so a configured 0 reaches it as 0 ("disable retries", "no wait")
// instead of being read as "unset".
func retryPolicyFrom(config NodePingProviderModel) client.RetryPolicy {
	retry := client.RetryPolicy{
		MaxRetries: client.DefaultMaxRetries,
		MinWait:    client.DefaultRetryMinWait,
		MaxWait:    client.DefaultRetryMaxWait,
	}
	if !config.MaxRetries.IsNull() && !config.MaxRetries.IsUnknown() {
		retry.MaxRetries = int(config.MaxRetries.ValueInt64())
	}
	if !config.RetryWaitMin.IsNull() && !config.RetryWaitMin.IsUnknown() {
		retry.MinWait = time.Duration(config.RetryWaitMin.ValueInt64()) * time.Second
	}
	if !config.RetryWaitMax.IsNull() && !config.RetryWaitMax.IsUnknown() {
		retry.MaxWait = time.Duration(config.RetryWaitMax.ValueInt64()) * time.Second
	}
	return retry
}

func defaultTagsFrom(ctx context.Context, config NodePingProviderModel, diags *diag.Diagnostics) ([]string, bool) {
	var defaultTags []string
	if !config.DefaultTags.IsNull() && !config.DefaultTags.IsUnknown() {
		diags.Append(config.DefaultTags.ElementsAs(ctx, &defaultTags, false)...)
		if diags.HasError() {
			return nil, false
		}
	}
	return defaultTags, true
}

func (p *NodePingProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		checkresource.NewCheckResource,
		contactresource.NewContactResource,
		contactgroupresource.NewContactGroupResource,
		maintenanceresource.NewMaintenanceResource,
		notificationprofileresource.NewNotificationProfileResource,
		scheduleresource.NewScheduleResource,
		subaccountresource.NewSubaccountResource,
	}
}

func (p *NodePingProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		account.NewAccountDataSource,
		accounts.NewAccountsDataSource,
		check.NewCheckDataSource,
		checks.NewChecksDataSource,
		contact.NewContactDataSource,
		contacts.NewContactsDataSource,
		contactgroup.NewContactGroupDataSource,
		contactgroups.NewContactGroupsDataSource,
		maintenance.NewMaintenanceDataSource,
		maintenances.NewMaintenancesDataSource,
		notificationprofile.NewNotificationProfileDataSource,
		notificationprofiles.NewNotificationProfilesDataSource,
		schedule.NewScheduleDataSource,
		schedules.NewSchedulesDataSource,
	}
}
