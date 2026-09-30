package schedule

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// recordCreatedID writes the identifier of a schedule NodePing may already hold
// into state, on the way out of a Create that is returning an error.
//
// Without it a Create that fails after the write has landed leaks: the schedule
// exists in NodePing, nothing in the state file points at it, and OpenTofu
// cannot destroy an object it never recorded. Three schedules were removed by
// hand this way.
//
// Writing state on the error path is sound rather than a trick. The framework
// keeps whatever state an errored Create set -- fwserver only rejects a null
// state when there are no errors -- and OpenTofu persists a non-null object
// whenever apply reports errors (node_resource_abstract_instance.go, the
// "diags.HasErrors() && !newVal.IsNull()" case). The plan/apply consistency
// check is skipped in that same situation, so the attributes left null here are
// legal; the next refresh fills them in, or the next destroy removes the object.
//
// Only known values are written. Putting the planned model in state instead
// would carry unknowns, which OpenTofu reports as a provider bug and nulls out
// anyway.
//
// customer_id is written only when the practitioner configured one, matching
// clientFor: that is the value Read must use to scope its lookup. Left unset,
// the schedule belongs to the token's own account and a null says the same.
//
// Identity is deliberately left untouched. customer_id can still be unknown
// here, and an identity that disagrees with the one the next Read derives is
// rejected as "Unexpected Identity Change" -- which would block the very
// recovery this exists to enable. A fully null prior identity is accepted, and
// Read populates it.
func recordCreatedID(ctx context.Context, resp *resource.CreateResponse, id string, customerID types.String) {
	if !resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(id))...)

	if customerID.IsNull() || customerID.IsUnknown() || customerID.ValueString() == "" {
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("customer_id"), customerID)...)
}
