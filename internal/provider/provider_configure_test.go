package provider

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-log/tflogtest"

	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/client"
)

// providerSchema returns the provider schema under test.
func providerSchema(t *testing.T, ctx context.Context) fwprovider.SchemaResponse {
	t.Helper()

	p := New("test")().(*NodePingProvider)
	resp := fwprovider.SchemaResponse{}
	p.Schema(ctx, fwprovider.SchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %v", resp.Diagnostics)
	}

	return resp
}

// configAttrs describes a provider block. A nil entry is a null attribute; the
// sentinel unknownValue marks an attribute whose value is not yet known.
type configAttrs map[string]interface{}

type unknownMarker struct{}

var unknownValue = unknownMarker{}

// newProviderConfig builds a tfsdk.Config for the provider schema from a map of
// attribute name to Go value. Attributes not present in the map are null.
func newProviderConfig(t *testing.T, ctx context.Context, attrs configAttrs) tfsdk.Config {
	t.Helper()

	s := providerSchema(t, ctx).Schema
	objType := s.Type().TerraformType(ctx).(tftypes.Object)

	values := map[string]tftypes.Value{}
	for name, attrType := range objType.AttributeTypes {
		raw, ok := attrs[name]
		switch {
		case !ok:
			values[name] = tftypes.NewValue(attrType, nil)
		default:
			if _, isUnknown := raw.(unknownMarker); isUnknown {
				values[name] = tftypes.NewValue(attrType, tftypes.UnknownValue)
				continue
			}

			values[name] = tftypes.NewValue(attrType, raw)
		}
	}

	for name := range attrs {
		if _, ok := objType.AttributeTypes[name]; !ok {
			t.Fatalf("attribute %q is not part of the provider schema", name)
		}
	}

	return tfsdk.Config{Schema: s, Raw: tftypes.NewValue(objType, values)}
}

// configure runs Configure with the given provider block and returns the
// response. Environment variables must be set by the caller via t.Setenv.
func configure(t *testing.T, ctx context.Context, attrs configAttrs) fwprovider.ConfigureResponse {
	t.Helper()

	p := New("test")().(*NodePingProvider)
	req := fwprovider.ConfigureRequest{Config: newProviderConfig(t, ctx, attrs)}
	resp := fwprovider.ConfigureResponse{}
	p.Configure(ctx, req, &resp)

	return resp
}

// clearNodePingEnv unsets every environment variable Configure reads so a test
// starts from a known state. t.Setenv also marks the test as non-parallel.
func clearNodePingEnv(t *testing.T) {
	t.Helper()

	for _, key := range []string{
		"NODEPING_API_TOKEN",
		"NODEPING_API_KEY",
		"NODEPING_CUSTOMER_ID",
		"NODEPING_API_URL",
	} {
		t.Setenv(key, "")
	}
}

func configuredClient(t *testing.T, resp fwprovider.ConfigureResponse) *client.Client {
	t.Helper()

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected configure diagnostics: %v", resp.Diagnostics)
	}

	c, ok := resp.ResourceData.(*client.Client)
	if !ok || c == nil {
		t.Fatalf("expected ResourceData to be a non-nil *client.Client, got %#v", resp.ResourceData)
	}

	if resp.DataSourceData != resp.ResourceData {
		t.Errorf("expected DataSourceData and ResourceData to be the same client")
	}

	return c
}

// hasAttributeError reports whether the diagnostics contain an error whose
// summary contains want.
func hasErrorSummary(t *testing.T, resp fwprovider.ConfigureResponse, want string) bool {
	t.Helper()

	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), want) {
			return true
		}
	}

	return false
}

func TestProviderMetadata_typeNameAndVersion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	p := New("1.2.3")().(*NodePingProvider)

	resp := fwprovider.MetadataResponse{}
	p.Metadata(ctx, fwprovider.MetadataRequest{}, &resp)

	if resp.TypeName != "nodeping" {
		t.Errorf("expected TypeName %q, got %q", "nodeping", resp.TypeName)
	}

	if resp.Version != "1.2.3" {
		t.Errorf("expected Version %q, got %q", "1.2.3", resp.Version)
	}
}

