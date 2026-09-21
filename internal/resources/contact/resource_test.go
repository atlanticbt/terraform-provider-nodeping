package contact

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/client"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func addrModel(id, typ, address string) AddressModel {
	m := AddressModel{
		Type:          types.StringValue(typ),
		Address:       types.StringValue(address),
		SuppressUp:    types.BoolValue(false),
		SuppressDown:  types.BoolValue(false),
		SuppressFirst: types.BoolValue(false),
		SuppressDiag:  types.BoolValue(false),
		SuppressAll:   types.BoolValue(false),
		Mute:          types.BoolValue(false),
		Action:        types.StringNull(),
		Headers:       types.MapNull(types.StringType),
		QueryStrings:  types.MapNull(types.StringType),
		Data:          NewJSONStringNull(),
		Priority:      types.Int64Null(),
	}
	if id == "" {
		m.ID = types.StringUnknown()
	} else {
		m.ID = types.StringValue(id)
	}
	return m
}

func apiAddr(typ, address string) client.ContactAddress {
	return client.ContactAddress{Type: typ, Address: address}
}

func newTestResource(t *testing.T, handler http.HandlerFunc) (*ContactResource, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c := client.NewClient(client.ClientConfig{
		APIToken:     "test-token",
		BaseURL:      srv.URL,
		RateLimit:    1000,
		MaxRetries:   1,
		RetryMinWait: time.Millisecond,
		RetryMaxWait: time.Millisecond,
		Timeout:      5 * time.Second,
	})

	return &ContactResource{client: c}, srv
}

func emptyPlan(t *testing.T, ctx context.Context, model ContactResourceModel) tfsdk.Plan {
	t.Helper()
	s := ContactSchema()
	p := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	diags := p.Set(ctx, &model)
	if diags.HasError() {
		t.Fatalf("failed to build plan: %v", diags)
	}
	return p
}

func emptyState(t *testing.T, ctx context.Context, model *ContactResourceModel) tfsdk.State {
	t.Helper()
	s := ContactSchema()
	st := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if model != nil {
		diags := st.Set(ctx, model)
		if diags.HasError() {
			t.Fatalf("failed to build state: %v", diags)
		}
	}
	return st
}

func decodeState(t *testing.T, ctx context.Context, st tfsdk.State) ContactResourceModel {
	t.Helper()
	var out ContactResourceModel
	diags := st.Get(ctx, &out)
	if diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	return out
}

func contactJSON(id, customerID, name string, addresses map[string]client.ContactAddress) []byte {
	body, err := json.Marshal(client.Contact{
		ID:         id,
		CustomerID: customerID,
		Name:       name,
		CustRole:   "notify",
		Addresses:  addresses,
	})
	if err != nil {
		panic(err)
	}
	return body
}

// ---------------------------------------------------------------------------
// finding 1 -- deterministic address ordering
// ---------------------------------------------------------------------------

func TestMapAddressesToModel_FollowsPlanOrderDeterministically(t *testing.T) {
	ctx := context.Background()

	// API IDs deliberately sort in the opposite order to the plan.
	api := map[string]client.ContactAddress{
		"E5": apiAddr("email", "one@example.com"),
		"D4": apiAddr("email", "two@example.com"),
		"C3": apiAddr("sms", "+15551230000"),
		"B2": apiAddr("webhook", "https://hooks.example.com/a"),
		"A1": apiAddr("webhook", "https://hooks.example.com/b"),
	}

	plan := []AddressModel{
		addrModel("", "email", "one@example.com"),
		addrModel("", "email", "two@example.com"),
		addrModel("", "sms", "+15551230000"),
		addrModel("", "webhook", "https://hooks.example.com/a"),
		addrModel("", "webhook", "https://hooks.example.com/b"),
	}

	want := []string{"E5", "D4", "C3", "B2", "A1"}

	for i := 0; i < 200; i++ {
		var diags diag.Diagnostics
		got := mapAddressesToModel(ctx, api, plan, &diags)
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if len(got) != len(want) {
			t.Fatalf("got %d addresses, want %d", len(got), len(want))
		}
		for j, id := range want {
			if got[j].ID.ValueString() != id {
				t.Fatalf("iteration %d: address[%d] id = %q, want %q", i, j, got[j].ID.ValueString(), id)
			}
			if got[j].Address.ValueString() != plan[j].Address.ValueString() {
				t.Fatalf("iteration %d: address[%d] = %q, want %q", i, j, got[j].Address.ValueString(), plan[j].Address.ValueString())
			}
		}
	}
}

func TestMapAddressesToModel_ExtraAPIAddressesSortedByID(t *testing.T) {
	ctx := context.Background()

	api := map[string]client.ContactAddress{
		"ZZ": apiAddr("email", "known@example.com"),
		"MM": apiAddr("sms", "+15550000002"),
		"AA": apiAddr("sms", "+15550000001"),
	}

	plan := []AddressModel{addrModel("ZZ", "email", "known@example.com")}

	for i := 0; i < 100; i++ {
		var diags diag.Diagnostics
		got := mapAddressesToModel(ctx, api, plan, &diags)
		ids := []string{got[0].ID.ValueString(), got[1].ID.ValueString(), got[2].ID.ValueString()}
		want := []string{"ZZ", "AA", "MM"}
		for j := range want {
			if ids[j] != want[j] {
				t.Fatalf("iteration %d: ids = %v, want %v", i, ids, want)
			}
		}
	}
}

