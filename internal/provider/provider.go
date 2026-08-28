package provider

import (
	"context"
	"os"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/client"
	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/datasources/account"
	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/datasources/accounts"
	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/datasources/check"
	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/datasources/checks"
	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/datasources/contact"
	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/datasources/contactgroup"
	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/datasources/contactgroups"
	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/datasources/contacts"
	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/datasources/maintenance"
	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/datasources/maintenances"
	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/datasources/notificationprofile"
	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/datasources/notificationprofiles"
	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/datasources/schedule"
	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/datasources/schedules"
	checkresource "bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/resources/check"
	contactresource "bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/resources/contact"
	contactgroupresource "bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/resources/contactgroup"
	maintenanceresource "bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/resources/maintenance"
	notificationprofileresource "bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/resources/notificationprofile"
	scheduleresource "bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/resources/schedule"
	subaccountresource "bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/resources/subaccount"
)

var _ provider.Provider = &NodePingProvider{}

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
  tags   = ["production"]  # Will be merged with default_tags
}
` + "```" + `

The resulting check will have tags: ` + "`[\"managed-by-terraform\", \"team-devops\", \"production\"]`" + `. Duplicate tags are automatically removed.
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
				Description:         "NodePing API base URL. Defaults to https://api.nodeping.com/api/1. Can also be set via NODEPING_API_URL environment variable.",
				MarkdownDescription: "NodePing API base URL. Defaults to `https://api.nodeping.com/api/1`. Can also be set via `NODEPING_API_URL` environment variable.",
				Optional:            true,
			},
			"rate_limit": schema.Float64Attribute{
				Description: "Maximum requests per second to the NodePing API. Defaults to 10.",
				Optional:    true,
			},
			"max_retries": schema.Int64Attribute{
				Description: "Maximum number of retries for failed requests. Defaults to 3.",
				Optional:    true,
			},
			"retry_wait_min": schema.Int64Attribute{
				Description: "Minimum wait time in seconds between retries. Defaults to 1.",
				Optional:    true,
			},
			"retry_wait_max": schema.Int64Attribute{
				Description: "Maximum wait time in seconds between retries. Defaults to 30.",
				Optional:    true,
			},
			"default_tags": schema.ListAttribute{
				Description:         "Default tags to apply to all resources that support tags (e.g., checks). These tags are merged with resource-specific tags.",
				MarkdownDescription: "Default tags to apply to all resources that support tags (e.g., checks). These tags are merged with resource-specific tags.",
				Optional:            true,
				ElementType:         types.StringType,
			},
		},
	}
}

func (p *NodePingProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	tflog.Info(ctx, "Configuring NodePing provider")

	var config NodePingProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Unknown values reach Configure when an attribute is derived from another
	// resource's not-yet-applied output. Treating them as empty would either
	// raise a misleading "missing token" error or silently fall back to an
	// environment variable the practitioner did not intend to use.
	if config.APIToken.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_token"),
			"Unknown NodePing API Token",
			"The provider cannot create the NodePing API client as there is an unknown configuration value for the NodePing API token. "+
				"Either apply the source of the value first, set the value statically in the configuration, or use the NODEPING_API_TOKEN environment variable.",
		)
	}

	if config.CustomerID.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("customer_id"),
			"Unknown NodePing Customer ID",
			"The provider cannot create the NodePing API client as there is an unknown configuration value for the NodePing customer ID. "+
				"Either apply the source of the value first, set the value statically in the configuration, or use the NODEPING_CUSTOMER_ID environment variable.",
		)
	}

	if config.APIURL.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_url"),
			"Unknown NodePing API URL",
			"The provider cannot create the NodePing API client as there is an unknown configuration value for the NodePing API URL. "+
				"Either apply the source of the value first, set the value statically in the configuration, or use the NODEPING_API_URL environment variable.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	apiToken := os.Getenv("NODEPING_API_TOKEN")
	if apiToken == "" {
		apiToken = os.Getenv("NODEPING_API_KEY")
	}
	if !config.APIToken.IsNull() {
		apiToken = config.APIToken.ValueString()
	}

	if apiToken == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_token"),
			"Missing NodePing API Token",
			"The provider cannot create the NodePing API client as there is a missing or empty value for the NodePing API token. "+
				"Set the api_token value in the configuration or use the NODEPING_API_TOKEN environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
		return
	}

	customerID := os.Getenv("NODEPING_CUSTOMER_ID")
	if !config.CustomerID.IsNull() {
		customerID = config.CustomerID.ValueString()
	}

	apiURL := os.Getenv("NODEPING_API_URL")
	if !config.APIURL.IsNull() {
		apiURL = config.APIURL.ValueString()
	}

	var rateLimit float64 = client.DefaultRateLimit
	if !config.RateLimit.IsNull() && !config.RateLimit.IsUnknown() {
		rateLimit = config.RateLimit.ValueFloat64()
	}

	var maxRetries int = client.DefaultMaxRetries
	if !config.MaxRetries.IsNull() && !config.MaxRetries.IsUnknown() {
		maxRetries = int(config.MaxRetries.ValueInt64())
	}

	var retryWaitMin time.Duration = client.DefaultRetryMinWait
	if !config.RetryWaitMin.IsNull() && !config.RetryWaitMin.IsUnknown() {
		retryWaitMin = time.Duration(config.RetryWaitMin.ValueInt64()) * time.Second
	}

	var retryWaitMax time.Duration = client.DefaultRetryMaxWait
	if !config.RetryWaitMax.IsNull() && !config.RetryWaitMax.IsUnknown() {
		retryWaitMax = time.Duration(config.RetryWaitMax.ValueInt64()) * time.Second
	}

	var defaultTags []string
	if !config.DefaultTags.IsNull() && !config.DefaultTags.IsUnknown() {
		resp.Diagnostics.Append(config.DefaultTags.ElementsAs(ctx, &defaultTags, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	clientCfg := client.ClientConfig{
		APIToken:     apiToken,
		CustomerID:   customerID,
		BaseURL:      apiURL,
		RateLimit:    rateLimit,
		MaxRetries:   maxRetries,
		RetryMinWait: retryWaitMin,
		RetryMaxWait: retryWaitMax,
		UserAgent:    "terraform-provider-nodeping/" + p.version,
		DefaultTags:  defaultTags,
	}

	c := client.NewClient(clientCfg)

	tflog.Debug(ctx, "NodePing client configured", map[string]interface{}{
		"api_url":     apiURL,
		"customer_id": customerID,
		"rate_limit":  rateLimit,
		"max_retries": maxRetries,
	})

	resp.DataSourceData = c
	resp.ResourceData = c
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
