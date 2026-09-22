package subaccount

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// recordCreatedID writes the identifier of a subaccount NodePing has already
// created into state, on the way out of a Create that is returning an error.
//
// This one matters most: the POST bills the customer for a real account, and
// two further API calls follow it before state is written. Without this, either
// of them failing leaves an account nobody is tracking, recoverable only by
// someone finding it in the NodePing dashboard.
//
// Writing state on the error path is sound rather than a trick. The framework
// keeps whatever state an errored Create set -- fwserver only rejects a null
// state when there are no errors -- and OpenTofu persists a non-null object
// whenever apply reports errors (node_resource_abstract_instance.go, the
// "diags.HasErrors() && !newVal.IsNull()" case). The plan/apply consistency
// check is skipped in that same situation, so the attributes left null here are
// legal. Read and Delete both need nothing but the ID, so the next refresh
// completes the state and the next destroy removes the account.
//
// Only the identifier is written. Putting the planned model in state instead
// would carry unknowns, which OpenTofu reports as a provider bug and nulls out
// anyway.
//
// Identity is deliberately left untouched: a fully null prior identity is
// accepted by the framework, and Read populates it on the next refresh.
func recordCreatedID(ctx context.Context, resp *resource.CreateResponse, id string) {
	if !resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(id))...)
}