func TestMapAddressesToModel_Empty(t *testing.T) {
	var diags diag.Diagnostics
	if got := mapAddressesToModel(context.Background(), nil, nil, &diags); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestMapAddressesToModel_NoPlanSortsByID(t *testing.T) {
	ctx := context.Background()
	api := map[string]client.ContactAddress{
		"C": apiAddr("email", "c@example.com"),
		"A": apiAddr("email", "a@example.com"),
		"B": apiAddr("email", "b@example.com"),
	}
	for i := 0; i < 50; i++ {
		var diags diag.Diagnostics
		got := mapAddressesToModel(ctx, api, nil, &diags)
		if got[0].ID.ValueString() != "A" || got[1].ID.ValueString() != "B" || got[2].ID.ValueString() != "C" {
			t.Fatalf("unsorted result on iteration %d", i)
		}
	}
}

func TestAddressToModel_AllFields(t *testing.T) {
	ctx := context.Background()
	priority := 2
	addr := client.ContactAddress{
		Type:          "webhook",
		Address:       "https://hooks.example.com/x",
		SuppressUp:    true,
		SuppressDown:  true,
		SuppressFirst: true,
		SuppressDiag:  true,
		SuppressAll:   true,
		Mute:          json.RawMessage(`true`),
		Action:        "post",
		Headers:       map[string]string{"content-type": "application/json"},
		QueryStrings:  map[string]string{"key": "1"},
		Data:          `{"a":1}`,
		Priority:      &priority,
	}

	var diags diag.Diagnostics
	model := addressToModel(ctx, "ID1", addr, nil, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !model.Mute.ValueBool() || !model.SuppressAll.ValueBool() {
		t.Fatal("expected mute/suppress_all to be true")
	}
	if model.Action.ValueString() != "post" {
		t.Fatalf("action = %q", model.Action.ValueString())
	}
	if model.Priority.ValueInt64() != 2 {
		t.Fatalf("priority = %d", model.Priority.ValueInt64())
	}
	if model.Headers.IsNull() || model.QueryStrings.IsNull() {
		t.Fatal("expected headers and querystrings to be set")
	}

	// numeric mute
	addr.Mute = json.RawMessage(`1700000000`)
	model = addressToModel(ctx, "ID1", addr, nil, &diags)
	if !model.Mute.ValueBool() {
		t.Fatal("expected numeric mute to map to true")
	}

	// unset optional fields
	bare := client.ContactAddress{Type: "email", Address: "a@example.com"}
	model = addressToModel(ctx, "ID2", bare, nil, &diags)
	if !model.Action.IsNull() || !model.Priority.IsNull() || !model.Headers.IsNull() || !model.QueryStrings.IsNull() || !model.Data.IsNull() {
		t.Fatal("expected optional fields to be null")
	}
	if model.Mute.ValueBool() {
		t.Fatal("expected mute false")
	}
}

// ---------------------------------------------------------------------------
// finding 2 -- address IDs matched by content, not position
// ---------------------------------------------------------------------------

func TestMatchAddresses_UnchangedAddressesKeepIDs(t *testing.T) {
	state := []AddressModel{
		addrModel("A1", "email", "a@example.com"),
		addrModel("B2", "email", "b@example.com"),
		addrModel("C3", "email", "c@example.com"),
	}

	// The plan lists the same addresses in a different order.
	plan := []AddressModel{
		addrModel("A1", "email", "c@example.com"),
		addrModel("B2", "email", "a@example.com"),
		addrModel("C3", "email", "b@example.com"),
	}

	got := matchAddresses(plan, candidatesFromModels(state))
	want := []string{"C3", "A1", "B2"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("matchAddresses = %v, want %v", got, want)
		}
	}
}

func TestMatchAddresses_EditedAddressTargetsItsOwnID(t *testing.T) {
	state := []AddressModel{
		addrModel("A1", "email", "a@example.com"),
		addrModel("B2", "email", "b@example.com"),
		addrModel("C3", "email", "c@example.com"),
	}

	// The middle address is edited in place; the stale positional ID from core
	// is deliberately left on the plan entries.
	plan := []AddressModel{
		addrModel("A1", "email", "a@example.com"),
		addrModel("B2", "email", "b-new@example.com"),
		addrModel("C3", "email", "c@example.com"),
	}

	got := matchAddresses(plan, candidatesFromModels(state))
	want := []string{"A1", "B2", "C3"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("matchAddresses = %v, want %v", got, want)
		}
	}
}