func TestProviderSchema_apiTokenIsSensitive(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := providerSchema(t, ctx).Schema

	attr, ok := s.Attributes["api_token"]
	if !ok {
		t.Fatal("api_token attribute is missing from the provider schema")
	}

	if !attr.IsSensitive() {
		t.Error("api_token must be marked Sensitive so Terraform redacts it from plan output, logs, and state diffs")
	}

	if !attr.IsOptional() {
		t.Error("api_token must be Optional so it can come from the environment")
	}
}

func TestProviderSchema_allAttributesOptional(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := providerSchema(t, ctx).Schema

	want := []string{
		"api_token", "customer_id", "api_url", "rate_limit",
		"max_retries", "retry_wait_min", "retry_wait_max", "default_tags",
	}

	if len(s.Attributes) != len(want) {
		t.Errorf("expected %d provider attributes, got %d", len(want), len(s.Attributes))
	}

	for _, name := range want {
		attr, ok := s.Attributes[name]
		if !ok {
			t.Errorf("expected attribute %q in provider schema", name)
			continue
		}

		if attr.IsRequired() {
			t.Errorf("attribute %q must not be Required", name)
		}
	}
}

func TestProviderSchema_nonTokenAttributesNotSensitive(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := providerSchema(t, ctx).Schema

	for name, attr := range s.Attributes {
		if name == "api_token" {
			continue
		}

		if attr.IsSensitive() {
			t.Errorf("attribute %q is unexpectedly Sensitive", name)
		}
	}
}

func TestProviderConfigure_missingTokenProducesDiagnostic(t *testing.T) {
	clearNodePingEnv(t)

	ctx := context.Background()
	resp := configure(t, ctx, nil)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic when no api_token is configured")
	}

	if !hasErrorSummary(t, resp, "Missing NodePing API Token") {
		t.Errorf("expected a Missing NodePing API Token diagnostic, got %v", resp.Diagnostics)
	}

	if resp.ResourceData != nil || resp.DataSourceData != nil {
		t.Error("no client must be handed to resources when the token is missing")
	}
}

func TestProviderConfigure_emptyStringTokenProducesDiagnostic(t *testing.T) {
	clearNodePingEnv(t)
	t.Setenv("NODEPING_API_TOKEN", "token-from-env")

	ctx := context.Background()
	resp := configure(t, ctx, configAttrs{"api_token": ""})

	if !hasErrorSummary(t, resp, "Missing NodePing API Token") {
		t.Errorf("an explicitly empty api_token must not silently fall back to the environment; got %v", resp.Diagnostics)
	}
}

func TestProviderConfigure_unknownTokenProducesUnknownDiagnostic(t *testing.T) {
	clearNodePingEnv(t)
	t.Setenv("NODEPING_API_TOKEN", "token-from-env")

	ctx := context.Background()
	resp := configure(t, ctx, configAttrs{"api_token": unknownValue})

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic for an unknown api_token")
	}

	if !hasErrorSummary(t, resp, "Unknown NodePing API Token") {
		t.Errorf("an unknown api_token must produce an Unknown diagnostic, not be treated as empty; got %v", resp.Diagnostics)
	}

	if resp.ResourceData != nil {
		t.Error("no client must be handed to resources when the token is unknown")
	}
}

func TestProviderConfigure_unknownCustomerIDProducesUnknownDiagnostic(t *testing.T) {
	clearNodePingEnv(t)

	ctx := context.Background()
	resp := configure(t, ctx, configAttrs{
		"api_token":   "token",
		"customer_id": unknownValue,
	})

	if !hasErrorSummary(t, resp, "Unknown NodePing Customer ID") {
		t.Errorf("expected an Unknown NodePing Customer ID diagnostic, got %v", resp.Diagnostics)
	}
}

func TestProviderConfigure_unknownAPIURLProducesUnknownDiagnostic(t *testing.T) {
	clearNodePingEnv(t)

	ctx := context.Background()
	resp := configure(t, ctx, configAttrs{
		"api_token": "token",
		"api_url":   unknownValue,
	})

	if !hasErrorSummary(t, resp, "Unknown NodePing API URL") {
		t.Errorf("expected an Unknown NodePing API URL diagnostic, got %v", resp.Diagnostics)
	}
}

