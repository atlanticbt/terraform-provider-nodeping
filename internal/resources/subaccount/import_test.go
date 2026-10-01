package subaccount

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// GET /accounts/:id returns customer_name, timezone, defaultlocations, emailme
// (a boolean) and status, but never contactname, email or location. An
// imported subaccount must read back without those being required.
func TestRead_ShouldFillLocationAndEmailMeWhenImported(t *testing.T) {
	ctx := context.Background()
	r := newTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		body, _ := json.Marshal(map[string]interface{}{
			"_id": accountID, "type": "customer", "customer_name": "Example Client",
			"creation_date": 1522264837864, "status": "Active", "timezone": "-5.0",
			"defaultlocations": []string{"eur"}, "emailme": true,
		})
		_, _ = w.Write(body)
	})

	prior := SubaccountResourceModel{ID: types.StringValue(accountID)}
	prior.Name, prior.ContactName, prior.Email, prior.Location = types.StringNull(), types.StringNull(), types.StringNull(), types.StringNull()
	prior.Timezone, prior.EmailMe, prior.AutodiagNotifications, prior.Status = types.StringNull(), types.StringNull(), types.StringNull(), types.StringNull()
	prior.Type, prior.NextBillingDate = types.StringNull(), types.StringNull()
	prior.CreationDate = types.Int64Null()
	prior.DefaultLocations = types.ListNull(types.StringType)

	resp := &resource.ReadResponse{State: emptyState(t, ctx, &prior)}
	r.Read(ctx, resource.ReadRequest{State: emptyState(t, ctx, &prior)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	got := decodeState(t, ctx, resp.State)
	if got.Location.ValueString() != "eur" {
		t.Errorf("location = %v, want eur from defaultlocations", got.Location)
	}
	if got.EmailMe.ValueString() != "yes" {
		t.Errorf("emailme = %v, want yes from the API", got.EmailMe)
	}
	if !got.ContactName.IsNull() || !got.Email.IsNull() {
		t.Errorf("contactname/email = %v/%v, want null: NodePing never returns them", got.ContactName, got.Email)
	}
	if got.Timezone.ValueString() != "-5.0" {
		t.Errorf("timezone = %v", got.Timezone)
	}
}

func modifyPlan(t *testing.T, plan SubaccountResourceModel, state *SubaccountResourceModel) bool {
	t.Helper()
	ctx := context.Background()
	st := tfsdk.State{Schema: SubaccountSchema(), Raw: tftypes.NewValue(SubaccountSchema().Type().TerraformType(ctx), nil)}
	if state != nil {
		st = emptyState(t, ctx, state)
	}
	resp := &resource.ModifyPlanResponse{Plan: emptyPlan(t, ctx, plan)}
	(&SubaccountResource{}).ModifyPlan(ctx, resource.ModifyPlanRequest{Plan: emptyPlan(t, ctx, plan), State: st}, resp)
	return resp.Diagnostics.HasError()
}

func TestModifyPlan_ShouldRequireCreateOnlyFieldsOnCreate(t *testing.T) {
	for _, field := range []string{"contactname", "email", "location"} {
		p := planModel()
		switch field {
		case "contactname":
			p.ContactName = types.StringNull()
		case "email":
			p.Email = types.StringNull()
		case "location":
			p.Location = types.StringNull()
		}
		if !modifyPlan(t, p, nil) {
			t.Errorf("create without %s: expected an error", field)
		}
	}
	if modifyPlan(t, planModel(), nil) {
		t.Error("create with every field set: unexpected error")
	}
}

func TestModifyPlan_ShouldAllowMissingCreateOnlyFieldsOnExistingSubaccount(t *testing.T) {
	p := planModel()
	p.ContactName, p.Email = types.StringNull(), types.StringNull()
	st := stateModel()
	st.ContactName, st.Email = types.StringNull(), types.StringNull()
	if modifyPlan(t, p, &st) {
		t.Error("an imported subaccount without contactname/email must plan without error")
	}
}