func TestMatchAddresses_InsertAtFrontDoesNotStealIDs(t *testing.T) {
	state := []AddressModel{
		addrModel("A1", "email", "a@example.com"),
		addrModel("B2", "webhook", "https://hooks.example.com/b"),
	}

	// A brand new address is inserted first. Positional pairing would give it A1.
	plan := []AddressModel{
		addrModel("A1", "sms", "+15551230000"),
		addrModel("B2", "email", "a@example.com"),
		addrModel("", "webhook", "https://hooks.example.com/b"),
	}

	got := matchAddresses(plan, candidatesFromModels(state))
	want := []string{"", "A1", "B2"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("matchAddresses = %v, want %v", got, want)
		}
	}
}

func TestMatchAddresses_DuplicateContentConsumesEachCandidateOnce(t *testing.T) {
	state := []AddressModel{
		addrModel("A1", "email", "dup@example.com"),
		addrModel("B2", "email", "dup@example.com"),
	}
	plan := []AddressModel{
		addrModel("", "email", "dup@example.com"),
		addrModel("", "email", "dup@example.com"),
		addrModel("", "email", "dup@example.com"),
	}

	got := matchAddresses(plan, candidatesFromModels(state))
	if got[0] != "A1" || got[1] != "B2" || got[2] != "" {
		t.Fatalf("matchAddresses = %v", got)
	}
}

func TestMatchAddresses_UnknownValues(t *testing.T) {
	state := []AddressModel{addrModel("A1", "email", "a@example.com")}

	unknown := addrModel("", "email", "a@example.com")
	unknown.Address = types.StringUnknown()

	got := matchAddresses([]AddressModel{unknown}, candidatesFromModels(state))
	// Falls back to the same-type positional pass.
	if got[0] != "A1" {
		t.Fatalf("matchAddresses = %v, want [A1]", got)
	}

	unknownType := addrModel("", "email", "a@example.com")
	unknownType.Type = types.StringUnknown()
	got = matchAddresses([]AddressModel{unknownType}, candidatesFromModels(state))
	if got[0] != "" {
		t.Fatalf("matchAddresses = %v, want [\"\"]", got)
	}
}

func TestCandidatesFromModels_SkipsUnknownIDs(t *testing.T) {
	models := []AddressModel{
		addrModel("", "email", "new@example.com"),
		addrModel("A1", "email", "a@example.com"),
	}
	got := candidatesFromModels(models)
	if len(got) != 1 || got[0].ID != "A1" {
		t.Fatalf("candidatesFromModels = %+v", got)
	}
}

// ---------------------------------------------------------------------------
// finding 3 -- data is not rewritten
// ---------------------------------------------------------------------------

func TestDataToModel_PreservesConfiguredFormatting(t *testing.T) {
	configured := "{\n  \"@type\": \"MessageCard\",\n  \"text\": \"hi\"\n}"
	planned := addrModel("A1", "webhook", "https://hooks.example.com/a")
	planned.Data = NewJSONStringValue(configured)

	// API returns the same document compacted / re-ordered.
	got := dataToModel(`{"text":"hi","@type":"MessageCard"}`, &planned)
	if got.ValueString() != configured {
		t.Fatalf("data = %q, want the configured string preserved", got.ValueString())
	}

	// API returns a decoded object.
	got = dataToModel(map[string]interface{}{"@type": "MessageCard", "text": "hi"}, &planned)
	if got.ValueString() != configured {
		t.Fatalf("data = %q, want the configured string preserved", got.ValueString())
	}
}

func TestDataToModel_APIValueWinsWhenDifferent(t *testing.T) {
	planned := addrModel("A1", "webhook", "https://hooks.example.com/a")
	planned.Data = NewJSONStringValue(`{"a":1}`)

	got := dataToModel(`{"a":2}`, &planned)
	if got.ValueString() != `{"a":2}` {
		t.Fatalf("data = %q, want the API value", got.ValueString())
	}
}

func TestDataToModel_NonJSONPreserved(t *testing.T) {
	planned := addrModel("A1", "webhook", "https://hooks.example.com/a")
	planned.Data = NewJSONStringValue("field=value&other=1")

	got := dataToModel("field=value&other=1", &planned)
	if got.ValueString() != "field=value&other=1" {
		t.Fatalf("data = %q", got.ValueString())
	}
}

func TestDataToModel_NullCases(t *testing.T) {
	if got := dataToModel(nil, nil); !got.IsNull() {
		t.Fatal("expected null for missing API data")
	}
	if got := dataToModel("", nil); !got.IsNull() {
		t.Fatal("expected null for empty API data")
	}

	planned := addrModel("A1", "webhook", "https://hooks.example.com/a")
	planned.Data = NewJSONStringValue(`{"a":1}`)
	if got := dataToModel(nil, &planned); got.ValueString() != `{"a":1}` {
		t.Fatalf("expected planned data to be kept when the API omits it, got %q", got.ValueString())
	}

	if got := dataToModel(float64(12), nil); got.ValueString() != "12" {
		t.Fatalf("data = %q", got.ValueString())
	}
}