func TestProviderConfigure_unknownNumericAttributesFallBackToDefaults(t *testing.T) {
	clearNodePingEnv(t)

	ctx := context.Background()
	resp := configure(t, ctx, configAttrs{
		"api_token":      "token",
		"rate_limit":     unknownValue,
		"max_retries":    unknownValue,
		"retry_wait_min": unknownValue,
		"retry_wait_max": unknownValue,
	})

	configuredClient(t, resp)
}

func TestProviderConfigure_unknownDefaultTagsDoesNotPanic(t *testing.T) {
	clearNodePingEnv(t)

	ctx := context.Background()
	resp := configure(t, ctx, configAttrs{
		"api_token":    "token",
		"default_tags": unknownValue,
	})

	c := configuredClient(t, resp)
	if got := c.GetDefaultTags(); len(got) != 0 {
		t.Errorf("expected no default tags for an unknown list, got %v", got)
	}
}

func TestProviderConfigure_defaultTags(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
		want  []string
	}{
		{
			name:  "null list yields no tags",
			value: nil,
			want:  nil,
		},
		{
			name:  "empty list yields no tags",
			value: []tftypes.Value{},
			want:  []string{},
		},
		{
			name: "single tag",
			value: []tftypes.Value{
				tftypes.NewValue(tftypes.String, "managed-by-terraform"),
			},
			want: []string{"managed-by-terraform"},
		},
		{
			name: "multiple tags preserve order",
			value: []tftypes.Value{
				tftypes.NewValue(tftypes.String, "team-devops"),
				tftypes.NewValue(tftypes.String, "production"),
			},
			want: []string{"team-devops", "production"},
		},
		{
			name: "empty string tag is preserved verbatim",
			value: []tftypes.Value{
				tftypes.NewValue(tftypes.String, ""),
			},
			want: []string{""},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearNodePingEnv(t)

			attrs := configAttrs{"api_token": "token"}
			if tc.value != nil {
				attrs["default_tags"] = tc.value
			}

			c := configuredClient(t, configure(t, context.Background(), attrs))

			got := c.GetDefaultTags()
			if len(got) != len(tc.want) {
				t.Fatalf("expected default tags %v, got %v", tc.want, got)
			}

			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("default tag %d: expected %q, got %q", i, tc.want[i], got[i])
				}
			}
		})
	}
}

func TestProviderConfigure_defaultTagsWithNullElementFails(t *testing.T) {
	clearNodePingEnv(t)

	ctx := context.Background()
	resp := configure(t, ctx, configAttrs{
		"api_token": "token",
		"default_tags": []tftypes.Value{
			tftypes.NewValue(tftypes.String, nil),
		},
	})

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic for a null element in default_tags")
	}

	if resp.ResourceData != nil {
		t.Error("no client must be handed to resources when default_tags cannot be parsed")
	}
}

func TestProviderConfigure_tuningAttributes(t *testing.T) {
	tests := []struct {
		name  string
		attrs configAttrs
	}{
		{
			name:  "defaults when unset",
			attrs: configAttrs{"api_token": "token"},
		},
		{
			name: "explicit overrides",
			attrs: configAttrs{
				"api_token":      "token",
				"rate_limit":     5.5,
				"max_retries":    int64(7),
				"retry_wait_min": int64(2),
				"retry_wait_max": int64(60),
			},
		},
		{
			name: "zero values fall back to client defaults",
			attrs: configAttrs{
				"api_token":      "token",
				"rate_limit":     0.0,
				"max_retries":    int64(0),
				"retry_wait_min": int64(0),
				"retry_wait_max": int64(0),
			},
		},
		{
			name: "negative values fall back to client defaults",
			attrs: configAttrs{
				"api_token":      "token",
				"rate_limit":     -1.0,
				"max_retries":    int64(-5),
				"retry_wait_min": int64(-1),
				"retry_wait_max": int64(-30),
			},
		},
		{
			name: "retry_wait_min greater than retry_wait_max is accepted",
			attrs: configAttrs{
				"api_token":      "token",
				"retry_wait_min": int64(90),
				"retry_wait_max": int64(1),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearNodePingEnv(t)

			configuredClient(t, configure(t, context.Background(), tc.attrs))
		})
	}
}

