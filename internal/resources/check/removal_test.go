package check

import (
	"context"
	"sort"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// optionalOnlyAttributes is the set of check attributes that must stay
// Optional-only. NodePing never supplies a value for any of them: each is
// content the practitioner wrote, echoed back verbatim or omitted entirely.
// Marking them Computed would make Terraform substitute prior state whenever
// the attribute is deleted from configuration, so the removal would silently
// never reach the API.
var optionalOnlyAttributes = map[string]bool{
	"dep":            true,
	"description":    true,
	"contentstring":  true,
	"sendheaders":    true,
	"receiveheaders": true,
	"postdata":       true,
	"username":       true,
	"password":       true,
	"dnstoresolve":   true,
	"servername":     true,
	"email":          true,
	"database":       true,
	"query":          true,
	"namespace":      true,
	"sshkey":         true,
	"clientcert":     true,
	"data":           true,
	"fields":         true,
	"hosts":          true,
	"sentinelname":   true,
	"edns":           true,
	"whoisserver":    true,
	"rdapurl":        true,
	"ignore":         true,
}

// removableAttribute describes one attribute the practitioner can delete from
// configuration, together with how to set it, how to read it back, and the key
// it occupies in a NodePing request body.
type removableAttribute struct {
	name    string
	apiKey  string
	set     func(*CheckResourceModel)
	get     func(CheckResourceModel) attr.Value
	wantAPI interface{}
}

func stringMap(t *testing.T, elements map[string]string) types.Map {
	t.Helper()
	value, diags := types.MapValueFrom(context.Background(), types.StringType, elements)
	if diags.HasError() {
		t.Fatalf("could not build map: %v", diags)
	}
	return value
}

func removableAttributes(t *testing.T) []removableAttribute {
	t.Helper()

	fields, diags := types.MapValueFrom(context.Background(), CheckFieldObjectType(), map[string]CheckFieldModel{
		"f1": {Name: types.StringValue("load"), Min: types.Int64Null(), Max: types.Int64Null(), Match: types.StringNull()},
	})
	if diags.HasError() {
		t.Fatalf("could not build fields: %v", diags)
	}
	hosts, diags := types.MapValueFrom(context.Background(), RedisHostObjectType(), map[string]RedisHostModel{
		"node1": {
			Host:              types.StringValue("redis1.example.com"),
			Port:              types.Int64Value(6379),
			Password:          types.StringNull(),
			PasswordWO:        types.StringNull(),
			PasswordWOVersion: types.Int64Null(),
		},
	})
	if diags.HasError() {
		t.Fatalf("could not build hosts: %v", diags)
	}

	str := func(name, apiKey, value string, set func(*CheckResourceModel, types.String), get func(CheckResourceModel) types.String) removableAttribute {
		return removableAttribute{
			name:    name,
			apiKey:  apiKey,
			wantAPI: value,
			set:     func(m *CheckResourceModel) { set(m, types.StringValue(value)) },
			get:     func(m CheckResourceModel) attr.Value { return get(m) },
		}
	}

	return []removableAttribute{
		str("dep", "dep", "CHK-DEP",
			func(m *CheckResourceModel, v types.String) { m.Dep = v },
			func(m CheckResourceModel) types.String { return m.Dep }),
		str("description", "description", "notes",
			func(m *CheckResourceModel, v types.String) { m.Description = v },
			func(m CheckResourceModel) types.String { return m.Description }),
		str("contentstring", "contentstring", "ok",
			func(m *CheckResourceModel, v types.String) { m.ContentString = v },
			func(m CheckResourceModel) types.String { return m.ContentString }),
		str("postdata", "postdata", "a=b",
			func(m *CheckResourceModel, v types.String) { m.PostData = v },
			func(m CheckResourceModel) types.String { return m.PostData }),
		str("username", "username", "reader",
			func(m *CheckResourceModel, v types.String) { m.Username = v },
			func(m CheckResourceModel) types.String { return m.Username }),
		str("password", "password", "hunter2",
			func(m *CheckResourceModel, v types.String) { m.Password = v },
			func(m CheckResourceModel) types.String { return m.Password }),
		str("dnstoresolve", "dnstoresolve", "example.com",
			func(m *CheckResourceModel, v types.String) { m.DNSToResolve = v },
			func(m CheckResourceModel) types.String { return m.DNSToResolve }),
		str("servername", "servername", "example.com",
			func(m *CheckResourceModel, v types.String) { m.ServerName = v },
			func(m CheckResourceModel) types.String { return m.ServerName }),
		str("email", "email", "a@example.com",
			func(m *CheckResourceModel, v types.String) { m.Email = v },
			func(m CheckResourceModel) types.String { return m.Email }),
		str("database", "database", "db",
			func(m *CheckResourceModel, v types.String) { m.Database = v },
			func(m CheckResourceModel) types.String { return m.Database }),
		str("query", "query", "select 1",
			func(m *CheckResourceModel, v types.String) { m.Query = v },
			func(m CheckResourceModel) types.String { return m.Query }),
		str("namespace", "namespace", "ns",
			func(m *CheckResourceModel, v types.String) { m.Namespace = v },
			func(m CheckResourceModel) types.String { return m.Namespace }),
		str("sshkey", "sshkey", "KEY-1",
			func(m *CheckResourceModel, v types.String) { m.SSHKey = v },
			func(m CheckResourceModel) types.String { return m.SSHKey }),
		str("clientcert", "clientcert", "CERT-1",
			func(m *CheckResourceModel, v types.String) { m.ClientCert = v },
			func(m CheckResourceModel) types.String { return m.ClientCert }),
		str("sentinelname", "sentinelname", "mymaster",
			func(m *CheckResourceModel, v types.String) { m.SentinelName = v },
			func(m CheckResourceModel) types.String { return m.SentinelName }),
		str("whoisserver", "whoisserver", "whois.example.com",
			func(m *CheckResourceModel, v types.String) { m.WhoisServer = v },
			func(m CheckResourceModel) types.String { return m.WhoisServer }),
		str("rdapurl", "rdapurl", "https://rdap.example.com",
			func(m *CheckResourceModel, v types.String) { m.RDAPUrl = v },
			func(m CheckResourceModel) types.String { return m.RDAPUrl }),
		str("ignore", "ignore", "zen.example.com",
			func(m *CheckResourceModel, v types.String) { m.Ignore = v },
			func(m CheckResourceModel) types.String { return m.Ignore }),
		{
			name:    "sendheaders",
			apiKey:  "sendheaders",
			wantAPI: map[string]interface{}{"Authorization": "Bearer x"},
			set: func(m *CheckResourceModel) {
				m.SendHeaders = stringMap(t, map[string]string{"Authorization": "Bearer x"})
			},
			get: func(m CheckResourceModel) attr.Value { return m.SendHeaders },
		},
		{
			name:    "receiveheaders",
			apiKey:  "receiveheaders",
			wantAPI: map[string]interface{}{"Server": "nginx"},
			set:     func(m *CheckResourceModel) { m.ReceiveHeaders = stringMap(t, map[string]string{"Server": "nginx"}) },
			get:     func(m CheckResourceModel) attr.Value { return m.ReceiveHeaders },
		},
		{
			name:    "data",
			apiKey:  "data",
			wantAPI: map[string]interface{}{"CHK-2": "1"},
			set:     func(m *CheckResourceModel) { m.Data = stringMap(t, map[string]string{"CHK-2": "1"}) },
			get:     func(m CheckResourceModel) attr.Value { return m.Data },
		},
		{
			name:   "edns",
			apiKey: "edns",
			// The mock echoes the request body, so the expected value is the
			// JSON shape the client sends rather than the model's.
			wantAPI: map[string]interface{}{"8": "0"},
			set:     func(m *CheckResourceModel) { m.EDNS = stringMap(t, map[string]string{"8": "0"}) },
			get:     func(m CheckResourceModel) attr.Value { return m.EDNS },
		},
		{
			name:   "fields",
			apiKey: "fields",
			set:    func(m *CheckResourceModel) { m.Fields = fields },
			get:    func(m CheckResourceModel) attr.Value { return m.Fields },
		},
		{
			name:   "hosts",
			apiKey: "hosts",
			set:    func(m *CheckResourceModel) { m.Hosts = hosts },
			get:    func(m CheckResourceModel) attr.Value { return m.Hosts },
		},
	}
}

// TestOptionalOnlyAttributes_areCoveredByARemovalTest keeps the table and the
// schema policy in step: a newly demoted attribute without a removal test would
// otherwise pass unnoticed.
func TestOptionalOnlyAttributes_areCoveredByARemovalTest(t *testing.T) {
	covered := map[string]bool{}
	for _, a := range removableAttributes(t) {
		covered[a.name] = true
	}

	missing := []string{}
	for name := range optionalOnlyAttributes {
		if !covered[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("Optional-only attributes without a removal test: %v", missing)
	}

	for name := range covered {
		if !optionalOnlyAttributes[name] {
			t.Errorf("%s has a removal test but is not listed as Optional-only", name)
		}
	}
}

// proposedNewValue reproduces the rule Terraform core applies when it builds the
// proposed new state for an attribute the practitioner deleted from
// configuration: a value is carried over from prior state only when the
// attribute is Computed. This is the whole mechanism behind A-007, and encoding
// it here is what makes "removal produces a diff" a testable property of the
// schema rather than a claim about core.
func proposedNewValue(computed bool, prior attr.Value) attr.Value {
	if computed {
		return prior
	}
	return nil
}

func TestRemovingAnOptionalOnlyAttribute_producesADiff(t *testing.T) {
	s := CheckSchema()

	for _, a := range removableAttributes(t) {
		t.Run(a.name, func(t *testing.T) {
			attribute, ok := s.Attributes[a.name]
			if !ok {
				t.Fatalf("%s is not a schema attribute", a.name)
			}

			model := baseModel()
			a.set(&model)
			prior := a.get(model)
			if prior.IsNull() {
				t.Fatalf("the fixture for %s must set a value", a.name)
			}

			proposed := proposedNewValue(attribute.IsComputed(), prior)
			if proposed != nil {
				t.Fatalf("deleting %s from configuration proposes the prior value %v, so no diff is produced", a.name, proposed)
			}
		})
	}
}

// TestRemovingAnOptionalOnlyAttribute_clearsTheValue drives the other half:
// once core plans the removal, the apply must actually drop the field from the
// request and leave state null.
func TestRemovingAnOptionalOnlyAttribute_clearsTheValue(t *testing.T) {
	for _, a := range removableAttributes(t) {
		t.Run(a.name, func(t *testing.T) {
			ctx := testContext()
			api := newMockAPI(t)
			r := api.resource()

			plan := baseModel()
			a.set(&plan)

			createResp := &resource.CreateResponse{State: emptyState(ctx)}
			r.Create(ctx, newCreateRequest(t, ctx, plan), createResp)
			if createResp.Diagnostics.HasError() {
				t.Fatalf("create failed: %v", createResp.Diagnostics)
			}
			if _, ok := api.lastBody[a.apiKey]; !ok {
				t.Fatalf("%s was not sent to the API on create", a.apiKey)
			}
			if a.wantAPI != nil {
				if got := api.lastBody[a.apiKey]; !equalJSON(got, a.wantAPI) {
					t.Fatalf("expected %s to be sent as %#v, got %#v", a.apiKey, a.wantAPI, got)
				}
			}

			state := stateModel(t, ctx, createResp.State)
			if a.get(state).IsNull() {
				t.Fatalf("%s should have been recorded in state after create", a.name)
			}

			// The practitioner deletes the attribute: core proposes null, and
			// nothing in the provider may put the prior value back.
			updatePlan := state
			a.set(&updatePlan)
			clearAttribute(t, &updatePlan, a)
			updatePlan.Modified = types.Int64Unknown()
			updatePlan.State = types.Int64Unknown()

			updateResp := &resource.UpdateResponse{State: emptyState(ctx)}
			r.Update(ctx, newUpdateRequest(t, ctx, updatePlan, state), updateResp)
			if updateResp.Diagnostics.HasError() {
				t.Fatalf("update failed: %v", updateResp.Diagnostics)
			}

			if _, ok := api.lastBody[a.apiKey]; ok {
				t.Errorf("%s must be omitted from the update request once it is removed from configuration", a.apiKey)
			}

			got := stateModel(t, ctx, updateResp.State)
			if !a.get(got).IsNull() {
				t.Errorf("%s must be null in state after removal, got %v", a.name, a.get(got))
			}
		})
	}
}

// clearAttribute nulls the attribute under test, preserving the element type
// that collection values must carry even when null.
func clearAttribute(t *testing.T, m *CheckResourceModel, a removableAttribute) {
	t.Helper()
	switch current := a.get(*m).(type) {
	case types.String:
		_ = current
		setNullString(m, a.name)
	case types.Map:
		setNullMap(m, a.name)
	default:
		t.Fatalf("no null form defined for %s (%T)", a.name, current)
	}
}

func setNullString(m *CheckResourceModel, name string) {
	targets := map[string]*types.String{
		"dep":           &m.Dep,
		"description":   &m.Description,
		"contentstring": &m.ContentString,
		"postdata":      &m.PostData,
		"username":      &m.Username,
		"password":      &m.Password,
		"dnstoresolve":  &m.DNSToResolve,
		"servername":    &m.ServerName,
		"email":         &m.Email,
		"database":      &m.Database,
		"query":         &m.Query,
		"namespace":     &m.Namespace,
		"sshkey":        &m.SSHKey,
		"clientcert":    &m.ClientCert,
		"sentinelname":  &m.SentinelName,
		"whoisserver":   &m.WhoisServer,
		"rdapurl":       &m.RDAPUrl,
		"ignore":        &m.Ignore,
	}
	if target, ok := targets[name]; ok {
		*target = types.StringNull()
	}
}

func setNullMap(m *CheckResourceModel, name string) {
	switch name {
	case "sendheaders":
		m.SendHeaders = types.MapNull(types.StringType)
	case "receiveheaders":
		m.ReceiveHeaders = types.MapNull(types.StringType)
	case "data":
		m.Data = types.MapNull(types.StringType)
	case "edns":
		m.EDNS = types.MapNull(types.StringType)
	case "fields":
		m.Fields = types.MapNull(CheckFieldObjectType())
	case "hosts":
		m.Hosts = types.MapNull(RedisHostObjectType())
	}
}

// equalJSON compares two decoded JSON values structurally. The mock records the
// request body exactly as it was decoded, so maps arrive as
// map[string]interface{} regardless of the Go type that produced them.
func equalJSON(got, want interface{}) bool {
	switch wantMap := want.(type) {
	case map[string]interface{}:
		gotMap, ok := got.(map[string]interface{})
		if !ok || len(gotMap) != len(wantMap) {
			return false
		}
		for key, value := range wantMap {
			if !equalJSON(gotMap[key], value) {
				return false
			}
		}
		return true
	default:
		return got == want
	}
}

// ---------------------------------------------------------------------------
// attributes deliberately kept Optional + Computed
// ---------------------------------------------------------------------------

// TestServerDefaultedAttributes_keepPriorStateOnRemoval documents the other
// side of the audit. These attributes have a NodePing side default, so removing
// them from configuration must keep working the way it always has: the prior
// value survives and no spurious diff (or inconsistent-result error) appears.
func TestServerDefaultedAttributes_keepPriorStateOnRemoval(t *testing.T) {
	s := CheckSchema()

	// Documented NodePing defaults, plus the values the API derives.
	serverDefaulted := []string{
		"target", "label", "enabled", "public", "interval", "threshold", "sens",
		"mute", "autodiag", "runlocations", "homeloc", "tags", "regex", "invert",
		"follow", "method", "statuscode", "port", "secure", "verify", "ipv6",
		"dnstype", "dnssection", "dnsrd", "transport", "warningdays", "snmpv",
		"snmpcom", "redistype", "dohdot", "oldresultfail", "verifyvolume",
		"volumemin", "customer_id",
	}

	for _, name := range serverDefaulted {
		attribute, ok := s.Attributes[name]
		if !ok {
			t.Errorf("%s is no longer a schema attribute", name)
			continue
		}
		if !attribute.IsOptional() || !attribute.IsComputed() {
			t.Errorf("%s must stay Optional+Computed: NodePing supplies a value the practitioner never set", name)
		}
		if optionalOnlyAttributes[name] {
			t.Errorf("%s cannot be both server-defaulted and Optional-only", name)
		}
	}
}

// ---------------------------------------------------------------------------
// nested write-only REDIS host password
// ---------------------------------------------------------------------------

func TestHostsPasswordWO_isWriteOnlyAndNotComputed(t *testing.T) {
	s := CheckSchema()

	hosts, ok := s.Attributes["hosts"].(schema.MapNestedAttribute)
	if !ok {
		t.Fatalf("hosts is not a map nested attribute, got %T", s.Attributes["hosts"])
	}
	// The framework rejects a WriteOnly child inside a Computed nested
	// attribute, so the demotion in A-007 is a precondition for this.
	if hosts.Computed {
		t.Fatal("hosts must not be Computed: a Computed nested attribute cannot carry a WriteOnly child")
	}

	attrs := hosts.NestedObject.Attributes

	passwordWO, ok := attrs["password_wo"]
	if !ok {
		t.Fatal("hosts.password_wo is missing")
	}
	if !passwordWO.IsWriteOnly() {
		t.Error("hosts.password_wo must be WriteOnly so the credential never reaches state")
	}
	if passwordWO.IsComputed() {
		t.Error("hosts.password_wo must not be Computed; the framework forbids it on write-only attributes")
	}
	if !passwordWO.IsOptional() || !passwordWO.IsSensitive() {
		t.Error("hosts.password_wo must be Optional and Sensitive")
	}

	version, ok := attrs["password_wo_version"]
	if !ok {
		t.Fatal("hosts.password_wo_version is missing")
	}
	if version.IsWriteOnly() {
		t.Error("hosts.password_wo_version must be visible to the plan differ; it is the update trigger")
	}

	password, ok := attrs["password"]
	if !ok {
		t.Fatal("hosts.password is missing")
	}
	if password.GetDeprecationMessage() == "" {
		t.Error("hosts.password must be deprecated in favour of hosts.password_wo")
	}
}

func TestSchemaValidateImplementation_acceptsNestedWriteOnly(t *testing.T) {
	// This is the load-bearing assertion for A-013: terraform-plugin-framework
	// v1.19.0 validates nested write-only attributes during GetProviderSchema,
	// and rejects the combination outright when the containing MapNestedAttribute
	// is Computed. Running the schema's own validation is the direct proof that
	// the shape we ship is legal.
	ctx := testContext()
	resp := &resource.SchemaResponse{}
	(&CheckResource{}).Schema(ctx, resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema is invalid: %v", resp.Diagnostics)
	}

	diags := resp.Schema.ValidateImplementation(ctx)
	if diags.HasError() {
		t.Fatalf("nested write-only attribute rejected by the framework: %v", diags)
	}

	// The same validation must reject the shape we are not allowed to ship,
	// otherwise the assertion above proves nothing.
	broken := CheckSchema()
	hosts := broken.Attributes["hosts"].(schema.MapNestedAttribute)
	hosts.Computed = true
	broken.Attributes["hosts"] = hosts

	if !broken.ValidateImplementation(ctx).HasError() {
		t.Fatal("a Computed map nested attribute with a WriteOnly child should be rejected")
	}
}

func TestWriteOnlyHostPasswords_readsFromConfigNotPlan(t *testing.T) {
	ctx := testContext()

	model := baseModel()
	model.Hosts = hostsMap(t, ctx, map[string]RedisHostModel{
		"node1": {
			Host:              types.StringValue("redis1.example.com"),
			Port:              types.Int64Value(6379),
			Password:          types.StringNull(),
			PasswordWO:        types.StringValue("s3cret"),
			PasswordWOVersion: types.Int64Value(1),
		},
		"node2": {
			Host:              types.StringValue("redis2.example.com"),
			Port:              types.Int64Null(),
			Password:          types.StringNull(),
			PasswordWO:        types.StringNull(),
			PasswordWOVersion: types.Int64Null(),
		},
	})

	var diags diag.Diagnostics
	got := writeOnlyHostPasswords(ctx, newConfig(t, ctx, model), &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(got) != 1 {
		t.Fatalf("expected exactly one host password, got %d", len(got))
	}
	if got["node1"].ValueString() != "s3cret" {
		t.Errorf("expected the configured host password, got %q", got["node1"].ValueString())
	}

	// The plan never carries a write-only value, which is why the plan model is
	// not an acceptable source.
	planned := withoutWriteOnly(model)
	planned.Hosts = hostsMap(t, ctx, map[string]RedisHostModel{
		"node1": {
			Host:              types.StringValue("redis1.example.com"),
			Port:              types.Int64Value(6379),
			Password:          types.StringNull(),
			PasswordWO:        types.StringNull(),
			PasswordWOVersion: types.Int64Value(1),
		},
	})
	if fromPlan := writeOnlyHostPasswords(ctx, newConfig(t, ctx, planned), &diags); len(fromPlan) != 0 {
		t.Errorf("a plan-shaped value must yield no passwords, got %v", fromPlan)
	}

	// No hosts at all is not an error.
	empty := baseModel()
	if fromEmpty := writeOnlyHostPasswords(ctx, newConfig(t, ctx, empty), &diags); fromEmpty != nil {
		t.Errorf("expected nil for a check with no hosts, got %v", fromEmpty)
	}
}

func TestCreate_writeOnlyHostPasswordIsSentButNotStored(t *testing.T) {
	ctx := testContext()
	api := newMockAPI(t)
	r := api.resource()

	plan := baseModel()
	plan.Type = types.StringValue("REDIS")
	plan.RedisType = types.StringValue("cluster")
	plan.Hosts = hostsMap(t, ctx, map[string]RedisHostModel{
		"node1": {
			Host:              types.StringValue("redis1.example.com"),
			Port:              types.Int64Value(6379),
			Password:          types.StringNull(),
			PasswordWO:        types.StringValue("hunter2"),
			PasswordWOVersion: types.Int64Value(1),
		},
	})

	resp := &resource.CreateResponse{State: emptyState(ctx)}
	r.Create(ctx, newCreateRequest(t, ctx, plan), resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create failed: %v", resp.Diagnostics)
	}

	sent, ok := api.lastBody["hosts"].(map[string]interface{})
	if !ok {
		t.Fatalf("hosts were not sent to the API, got %#v", api.lastBody["hosts"])
	}
	node1, ok := sent["node1"].(map[string]interface{})
	if !ok {
		t.Fatalf("node1 missing from the request body, got %#v", sent)
	}
	if node1["password"] != "hunter2" {
		t.Errorf("expected the write-only host password to reach the API, got %v", node1["password"])
	}

	got := stateModel(t, ctx, resp.State)
	hosts := make(map[string]RedisHostModel)
	if d := got.Hosts.ElementsAs(ctx, &hosts, false); d.HasError() {
		t.Fatalf("could not decode hosts: %v", d)
	}
	if !hosts["node1"].PasswordWO.IsNull() {
		t.Errorf("hosts.password_wo must be null in state, got %q", hosts["node1"].PasswordWO.ValueString())
	}
	if !hosts["node1"].Password.IsNull() {
		t.Errorf("hosts.password must stay null when only password_wo was configured, got %q", hosts["node1"].Password.ValueString())
	}
	if hosts["node1"].PasswordWOVersion.ValueInt64() != 1 {
		t.Errorf("hosts.password_wo_version must be recorded in state, got %v", hosts["node1"].PasswordWOVersion)
	}
}

func TestUpdate_hostVersionBumpResendsTheWriteOnlyPassword(t *testing.T) {
	ctx := testContext()
	api := newMockAPI(t)
	r := api.resource()

	host := func(password string, version int64) types.Map {
		return hostsMap(t, ctx, map[string]RedisHostModel{
			"node1": {
				Host:              types.StringValue("redis1.example.com"),
				Port:              types.Int64Value(6379),
				Password:          types.StringNull(),
				PasswordWO:        types.StringValue(password),
				PasswordWOVersion: types.Int64Value(version),
			},
		})
	}

	plan := baseModel()
	plan.Type = types.StringValue("REDIS")
	plan.Hosts = host("first", 1)

	createResp := &resource.CreateResponse{State: emptyState(ctx)}
	r.Create(ctx, newCreateRequest(t, ctx, plan), createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create failed: %v", createResp.Diagnostics)
	}
	state := stateModel(t, ctx, createResp.State)

	updatePlan := state
	updatePlan.Hosts = host("second", 2)
	updatePlan.Modified = types.Int64Unknown()
	updatePlan.State = types.Int64Unknown()

	updateResp := &resource.UpdateResponse{State: emptyState(ctx)}
	r.Update(ctx, newUpdateRequest(t, ctx, updatePlan, state), updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("update failed: %v", updateResp.Diagnostics)
	}

	sent := api.lastBody["hosts"].(map[string]interface{})["node1"].(map[string]interface{})
	if sent["password"] != "second" {
		t.Errorf("expected the rotated host password to reach the API, got %v", sent["password"])
	}

	got := stateModel(t, ctx, updateResp.State)
	hosts := make(map[string]RedisHostModel)
	if d := got.Hosts.ElementsAs(ctx, &hosts, false); d.HasError() {
		t.Fatalf("could not decode hosts: %v", d)
	}
	if !hosts["node1"].PasswordWO.IsNull() {
		t.Error("hosts.password_wo must stay null in state after update")
	}
	if hosts["node1"].PasswordWOVersion.ValueInt64() != 2 {
		t.Errorf("expected hosts.password_wo_version 2, got %v", hosts["node1"].PasswordWOVersion)
	}
}

func TestBuildCreateRequest_hostPasswordWOWinsOverDeprecatedPassword(t *testing.T) {
	ctx := testContext()
	r := &CheckResource{}
	var diags diag.Diagnostics

	plan := baseModel()
	plan.Type = types.StringValue("REDIS")
	plan.Hosts = hostsMap(t, ctx, map[string]RedisHostModel{
		"node1": {
			Host:              types.StringValue("redis1.example.com"),
			Port:              types.Int64Value(6379),
			Password:          types.StringValue("legacy"),
			PasswordWO:        types.StringNull(),
			PasswordWOVersion: types.Int64Null(),
		},
	})

	req := r.buildCreateRequest(ctx, &plan, types.StringNull(), nil, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if req.Hosts["node1"].Password != "legacy" {
		t.Errorf("the deprecated host password must still be sent, got %q", req.Hosts["node1"].Password)
	}

	req = r.buildCreateRequest(ctx, &plan, types.StringNull(),
		map[string]types.String{"node1": types.StringValue("write-only")}, &diags)
	if req.Hosts["node1"].Password != "write-only" {
		t.Errorf("hosts.password_wo should win over the deprecated password, got %q", req.Hosts["node1"].Password)
	}
}

func TestValidateConfig_hostCredentialRules(t *testing.T) {
	ctx := testContext()
	r := &CheckResource{}

	run := func(host RedisHostModel) *resource.ValidateConfigResponse {
		config := baseModel()
		config.Type = types.StringValue("REDIS")
		config.Hosts = hostsMap(t, ctx, map[string]RedisHostModel{"node1": host})
		resp := &resource.ValidateConfigResponse{}
		r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: newConfig(t, ctx, config)}, resp)
		return resp
	}

	both := run(RedisHostModel{
		Host:              types.StringValue("redis1"),
		Port:              types.Int64Null(),
		Password:          types.StringValue("legacy"),
		PasswordWO:        types.StringValue("wo"),
		PasswordWOVersion: types.Int64Null(),
	})
	if !both.Diagnostics.HasError() {
		t.Error("setting both password and password_wo on a host must be rejected")
	}
	if !hasAttributeError(both.Diagnostics, path.Root("hosts").AtMapKey("node1").AtName("password_wo")) {
		t.Errorf("the conflict error should be attached to the host's password_wo, got: %v", both.Diagnostics)
	}

	orphanVersion := run(RedisHostModel{
		Host:              types.StringValue("redis1"),
		Port:              types.Int64Null(),
		Password:          types.StringNull(),
		PasswordWO:        types.StringNull(),
		PasswordWOVersion: types.Int64Value(3),
	})
	if !orphanVersion.Diagnostics.HasError() {
		t.Error("password_wo_version without password_wo must be rejected on a host too")
	}
	if !hasAttributeError(orphanVersion.Diagnostics, path.Root("hosts").AtMapKey("node1").AtName("password_wo_version")) {
		t.Errorf("the error should be attached to the host's password_wo_version, got: %v", orphanVersion.Diagnostics)
	}

	valid := run(RedisHostModel{
		Host:              types.StringValue("redis1"),
		Port:              types.Int64Value(6379),
		Password:          types.StringNull(),
		PasswordWO:        types.StringValue("wo"),
		PasswordWOVersion: types.Int64Value(1),
	})
	if valid.Diagnostics.HasError() {
		t.Errorf("a host with password_wo and a version is valid: %v", valid.Diagnostics)
	}
}