// ---------------------------------------------------------------------------
// plan modifier
// ---------------------------------------------------------------------------

func addressListType(ctx context.Context) types.ListType {
	s := ContactSchema()
	block := s.Blocks["address"]
	return types.ListType{ElemType: block.Type().(types.ListType).ElemType}
}

func addressList(t *testing.T, ctx context.Context, addresses []AddressModel) types.List {
	t.Helper()
	lt := addressListType(ctx)
	list, diags := types.ListValueFrom(ctx, lt.ElemType, addresses)
	if diags.HasError() {
		t.Fatalf("failed to build list: %v", diags)
	}
	return list
}

func TestAddressIDPlanModifier_ReassignsIDsByContent(t *testing.T) {
	ctx := context.Background()

	state := []AddressModel{
		addrModel("A1", "email", "a@example.com"),
		addrModel("B2", "email", "b@example.com"),
	}
	// Terraform core proposes the prior IDs positionally; the new block at index 0
	// wrongly inherits A1.
	plan := []AddressModel{
		addrModel("A1", "sms", "+15551230000"),
		addrModel("B2", "email", "a@example.com"),
		addrModel("", "email", "b@example.com"),
	}

	req := planmodifier.ListRequest{
		PlanValue:  addressList(t, ctx, plan),
		StateValue: addressList(t, ctx, state),
	}
	resp := &planmodifier.ListResponse{PlanValue: req.PlanValue}

	AddressIDPlanModifier().PlanModifyList(ctx, req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var got []AddressModel
	if diags := resp.PlanValue.ElementsAs(ctx, &got, false); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if !got[0].ID.IsUnknown() {
		t.Fatalf("address[0] id = %v, want unknown", got[0].ID)
	}
	if got[1].ID.ValueString() != "A1" {
		t.Fatalf("address[1] id = %q, want A1", got[1].ID.ValueString())
	}
	if got[2].ID.ValueString() != "B2" {
		t.Fatalf("address[2] id = %q, want B2", got[2].ID.ValueString())
	}
}

func TestAddressIDPlanModifier_NullAndEmpty(t *testing.T) {
	ctx := context.Background()
	lt := addressListType(ctx)

	m := AddressIDPlanModifier()
	if m.Description(ctx) == "" || m.MarkdownDescription(ctx) == "" {
		t.Fatal("expected descriptions")
	}

	// Destroy plan.
	req := planmodifier.ListRequest{PlanValue: types.ListNull(lt.ElemType), StateValue: types.ListNull(lt.ElemType)}
	resp := &planmodifier.ListResponse{PlanValue: req.PlanValue}
	m.PlanModifyList(ctx, req, resp)
	if !resp.PlanValue.IsNull() {
		t.Fatal("expected null plan to be left alone")
	}

	// Empty list.
	req = planmodifier.ListRequest{PlanValue: addressList(t, ctx, []AddressModel{}), StateValue: types.ListNull(lt.ElemType)}
	resp = &planmodifier.ListResponse{PlanValue: req.PlanValue}
	m.PlanModifyList(ctx, req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	// Create: no prior state, everything must be unknown.
	req = planmodifier.ListRequest{
		PlanValue:  addressList(t, ctx, []AddressModel{addrModel("", "email", "a@example.com")}),
		StateValue: types.ListNull(lt.ElemType),
	}
	resp = &planmodifier.ListResponse{PlanValue: req.PlanValue}
	m.PlanModifyList(ctx, req, resp)
	var got []AddressModel
	resp.PlanValue.ElementsAs(ctx, &got, false)
	if !got[0].ID.IsUnknown() {
		t.Fatalf("expected unknown id on create, got %v", got[0].ID)
	}
}

// ---------------------------------------------------------------------------
// schema / metadata / configure
// ---------------------------------------------------------------------------

func TestMetadataAndSchema(t *testing.T) {
	ctx := context.Background()
	r := NewContactResource().(*ContactResource)

	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "nodeping"}, metaResp)
	if metaResp.TypeName != "nodeping_contact" {
		t.Fatalf("type name = %q", metaResp.TypeName)
	}

	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", schemaResp.Diagnostics)
	}

	custID := schemaResp.Schema.Attributes["customer_id"]
	if !custID.IsOptional() || !custID.IsComputed() {
		t.Fatal("customer_id must be optional and computed so SubAccount contacts stay addressable")
	}

	block, ok := schemaResp.Schema.Blocks["address"].(rschema.ListNestedBlock)
	if !ok {
		t.Fatal("address block is not a list nested block")
	}
	nested := block.NestedObject.Attributes
	// tofu plan -generate-config-out writes `null # sensitive` for a sensitive
	// attribute. address is Required, so that makes a generated configuration
	// invalid and the contact impossible to import; headers and querystrings are
	// Optional, so instead the plan silently proposes deleting whatever they
	// hold. None of the three is marked sensitive. See A-015.
	for _, name := range []string{"address", "headers", "querystrings"} {
		if nested[name].IsSensitive() {
			t.Fatalf("%s must not be sensitive: config generation would emit null and the import would be lossy", name)
		}
	}
	if _, ok := nested["data"].GetType().(JSONStringType); !ok {
		t.Fatalf("data must use the semantic-equality custom type, got %T", nested["data"].GetType())
	}
	if nested["id"].(rschema.StringAttribute).PlanModifiers != nil {
		t.Fatal("address id must not use a positional plan modifier such as UseStateForUnknown")
	}
}