// TestProviderConfigure_tokenNeverLogged is a security regression guard: the
// API token must not reach the provider log stream in any field or message.
func TestProviderConfigure_tokenNeverLogged(t *testing.T) {
	clearNodePingEnv(t)

	const secret = "super-secret-nodeping-token-value"

	var buf bytes.Buffer
	ctx := tflogtest.RootLogger(context.Background(), &buf)

	p := New("test")().(*NodePingProvider)
	req := fwprovider.ConfigureRequest{
		Config: newProviderConfig(t, ctx, configAttrs{
			"api_token":   secret,
			"customer_id": "CUST123",
			"api_url":     "https://api.example.com/api/1",
		}),
	}
	resp := fwprovider.ConfigureResponse{}
	p.Configure(ctx, req, &resp)

	configuredClient(t, resp)

	if strings.Contains(buf.String(), secret) {
		t.Fatalf("SECURITY: the API token was written to the provider log stream:\n%s", buf.String())
	}

	entries, err := tflogtest.MultilineJSONDecode(&buf)
	if err != nil {
		t.Fatalf("failed to decode log output: %v", err)
	}

	if len(entries) == 0 {
		t.Fatal("expected Configure to emit log entries")
	}

	forbiddenFields := []string{"api_token", "apiToken", "token", "api_key", "apiKey"}
	for _, entry := range entries {
		for _, field := range forbiddenFields {
			if _, ok := entry[field]; ok {
				t.Errorf("SECURITY: log entry contains forbidden field %q: %v", field, entry)
			}
		}

		for key, value := range entry {
			s, ok := value.(string)
			if ok && strings.Contains(s, secret) {
				t.Errorf("SECURITY: log field %q contains the API token", key)
			}
		}
	}
}

func TestProviderResources_registrationIsCompleteAndDistinct(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	p := New("test")().(*NodePingProvider)

	constructors := p.Resources(ctx)
	if len(constructors) != 7 {
		t.Fatalf("expected 7 registered resources, got %d", len(constructors))
	}

	seen := map[string]bool{}
	for i, newResource := range constructors {
		if newResource == nil {
			t.Fatalf("resource constructor at index %d is nil", i)
		}

		r := newResource()
		if r == nil {
			t.Fatalf("resource constructor at index %d returned nil", i)
		}

		metaResp := resource.MetadataResponse{}
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "nodeping"}, &metaResp)

		if metaResp.TypeName == "" {
			t.Errorf("resource at index %d has an empty type name", i)
			continue
		}

		if !strings.HasPrefix(metaResp.TypeName, "nodeping_") {
			t.Errorf("resource type name %q must be prefixed with the provider type name", metaResp.TypeName)
		}

		if seen[metaResp.TypeName] {
			t.Errorf("resource type name %q is registered more than once", metaResp.TypeName)
		}
		seen[metaResp.TypeName] = true
	}

	for _, want := range []string{
		"nodeping_check",
		"nodeping_contact",
		"nodeping_contactgroup",
		"nodeping_maintenance",
		"nodeping_notificationprofile",
		"nodeping_schedule",
		"nodeping_subaccount",
	} {
		if !seen[want] {
			t.Errorf("expected resource %q to be registered; registered: %v", want, seen)
		}
	}
}

func TestProviderDataSources_registrationIsCompleteAndDistinct(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	p := New("test")().(*NodePingProvider)

	constructors := p.DataSources(ctx)
	if len(constructors) != 14 {
		t.Fatalf("expected 14 registered data sources, got %d", len(constructors))
	}

	seen := map[string]bool{}
	for i, newDataSource := range constructors {
		if newDataSource == nil {
			t.Fatalf("data source constructor at index %d is nil", i)
		}

		d := newDataSource()
		if d == nil {
			t.Fatalf("data source constructor at index %d returned nil", i)
		}

		metaResp := datasource.MetadataResponse{}
		d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "nodeping"}, &metaResp)

		if metaResp.TypeName == "" {
			t.Errorf("data source at index %d has an empty type name", i)
			continue
		}

		if !strings.HasPrefix(metaResp.TypeName, "nodeping_") {
			t.Errorf("data source type name %q must be prefixed with the provider type name", metaResp.TypeName)
		}

		if seen[metaResp.TypeName] {
			t.Errorf("data source type name %q is registered more than once", metaResp.TypeName)
		}
		seen[metaResp.TypeName] = true
	}

	if len(seen) != 14 {
		t.Errorf("expected 14 distinct data source type names, got %d: %v", len(seen), seen)
	}
}

