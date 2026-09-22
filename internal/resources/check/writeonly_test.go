package check

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/client"
)

// ---------------------------------------------------------------------------
// schema shape
// ---------------------------------------------------------------------------

func TestPasswordWO_isWriteOnlyAndNotComputed(t *testing.T) {
	s := CheckSchema()

	attribute, ok := s.Attributes["password_wo"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("password_wo is not a string attribute, got %T", s.Attributes["password_wo"])
	}
	if !attribute.WriteOnly {
		t.Error("password_wo must be WriteOnly so the value never reaches state")
	}
	if attribute.Computed {
		t.Error("password_wo must not be Computed; the framework forbids it on write-only attributes")
	}
	if !attribute.Optional || !attribute.Sensitive {
		t.Error("password_wo must be Optional and Sensitive")
	}

	version, ok := s.Attributes["password_wo_version"].(schema.Int64Attribute)
	if !ok {
		t.Fatalf("password_wo_version is not an int64 attribute, got %T", s.Attributes["password_wo_version"])
	}
	if version.WriteOnly {
		t.Error("password_wo_version must be visible to the plan differ; it is the update trigger")
	}
	if !version.Optional {
		t.Error("password_wo_version must be Optional")
	}
}

func TestPassword_isDeprecatedInFavourOfPasswordWO(t *testing.T) {
	attribute, ok := CheckSchema().Attributes["password"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("password is not a string attribute, got %T", CheckSchema().Attributes["password"])
	}
	if attribute.DeprecationMessage == "" {
		t.Fatal("password must carry a deprecation message")
	}
	if !strings.Contains(attribute.DeprecationMessage, "password_wo") {
		t.Errorf("the deprecation message should point at password_wo, got: %q", attribute.DeprecationMessage)
	}
}

func TestConfigValidators_declaresPasswordConflict(t *testing.T) {
	r := &CheckResource{}
	validators := r.ConfigValidators(testContext())
	if len(validators) != 1 {
		t.Fatalf("expected exactly one config validator, got %d", len(validators))
	}
	description := validators[0].Description(testContext())
	for _, want := range []string{"password", "password_wo"} {
		if !strings.Contains(description, want) {
			t.Errorf("config validator description should mention %s, got: %q", want, description)
		}
	}
}

// ---------------------------------------------------------------------------
// write-only plumbing
// ---------------------------------------------------------------------------