func TestConfigure(t *testing.T) {
	ctx := context.Background()
	r := &ContactResource{}

	resp := &resource.ConfigureResponse{}
	r.Configure(ctx, resource.ConfigureRequest{}, resp)
	if r.client != nil {
		t.Fatal("expected nil client when no provider data")
	}

	resp = &resource.ConfigureResponse{}
	r.Configure(ctx, resource.ConfigureRequest{ProviderData: "nope"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for wrong provider data type")
	}

	c := client.NewClient(client.ClientConfig{APIToken: "t"})
	resp = &resource.ConfigureResponse{}
	r.Configure(ctx, resource.ConfigureRequest{ProviderData: c}, resp)
	if resp.Diagnostics.HasError() || r.client != c {
		t.Fatal("expected client to be configured")
	}
}

func TestScopedClient(t *testing.T) {
	c := client.NewClient(client.ClientConfig{APIToken: "t"})
	r := &ContactResource{client: c}

	if r.scopedClient(types.StringNull()) != c {
		t.Fatal("expected the root client for a null customer id")
	}
	if r.scopedClient(types.StringUnknown()) != c {
		t.Fatal("expected the root client for an unknown customer id")
	}
	if r.scopedClient(types.StringValue("")) != c {
		t.Fatal("expected the root client for an empty customer id")
	}
	if r.scopedClient(types.StringValue("SUB1")) == c {
		t.Fatal("expected a scoped client for a customer id")
	}
}

// ---------------------------------------------------------------------------
// CRUD
// ---------------------------------------------------------------------------

func TestCreate(t *testing.T) {
	ctx := context.Background()

	var captured client.ContactCreateRequest
	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Errorf("bad request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(contactJSON("C1", "CUST1", "Example", map[string]client.ContactAddress{
			"ZZ": apiAddr("email", "a@example.com"),
			"AA": apiAddr("webhook", "https://hooks.example.com/b"),
		}))
	})

	headers, _ := types.MapValueFrom(ctx, types.StringType, map[string]string{"content-type": "application/json"})
	webhook := addrModel("", "webhook", "https://hooks.example.com/b")
	webhook.Action = types.StringValue("post")
	webhook.Headers = headers
	webhook.QueryStrings = headers
	webhook.Data = NewJSONStringValue("{\n  \"a\": 1\n}")
	webhook.Priority = types.Int64Value(1)
	webhook.Mute = types.BoolValue(true)

	plan := ContactResourceModel{
		ID:         types.StringUnknown(),
		CustomerID: types.StringUnknown(),
		Name:       types.StringValue("Example"),
		CustRole:   types.StringValue("notify"),
		Addresses: []AddressModel{
			addrModel("", "email", "a@example.com"),
			webhook,
		},
	}

	resp := &resource.CreateResponse{State: emptyState(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{Plan: emptyPlan(t, ctx, plan)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}

	if len(captured.NewAddresses) != 2 {
		t.Fatalf("sent %d new addresses", len(captured.NewAddresses))
	}
	if captured.NewAddresses[1].Data != "{\n  \"a\": 1\n}" {
		t.Fatalf("data was rewritten before being sent: %q", captured.NewAddresses[1].Data)
	}

	got := decodeState(t, ctx, resp.State)
	if got.ID.ValueString() != "C1" || got.CustomerID.ValueString() != "CUST1" {
		t.Fatalf("state ids = %q / %q", got.ID.ValueString(), got.CustomerID.ValueString())
	}
	// Plan order is preserved even though the API IDs sort the other way.
	if got.Addresses[0].ID.ValueString() != "ZZ" || got.Addresses[1].ID.ValueString() != "AA" {
		t.Fatalf("addresses out of order: %q, %q", got.Addresses[0].ID.ValueString(), got.Addresses[1].ID.ValueString())
	}
	if got.Addresses[1].Data.ValueString() != "{\n  \"a\": 1\n}" {
		t.Fatalf("data was normalized in state: %q", got.Addresses[1].Data.ValueString())
	}
}

func TestCreateError(t *testing.T) {
	ctx := context.Background()
	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"boom"}`))
	})

	plan := ContactResourceModel{
		ID:         types.StringUnknown(),
		CustomerID: types.StringUnknown(),
		Name:       types.StringValue("Example"),
		CustRole:   types.StringValue("notify"),
	}
	resp := &resource.CreateResponse{State: emptyState(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{Plan: emptyPlan(t, ctx, plan)}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic")
	}
}

func TestCreateUsesConfiguredCustomerID(t *testing.T) {
	ctx := context.Background()

	var gotCustomerID string
	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		gotCustomerID = req.URL.Query().Get("customerid")
		w.Write(contactJSON("C1", "SUB1", "Example", nil))
	})

	plan := ContactResourceModel{
		ID:         types.StringUnknown(),
		CustomerID: types.StringValue("SUB1"),
		Name:       types.StringValue("Example"),
		CustRole:   types.StringValue("notify"),
	}
	resp := &resource.CreateResponse{State: emptyState(t, ctx, nil)}
	r.Create(ctx, resource.CreateRequest{Plan: emptyPlan(t, ctx, plan)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}
	if gotCustomerID != "SUB1" {
		t.Fatalf("customerid query param = %q, want SUB1", gotCustomerID)
	}
	if got := decodeState(t, ctx, resp.State); got.CustomerID.ValueString() != "SUB1" {
		t.Fatalf("customer_id = %q", got.CustomerID.ValueString())
	}
}

func TestRead(t *testing.T) {
	ctx := context.Background()

	var gotCustomerID string
	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		gotCustomerID = req.URL.Query().Get("customerid")
		w.Write(contactJSON("C1", "SUB1", "Renamed", map[string]client.ContactAddress{
			"B2": apiAddr("email", "b@example.com"),
			"A1": apiAddr("email", "a@example.com"),
		}))
	})

	state := ContactResourceModel{
		ID:         types.StringValue("C1"),
		CustomerID: types.StringValue("SUB1"),
		Name:       types.StringValue("Example"),
		CustRole:   types.StringValue("notify"),
		Addresses: []AddressModel{
			addrModel("B2", "email", "b@example.com"),
			addrModel("A1", "email", "a@example.com"),
		},
	}

	resp := &resource.ReadResponse{State: emptyState(t, ctx, &state)}
	r.Read(ctx, resource.ReadRequest{State: emptyState(t, ctx, &state)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}
	if gotCustomerID != "SUB1" {
		t.Fatalf("read did not scope to the SubAccount, customerid = %q", gotCustomerID)
	}

	got := decodeState(t, ctx, resp.State)
	if got.Name.ValueString() != "Renamed" {
		t.Fatalf("name = %q", got.Name.ValueString())
	}
	if got.Addresses[0].ID.ValueString() != "B2" || got.Addresses[1].ID.ValueString() != "A1" {
		t.Fatalf("read reordered addresses: %q, %q", got.Addresses[0].ID.ValueString(), got.Addresses[1].ID.ValueString())
	}
}

func TestReadNotFoundRemovesResource(t *testing.T) {
	ctx := context.Background()
	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	})

	state := ContactResourceModel{ID: types.StringValue("C1"), CustomerID: types.StringNull(), Name: types.StringValue("x"), CustRole: types.StringValue("notify")}
	resp := &resource.ReadResponse{State: emptyState(t, ctx, &state)}
	r.Read(ctx, resource.ReadRequest{State: emptyState(t, ctx, &state)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected the resource to be removed from state")
	}
}

func TestReadError(t *testing.T) {
	ctx := context.Background()
	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"boom"}`))
	})

	state := ContactResourceModel{ID: types.StringValue("C1"), CustomerID: types.StringNull(), Name: types.StringValue("x"), CustRole: types.StringValue("notify")}
	resp := &resource.ReadResponse{State: emptyState(t, ctx, &state)}
	r.Read(ctx, resource.ReadRequest{State: emptyState(t, ctx, &state)}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic")
	}
}

