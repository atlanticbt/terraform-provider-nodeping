# ADR-003: Check Resource Design - Single Resource with Type-Specific Attributes

**Status**: Accepted  
**Date**: 2026-01-27  
**Decision Makers**: Terraform Provider Engineering

## Context

NodePing supports 30+ check types, each with different required and optional fields. We need to decide how to model this in Terraform:

1. **Single Resource**: One `nodeping_check` resource with all possible attributes
2. **Multiple Resources**: Separate resources per type (e.g., `nodeping_check_http`, `nodeping_check_dns`)
3. **Hybrid**: Base resource with type-specific nested blocks

## Decision

We will use a **Single Resource** approach with:

- Common attributes available for all check types
- Type-specific attributes validated based on `type` field
- Clear documentation of which attributes apply to which types

```hcl
resource "nodeping_check" "http" {
  type   = "HTTP"
  target = "https://example.com"
  # HTTP-specific attributes available
}

resource "nodeping_check" "dns" {
  type           = "DNS"
  target         = "8.8.8.8"
  dns_type       = "A"
  dns_to_resolve = "example.com"
  # DNS-specific attributes available
}
```

## Rationale

### Advantages of Single Resource

1. **API Alignment**: NodePing API uses single endpoint for all check types
2. **Simpler Codebase**: One resource implementation, not 30+
3. **Easier Maintenance**: API changes require updating one resource
4. **Flexible**: Users can change check types without resource replacement
5. **Consistent UX**: Same resource name regardless of check type

### Disadvantages

1. **Validation Complexity**: Must validate attributes per type
2. **Documentation**: Need clear per-type attribute documentation
3. **Schema Size**: Large schema with many optional attributes

### Mitigations

1. **Custom Validators**: Implement type-aware validation
2. **Generated Docs**: Auto-generate per-type documentation
3. **Attribute Grouping**: Group related attributes logically

## Implementation Details

### Attribute Categories

1. **Universal**: `id`, `type`, `target`, `label`, `enabled`, `interval`, `threshold`, `sens`, `notifications`, `tags`
2. **Location**: `runlocations`, `homeloc`
3. **Content Match**: `contentstring`, `regex`, `invert`
4. **HTTP**: `follow`, `method`, `statuscode`, `sendheaders`, `receiveheaders`, `data`, `postdata`
5. **DNS**: `dns_type`, `dns_to_resolve`, `dns_section`, `dns_rd`, `transport`
6. **SSL/TLS**: `warningdays`, `servername`, `verify`, `secure`
7. **Auth**: `username`, `password`, `sshkey`, `clientcert`
8. **Database**: `database`, `query`, `fields`, `namespace`
9. **Network**: `port`, `ipv6`

### Validation Strategy

```go
func (r *CheckResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
    var data CheckResourceModel
    resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
    
    switch data.Type.ValueString() {
    case "HTTP":
        // Validate HTTP-specific requirements
    case "DNS":
        // Validate DNS-specific requirements
        if data.DNSType.IsNull() {
            resp.Diagnostics.AddAttributeError(...)
        }
    // ... other types
    }
}
```

## Consequences

- Single `nodeping_check` resource handles all check types
- Type-specific validation implemented in `ValidateConfig`
- Documentation organized by check type
- Schema includes all possible attributes as optional

## Alternatives Considered

### Multiple Resources (Rejected)

- Would require 30+ resource implementations
- Significant code duplication
- Harder to maintain as API evolves
- Inconsistent with NodePing API design

### Hybrid with Nested Blocks (Rejected)

- Complex schema with many nested blocks
- Confusing UX (which block to use?)
- Still requires significant validation logic

## References

- [NodePing Check Types](https://nodeping.com/docs-api-checks.html)
- [Terraform Schema Design Patterns](https://developer.hashicorp.com/terraform/plugin/framework/handling-data)