func TestWriteOnlyPassword_readsFromConfigNotPlan(t *testing.T) {
	ctx := testContext()

	model := baseModel()
	model.PasswordWO = types.StringValue("s3cret")

	config := newConfig(t, ctx, model)

	var diags diag.Diagnostics
	got := writeOnlyPassword(ctx, config, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got.ValueString() != "s3cret" {
		t.Fatalf("expected the configured password, got %q", got.ValueString())
	}
}

func TestBuildCreateRequest_prefersWriteOnlyPassword(t *testing.T) {
	ctx := testContext()
	r := &CheckResource{}
	var diags diag.Diagnostics

	plan := baseModel()
	plan.Password = types.StringValue("legacy")

	req := r.buildCreateRequest(ctx, &plan, types.StringValue("write-only"), nil, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if req.Password != "write-only" {
		t.Errorf("password_wo should win over the deprecated password, got %q", req.Password)
	}

	plan.Password = types.StringValue("legacy")
	req = r.buildCreateRequest(ctx, &plan, types.StringNull(), nil, &diags)
	if req.Password != "legacy" {
		t.Errorf("the deprecated password must still be sent when password_wo is unset, got %q", req.Password)
	}
}

func TestCreate_writeOnlyPasswordIsSentButNotStored(t *testing.T) {
	ctx := testContext()
	api := newMockAPI(t)
	r := api.resource()

	plan := baseModel()
	plan.PasswordWO = types.StringValue("hunter2")
	plan.PasswordWOVersion = types.Int64Value(1)
	plan.Password = types.StringNull()

	resp := &resource.CreateResponse{State: emptyState(ctx)}
	r.Create(ctx, newCreateRequest(t, ctx, plan), resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create failed: %v", resp.Diagnostics)
	}

	if got := api.lastBody["password"]; got != "hunter2" {
		t.Fatalf("expected the write-only password to reach the API, got %v", got)
	}

	got := stateModel(t, ctx, resp.State)
	if !got.PasswordWO.IsNull() {
		t.Errorf("password_wo must be null in state, got %q", got.PasswordWO.ValueString())
	}
	if !got.Password.IsNull() {
		t.Errorf("password must stay null when only password_wo was configured, got %q", got.Password.ValueString())
	}
	if got.PasswordWOVersion.ValueInt64() != 1 {
		t.Errorf("password_wo_version must be recorded in state, got %v", got.PasswordWOVersion)
	}
}

func TestUpdate_versionBumpResendsTheWriteOnlyPassword(t *testing.T) {
	ctx := testContext()
	api := newMockAPI(t)
	r := api.resource()

	plan := baseModel()
	plan.PasswordWO = types.StringValue("first")
	plan.PasswordWOVersion = types.Int64Value(1)

	createResp := &resource.CreateResponse{State: emptyState(ctx)}
	r.Create(ctx, newCreateRequest(t, ctx, plan), createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create failed: %v", createResp.Diagnostics)
	}
	state := stateModel(t, ctx, createResp.State)

	updatePlan := state
	updatePlan.PasswordWO = types.StringValue("second")
	updatePlan.PasswordWOVersion = types.Int64Value(2)
	updatePlan.Modified = types.Int64Unknown()
	updatePlan.State = types.Int64Unknown()

	updateResp := &resource.UpdateResponse{State: emptyState(ctx)}
	r.Update(ctx, newUpdateRequest(t, ctx, updatePlan, state), updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("update failed: %v", updateResp.Diagnostics)
	}

	if got := api.lastBody["password"]; got != "second" {
		t.Fatalf("expected the rotated password to reach the API, got %v", got)
	}
	got := stateModel(t, ctx, updateResp.State)
	if !got.PasswordWO.IsNull() {
		t.Errorf("password_wo must stay null in state after update, got %q", got.PasswordWO.ValueString())
	}
	if got.PasswordWOVersion.ValueInt64() != 2 {
		t.Errorf("expected password_wo_version 2, got %v", got.PasswordWOVersion)
	}
}

func TestRead_neverCopiesTheAPIPasswordIntoState(t *testing.T) {
	ctx := testContext()
	var diags diag.Diagnostics

	check := &client.Check{
		ID:   "CHECK-1",
		Type: "MYSQL",
		Parameters: client.CheckParameters{
			Target:   "db.example.com",
			Username: "reader",
			Password: "plaintext-from-nodeping",
		},
	}

	// Import: nothing is known beforehand, so the credential must simply be
	// absent rather than picked up from the response.
	imported := mapForRead(ctx, check, nil, nil, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !imported.Password.IsNull() {
		t.Fatalf("import leaked the API password into state: %q", imported.Password.ValueString())
	}
	if !imported.PasswordWO.IsNull() {
		t.Fatal("password_wo must never be populated from an API response")
	}

	// Refresh: the deprecated attribute keeps whatever state already held.
	prior := &CheckResourceModel{Password: types.StringValue("configured")}
	refreshed := mapForRead(ctx, check, prior, nil, &diags)
	if refreshed.Password.ValueString() != "configured" {
		t.Fatalf("refresh should keep the prior password, got %q", refreshed.Password.ValueString())
	}
}

func TestRead_doesNotPersistRedisHostPasswords(t *testing.T) {
	ctx := testContext()
	var diags diag.Diagnostics

	check := &client.Check{
		ID:   "CHECK-1",
		Type: "REDIS",
		Parameters: client.CheckParameters{
			RedisType: "cluster",
			Hosts: map[string]client.RedisHost{
				"node1": {Host: "redis1.example.com", Port: 6379, Password: "leaked"},
				"node2": {Host: "redis2.example.com"},
			},
		},
	}

	imported := mapForRead(ctx, check, nil, nil, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	hosts := make(map[string]RedisHostModel)
	if d := imported.Hosts.ElementsAs(ctx, &hosts, false); d.HasError() {
		t.Fatalf("could not decode hosts: %v", d)
	}
	if len(hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(hosts))
	}
	if !hosts["node1"].Password.IsNull() {
		t.Errorf("the API host password leaked into state: %q", hosts["node1"].Password.ValueString())
	}
	if hosts["node1"].Host.ValueString() != "redis1.example.com" || hosts["node1"].Port.ValueInt64() != 6379 {
		t.Errorf("host and port must still be read from the API, got %+v", hosts["node1"])
	}
	if !hosts["node2"].Port.IsNull() {
		t.Errorf("an omitted port must map to null, got %v", hosts["node2"].Port)
	}
}

func TestRestoreHostPasswords_perKey(t *testing.T) {
	ctx := testContext()
	var diags diag.Diagnostics

	prior := hostsMap(t, ctx, map[string]RedisHostModel{
		"node1": {Host: types.StringValue("redis1"), Port: types.Int64Value(6379), Password: types.StringValue("kept")},
		"gone":  {Host: types.StringValue("redis9"), Port: types.Int64Null(), Password: types.StringValue("stale")},
	})
	current := hostsMap(t, ctx, map[string]RedisHostModel{
		"node1": {Host: types.StringValue("redis1"), Port: types.Int64Value(6379), Password: types.StringNull()},
		"node2": {Host: types.StringValue("redis2"), Port: types.Int64Null(), Password: types.StringNull()},
	})

	merged := restoreHostPasswords(ctx, prior, current, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	hosts := make(map[string]RedisHostModel)
	if d := merged.ElementsAs(ctx, &hosts, false); d.HasError() {
		t.Fatalf("could not decode hosts: %v", d)
	}
	if len(hosts) != 2 {
		t.Fatalf("the API is the source of truth for which hosts exist, got %d", len(hosts))
	}
	if hosts["node1"].Password.ValueString() != "kept" {
		t.Errorf("expected the prior password to be restored, got %v", hosts["node1"].Password)
	}
	if !hosts["node2"].Password.IsNull() {
		t.Errorf("a host with no prior password must stay null, got %v", hosts["node2"].Password)
	}
	if _, ok := hosts["gone"]; ok {
		t.Error("a host the API no longer reports must be dropped")
	}

	// Null on either side is a no-op.
	if got := restoreHostPasswords(ctx, types.MapNull(RedisHostObjectType()), current, &diags); !got.Equal(current) {
		t.Error("a null prior must leave the current value untouched")
	}
	nullCurrent := types.MapNull(RedisHostObjectType())
	if got := restoreHostPasswords(ctx, prior, nullCurrent, &diags); !got.Equal(nullCurrent) {
		t.Error("a null current must be returned unchanged")
	}
}

func hostsMap(t *testing.T, ctx context.Context, hosts map[string]RedisHostModel) types.Map {
	t.Helper()
	value, diags := types.MapValueFrom(ctx, RedisHostObjectType(), hosts)
	if diags.HasError() {
		t.Fatalf("could not build hosts map: %v", diags)
	}
	return value
}

// ---------------------------------------------------------------------------
// validation
// ---------------------------------------------------------------------------

func TestValidateConfig_sentinelRequiresSentinelName(t *testing.T) {
	ctx := testContext()
	r := &CheckResource{}

	config := baseModel()
	config.Type = types.StringValue("REDIS")
	config.RedisType = types.StringValue("sentinel")
	config.SentinelName = types.StringNull()

	resp := &resource.ValidateConfigResponse{}
	r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: newConfig(t, ctx, config)}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when redistype is sentinel and sentinelname is unset")
	}
	if !hasAttributeError(resp.Diagnostics, path.Root("sentinelname")) {
		t.Errorf("the error should be attached to sentinelname, got: %v", resp.Diagnostics)
	}

	config.SentinelName = types.StringValue("mymaster")
	resp = &resource.ValidateConfigResponse{}
	r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: newConfig(t, ctx, config)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("a sentinel check with a master name is valid: %v", resp.Diagnostics)
	}
}

func TestValidateConfig_versionWithoutWriteOnlyPassword(t *testing.T) {
	ctx := testContext()
	r := &CheckResource{}

	config := baseModel()
	config.PasswordWO = types.StringNull()
	config.PasswordWOVersion = types.Int64Value(3)

	resp := &resource.ValidateConfigResponse{}
	r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: newConfig(t, ctx, config)}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("password_wo_version without password_wo must be rejected")
	}
	if !hasAttributeError(resp.Diagnostics, path.Root("password_wo_version")) {
		t.Errorf("the error should be attached to password_wo_version, got: %v", resp.Diagnostics)
	}
}

func hasAttributeError(diags diag.Diagnostics, p path.Path) bool {
	for _, d := range diags.Errors() {
		if withPath, ok := d.(diag.DiagnosticWithPath); ok && withPath.Path().Equal(p) {
			return true
		}
	}
	return false
}