func TestUpdateTargetsTheEditedAddress(t *testing.T) {
	ctx := context.Background()

	var captured client.ContactUpdateRequest
	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Errorf("bad request body: %v", err)
		}
		w.Write(contactJSON("C1", "CUST1", "Example", map[string]client.ContactAddress{
			"A1": apiAddr("email", "a@example.com"),
			"B2": apiAddr("email", "b-new@example.com"),
			"C3": apiAddr("email", "c@example.com"),
		}))
	})

	state := ContactResourceModel{
		ID:         types.StringValue("C1"),
		CustomerID: types.StringValue("CUST1"),
		Name:       types.StringValue("Example"),
		CustRole:   types.StringValue("notify"),
		Addresses: []AddressModel{
			addrModel("A1", "email", "a@example.com"),
			addrModel("B2", "email", "b@example.com"),
			addrModel("C3", "email", "c@example.com"),
		},
	}

	plan := state
	plan.Addresses = []AddressModel{
		addrModel("A1", "email", "a@example.com"),
		addrModel("B2", "email", "b-new@example.com"),
		addrModel("C3", "email", "c@example.com"),
	}

	resp := &resource.UpdateResponse{State: emptyState(t, ctx, &state)}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  emptyPlan(t, ctx, plan),
		State: emptyState(t, ctx, &state),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", resp.Diagnostics)
	}

	if len(captured.NewAddresses) != 0 {
		t.Fatalf("expected no new addresses, got %d", len(captured.NewAddresses))
	}
	if len(captured.Addresses) != 3 {
		t.Fatalf("expected 3 address updates, got %d", len(captured.Addresses))
	}
	if captured.Addresses["B2"].Address != "b-new@example.com" {
		t.Fatalf("the edit was applied to the wrong address: %+v", captured.Addresses)
	}
	if captured.Addresses["A1"].Address != "a@example.com" || captured.Addresses["C3"].Address != "c@example.com" {
		t.Fatalf("unrelated addresses were rewritten: %+v", captured.Addresses)
	}
}

