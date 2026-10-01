package contact

import (
	"context"
	"testing"

	"bitbucket.org/atlanticbt/terraform-provider-nodeping/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Owner and login contacts come back with addresses that carry no type. The
// type must read as null, not "", or generated configuration sets
// type = "" and every plan shows a change.
func TestAddressToModel_ShouldReadNullTypeWhenNodePingOmitsIt(t *testing.T) {
	var diags diag.Diagnostics
	m := addressToModel(context.Background(), "VMVVD", client.ContactAddress{Address: "owner@example.com", Status: "new"}, nil, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !m.Type.IsNull() {
		t.Errorf("type = %v, want null", m.Type)
	}
}

// NodePing cannot create an address without a type, so a new address block
// that omits it is refused before the API call.
func TestNewAddressFromModel_ShouldFailWhenTypeIsMissing(t *testing.T) {
	var diags diag.Diagnostics
	m := addrModel("", "email", "new@example.com")
	m.Type = types.StringNull()
	newAddressFromModel(context.Background(), m, &diags)
	if !diags.HasError() {
		t.Error("expected an error for a new address without a type")
	}
}

// The parent account's owner contacts use custrole = "owner"; rejecting it made
// them impossible to import.
func TestSchema_ShouldAcceptOwnerCustRole(t *testing.T) {
	attr := ContactSchema().Attributes["custrole"].(schema.StringAttribute)
	for _, role := range []string{"owner", "edit", "view", "notify"} {
		resp := &validator.StringResponse{}
		for _, v := range attr.Validators {
			v.ValidateString(context.Background(), validator.StringRequest{Path: path.Root("custrole"), ConfigValue: types.StringValue(role)}, resp)
		}
		if resp.Diagnostics.HasError() {
			t.Errorf("custrole %q rejected: %v", role, resp.Diagnostics)
		}
	}
	resp := &validator.StringResponse{}
	for _, v := range attr.Validators {
		v.ValidateString(context.Background(), validator.StringRequest{Path: path.Root("custrole"), ConfigValue: types.StringValue("admin")}, resp)
	}
	if !resp.Diagnostics.HasError() {
		t.Error("custrole admin accepted, want rejected")
	}
}