func TestNew_returnsDistinctProviderInstances(t *testing.T) {
	t.Parallel()

	factory := New("9.9.9")

	first, ok := factory().(*NodePingProvider)
	if !ok {
		t.Fatal("factory did not return a *NodePingProvider")
	}

	second := factory().(*NodePingProvider)
	if first == second {
		t.Error("the provider factory must return a fresh instance on each call")
	}

	if first.version != "9.9.9" {
		t.Errorf("expected version %q, got %q", "9.9.9", first.version)
	}
}

// capturedRequest records how the configured client authenticated and addressed
// the API, which is the only externally observable proof of which configuration
// source won.
type capturedRequest struct {
	mu        sync.Mutex
	hits      int
	username  string
	userAgent string
	query     url.Values
}

func (c *capturedRequest) record(r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.hits++
	c.username, _, _ = r.BasicAuth()
	c.userAgent = r.UserAgent()
	c.query = r.URL.Query()
}

func (c *capturedRequest) snapshot() capturedRequest {
	c.mu.Lock()
	defer c.mu.Unlock()

	return capturedRequest{hits: c.hits, username: c.username, userAgent: c.userAgent, query: c.query}
}

// newCaptureServer returns a stub NodePing API that records the first request.
func newCaptureServer(t *testing.T) (string, *capturedRequest) {
	t.Helper()

	captured := &capturedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.record(r)
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{}`)); err != nil {
			t.Errorf("failed to write stub response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	return srv.URL, captured
}

// exercise makes one API call with the configured client so the stub server can
// observe the credentials the provider actually installed.
func exercise(t *testing.T, c *client.Client) {
	t.Helper()

	if _, err := c.ListChecks(context.Background()); err != nil {
		t.Fatalf("unexpected API error: %v", err)
	}
}

func TestProviderConfigure_apiTokenPrecedence(t *testing.T) {
	tests := []struct {
		name        string
		envToken    string
		envAPIKey   string
		configToken interface{}
		want        string
	}{
		{
			name:        "provider block beats NODEPING_API_TOKEN",
			envToken:    "token-from-env",
			configToken: "token-from-config",
			want:        "token-from-config",
		},
		{
			name:     "NODEPING_API_TOKEN alone is used",
			envToken: "token-from-env",
			want:     "token-from-env",
		},
		{
			name:      "NODEPING_API_KEY is honoured as a fallback",
			envAPIKey: "legacy-token",
			want:      "legacy-token",
		},
		{
			name:      "NODEPING_API_TOKEN beats NODEPING_API_KEY",
			envToken:  "primary-token",
			envAPIKey: "legacy-token",
			want:      "primary-token",
		},
		{
			name:        "provider block beats both environment variables",
			envToken:    "token-from-env",
			envAPIKey:   "legacy-token",
			configToken: "token-from-config",
			want:        "token-from-config",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearNodePingEnv(t)

			serverURL, captured := newCaptureServer(t)

			if tc.envToken != "" {
				t.Setenv("NODEPING_API_TOKEN", tc.envToken)
			}
			if tc.envAPIKey != "" {
				t.Setenv("NODEPING_API_KEY", tc.envAPIKey)
			}

			attrs := configAttrs{"api_url": serverURL}
			if tc.configToken != nil {
				attrs["api_token"] = tc.configToken
			}

			c := configuredClient(t, configure(t, context.Background(), attrs))
			exercise(t, c)

			got := captured.snapshot()
			if got.username != tc.want {
				t.Errorf("expected the client to authenticate with %q, got %q", tc.want, got.username)
			}
		})
	}
}

func TestProviderConfigure_customerIDPrecedence(t *testing.T) {
	tests := []struct {
		name       string
		env        string
		configerID interface{}
		want       string
	}{
		{
			name:       "provider block beats NODEPING_CUSTOMER_ID",
			env:        "env-customer",
			configerID: "config-customer",
			want:       "config-customer",
		},
		{
			name: "NODEPING_CUSTOMER_ID alone is used",
			env:  "env-customer",
			want: "env-customer",
		},
		{
			name: "unset customer_id sends no customerid parameter",
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearNodePingEnv(t)

			serverURL, captured := newCaptureServer(t)

			if tc.env != "" {
				t.Setenv("NODEPING_CUSTOMER_ID", tc.env)
			}

			attrs := configAttrs{"api_token": "token", "api_url": serverURL}
			if tc.configerID != nil {
				attrs["customer_id"] = tc.configerID
			}

			c := configuredClient(t, configure(t, context.Background(), attrs))
			exercise(t, c)

			got := captured.snapshot()
			if got.query.Get("customerid") != tc.want {
				t.Errorf("expected customerid %q, got %q", tc.want, got.query.Get("customerid"))
			}
		})
	}
}

func TestProviderConfigure_apiURLFromBlockBeatsEnvironment(t *testing.T) {
	clearNodePingEnv(t)

	configURL, configCaptured := newCaptureServer(t)
	envURL, envCaptured := newCaptureServer(t)

	t.Setenv("NODEPING_API_URL", envURL)

	c := configuredClient(t, configure(t, context.Background(), configAttrs{
		"api_token": "token",
		"api_url":   configURL,
	}))
	exercise(t, c)

	if configCaptured.snapshot().hits != 1 {
		t.Error("expected the request to go to the api_url from the provider block")
	}

	if envCaptured.snapshot().hits != 0 {
		t.Error("NODEPING_API_URL must not win over an explicit api_url")
	}
}

func TestProviderConfigure_apiURLFromEnvironmentAlone(t *testing.T) {
	clearNodePingEnv(t)

	envURL, envCaptured := newCaptureServer(t)
	t.Setenv("NODEPING_API_URL", envURL)

	c := configuredClient(t, configure(t, context.Background(), configAttrs{"api_token": "token"}))
	exercise(t, c)

	if envCaptured.snapshot().hits != 1 {
		t.Error("expected NODEPING_API_URL to be used when api_url is unset")
	}
}

func TestProviderConfigure_userAgentCarriesProviderVersion(t *testing.T) {
	clearNodePingEnv(t)

	serverURL, captured := newCaptureServer(t)

	p := New("4.5.6")().(*NodePingProvider)
	req := fwprovider.ConfigureRequest{
		Config: newProviderConfig(t, context.Background(), configAttrs{
			"api_token": "token",
			"api_url":   serverURL,
		}),
	}
	resp := fwprovider.ConfigureResponse{}
	p.Configure(context.Background(), req, &resp)

	exercise(t, configuredClient(t, resp))

	if got := captured.snapshot().userAgent; got != "terraform-provider-nodeping/4.5.6" {
		t.Errorf("expected the provider version in the User-Agent, got %q", got)
	}
}

// TestProviderConfigure_malformedConfigIsRejected covers the guard after the
// config is decoded: a config whose raw value does not match the schema must
// surface diagnostics and stop, not panic or hand out a half-built client.
func TestProviderConfigure_malformedConfigIsRejected(t *testing.T) {
	clearNodePingEnv(t)
	t.Setenv("NODEPING_API_TOKEN", "token-from-env")

	ctx := context.Background()
	s := providerSchema(t, ctx).Schema
	objType := s.Type().TerraformType(ctx).(tftypes.Object)

	brokenAttrTypes := map[string]tftypes.Type{}
	brokenValues := map[string]tftypes.Value{}
	for name, attrType := range objType.AttributeTypes {
		if name == "default_tags" {
			// A string where the schema declares a list of strings.
			brokenAttrTypes[name] = tftypes.String
			brokenValues[name] = tftypes.NewValue(tftypes.String, "not-a-list")
			continue
		}

		brokenAttrTypes[name] = attrType
		brokenValues[name] = tftypes.NewValue(attrType, nil)
	}

	brokenType := tftypes.Object{AttributeTypes: brokenAttrTypes}

	p := New("test")().(*NodePingProvider)
	req := fwprovider.ConfigureRequest{
		Config: tfsdk.Config{Schema: s, Raw: tftypes.NewValue(brokenType, brokenValues)},
	}
	resp := fwprovider.ConfigureResponse{}
	p.Configure(ctx, req, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected diagnostics for a config that does not match the schema")
	}

	if resp.ResourceData != nil || resp.DataSourceData != nil {
		t.Error("no client must be handed out when the config could not be decoded")
	}
}