func TestUpdateReorderDoesNotCrossAssignIDs(t *testing.T) {
	ctx := context.Background()

	var captured client.ContactUpdateRequest
	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		json.Unmarshal(body, &captured)
		w.Write(contactJSON("C1", "CUST1", "Example", map[string]client.ContactAddress{
			"A1": apiAddr("email", "a@example.com"),
			"B2": apiAddr("webhook", "https://hooks.example.com/b"),
			"D4": apiAddr("sms", "+15551230000"),
		}))
	})

	state := ContactResourceModel{
		ID:         types.StringValue("C1"),
		CustomerID: types.StringValue("CUST1"),
		Name:       types.StringValue("Example"),
		CustRole:   types.StringValue("notify"),
		Addresses: []AddressModel{
			addrModel("A1", "email", "a@example.com"),
			addrModel("B2", "webhook", "https://hooks.example.com/b"),
		},
	}

	plan := state
	plan.Addresses = []AddressModel{
		addrModel("", "sms", "+15551230000"),
		addrModel("A1", "webhook", "https://hooks.example.com/b"),
		addrModel("B2", "email", "a@example.com"),
	}

	resp := &resource.UpdateResponse{State: emptyState(t, ctx, &state)}
	r.Update(ctx, resource.UpdateRequest{Plan: emptyPlan(t, ctx, plan), State: emptyState(t, ctx, &state)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", resp.Diagnostics)
	}

	if len(captured.NewAddresses) != 1 || captured.NewAddresses[0].Type != "sms" {
		t.Fatalf("expected the sms address to be created, got %+v", captured.NewAddresses)
	}
	if captured.Addresses["A1"].Address != "a@example.com" {
		t.Fatalf("A1 was cross-assigned: %+v", captured.Addresses["A1"])
	}
	if captured.Addresses["B2"].Address != "https://hooks.example.com/b" {
		t.Fatalf("B2 was cross-assigned: %+v", captured.Addresses["B2"])
	}
}

func TestUpdateError(t *testing.T) {
	ctx := context.Background()
	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"boom"}`))
	})

	state := ContactResourceModel{ID: types.StringValue("C1"), CustomerID: types.StringNull(), Name: types.StringValue("x"), CustRole: types.StringValue("notify")}
	resp := &resource.UpdateResponse{State: emptyState(t, ctx, &state)}
	r.Update(ctx, resource.UpdateRequest{Plan: emptyPlan(t, ctx, state), State: emptyState(t, ctx, &state)}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic")
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()

	var gotCustomerID string
	var gotMethod string
	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		gotCustomerID = req.URL.Query().Get("customerid")
		gotMethod = req.Method
		w.Write([]byte(`{"ok":true}`))
	})

	state := ContactResourceModel{ID: types.StringValue("C1"), CustomerID: types.StringValue("SUB1"), Name: types.StringValue("x"), CustRole: types.StringValue("notify")}
	resp := &resource.DeleteResponse{State: emptyState(t, ctx, &state)}
	r.Delete(ctx, resource.DeleteRequest{State: emptyState(t, ctx, &state)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("delete diagnostics: %v", resp.Diagnostics)
	}
	if gotMethod != http.MethodDelete || gotCustomerID != "SUB1" {
		t.Fatalf("method = %q, customerid = %q", gotMethod, gotCustomerID)
	}
}

func TestDeleteError(t *testing.T) {
	ctx := context.Background()
	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"boom"}`))
	})

	state := ContactResourceModel{ID: types.StringValue("C1"), CustomerID: types.StringNull(), Name: types.StringValue("x"), CustRole: types.StringValue("notify")}
	resp := &resource.DeleteResponse{State: emptyState(t, ctx, &state)}
	r.Delete(ctx, resource.DeleteRequest{State: emptyState(t, ctx, &state)}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic")
	}
}

func TestImportStateSubAccount(t *testing.T) {
	ctx := context.Background()

	var gotCustomerID string
	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		gotCustomerID = req.URL.Query().Get("customerid")
		w.Write(contactJSON("C1", "SUB1", "Example", map[string]client.ContactAddress{
			"A1": apiAddr("email", "a@example.com"),
		}))
	})

	resp := &resource.ImportStateResponse{State: emptyState(t, ctx, nil)}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "SUB1:C1"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}
	if gotCustomerID != "SUB1" {
		t.Fatalf("customerid = %q", gotCustomerID)
	}

	got := decodeState(t, ctx, resp.State)
	if got.CustomerID.ValueString() != "SUB1" {
		t.Fatalf("customer_id was not persisted: %q", got.CustomerID.ValueString())
	}
	if got.ID.ValueString() != "C1" || len(got.Addresses) != 1 {
		t.Fatalf("unexpected imported state: %+v", got)
	}
}

func TestImportStatePlain(t *testing.T) {
	ctx := context.Background()

	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		if q := req.URL.Query().Get("customerid"); q != "" {
			t.Errorf("unexpected customerid query param %q", q)
		}
		w.Write(contactJSON("C1", "ROOT", "Example", nil))
	})

	resp := &resource.ImportStateResponse{State: emptyState(t, ctx, nil)}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "C1"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import diagnostics: %v", resp.Diagnostics)
	}
	if got := decodeState(t, ctx, resp.State); got.CustomerID.ValueString() != "ROOT" {
		t.Fatalf("customer_id = %q", got.CustomerID.ValueString())
	}
}

func TestImportStateInvalid(t *testing.T) {
	ctx := context.Background()
	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	})

	for _, id := range []string{"a:b:c", ""} {
		resp := &resource.ImportStateResponse{State: emptyState(t, ctx, nil)}
		r.ImportState(ctx, resource.ImportStateRequest{ID: id}, resp)
		if !resp.Diagnostics.HasError() {
			t.Fatalf("expected an error for import id %q", id)
		}
	}

	resp := &resource.ImportStateResponse{State: emptyState(t, ctx, nil)}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "missing"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when the contact does not exist")
	}
}

func TestStringMapFromModel(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	if got := stringMapFromModel(ctx, types.MapNull(types.StringType), &diags); got != nil {
		t.Fatalf("expected nil for a null map, got %v", got)
	}
	if got := stringMapFromModel(ctx, types.MapUnknown(types.StringType), &diags); got != nil {
		t.Fatalf("expected nil for an unknown map, got %v", got)
	}

	m, _ := types.MapValueFrom(ctx, types.StringType, map[string]string{"a": "b"})
	got := stringMapFromModel(ctx, m, &diags)
	if got["a"] != "b" {
		t.Fatalf("map = %v", got)
	}
}

func TestNewAddressAndContactAddressFromModel(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	headers, _ := types.MapValueFrom(ctx, types.StringType, map[string]string{"x": "y"})
	m := addrModel("A1", "webhook", "https://hooks.example.com/a")
	m.Action = types.StringValue("post")
	m.Data = NewJSONStringValue(`{"a":1}`)
	m.Priority = types.Int64Value(-2)
	m.Headers = headers
	m.QueryStrings = headers
	m.Mute = types.BoolValue(true)

	na := newAddressFromModel(ctx, m, &diags)
	if na.Action != "post" || na.Data != `{"a":1}` || *na.Priority != -2 || na.Headers["x"] != "y" || na.Mute != true {
		t.Fatalf("newAddressFromModel = %+v", na)
	}

	ca := contactAddressFromModel(ctx, m, &diags)
	if ca.Action != "post" || ca.Data != `{"a":1}` || *ca.Priority != -2 || string(ca.Mute) != "true" {
		t.Fatalf("contactAddressFromModel = %+v", ca)
	}

	bare := addrModel("A1", "email", "a@example.com")
	ca = contactAddressFromModel(ctx, bare, &diags)
	if ca.Action != "" || ca.Priority != nil || ca.Headers != nil {
		t.Fatalf("contactAddressFromModel = %+v", ca)
	}
	// Un-muting must reach the API; omitting the field makes it a silent no-op.
	if string(ca.Mute) != "false" {
		t.Fatalf("mute = %q, want an explicit false", string(ca.Mute))
	}
}

func TestUpdateSendsExplicitUnmute(t *testing.T) {
	ctx := context.Background()

	var raw map[string]interface{}
	r, _ := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		json.Unmarshal(body, &raw)
		w.Write(contactJSON("C1", "CUST1", "Example", map[string]client.ContactAddress{
			"A1": apiAddr("email", "a@example.com"),
		}))
	})

	muted := addrModel("A1", "email", "a@example.com")
	muted.Mute = types.BoolValue(true)

	state := ContactResourceModel{
		ID:         types.StringValue("C1"),
		CustomerID: types.StringValue("CUST1"),
		Name:       types.StringValue("Example"),
		CustRole:   types.StringValue("notify"),
		Addresses:  []AddressModel{muted},
	}

	plan := state
	plan.Addresses = []AddressModel{addrModel("A1", "email", "a@example.com")}

	resp := &resource.UpdateResponse{State: emptyState(t, ctx, &state)}
	r.Update(ctx, resource.UpdateRequest{Plan: emptyPlan(t, ctx, plan), State: emptyState(t, ctx, &state)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", resp.Diagnostics)
	}

	addresses, _ := raw["addresses"].(map[string]interface{})
	a1, _ := addresses["A1"].(map[string]interface{})
	mute, present := a1["mute"]
	if !present {
		t.Fatalf("mute was omitted from the update payload: %+v", a1)
	}
	if mute != false {
		t.Fatalf("mute = %v, want false", mute)
	}
}
