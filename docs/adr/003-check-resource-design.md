<!--
doc: adr
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# ADR-003: One Check Resource with Type-Specific Attributes

An ADR (Architecture Decision Record) records one design decision and its reasons.

| Field | Value |
|---|---|
| Status | Accepted |
| Date | 2026-01-27 |
| Decision makers | Provider engineering |

## Context

NodePing supports more than 30 check types. Each type has different required and optional
fields. We had three possible models:

| Option | Description |
|---|---|
| One resource | One `nodeping_check` resource with all possible attributes |
| Many resources | One resource for each type, for example `nodeping_check_http` and `nodeping_check_dns` |
| Hybrid | One base resource with a nested block for each type |

## Decision

We use one resource:

- All check types share the common attributes.
- The provider validates the type-specific attributes against the `type` attribute.
- The documentation shows which attributes apply to which types.

```hcl
resource "nodeping_check" "http" {
  type   = "HTTP"
  target = "https://example.com"
}

resource "nodeping_check" "dns" {
  type         = "DNS"
  target       = "8.8.8.8"
  dnstype      = "A"
  dnstoresolve = "example.com"
}
```

## Rationale

### Advantages

| Advantage | Description |
|---|---|
| API alignment | The NodePing API uses one endpoint for all check types. |
| Simple code | One resource to maintain, not more than 30. |
| Easy maintenance | An API change needs a change to one resource only. |
| Flexible | A change of `type` is an update in place. It does not replace the resource. |
| Consistent | The resource name is the same for all check types. |

### Disadvantages

| Disadvantage | Description |
|---|---|
| Complex validation | The provider must validate attributes for each type. |
| Documentation | The documentation must show which attributes apply to each type. |
| Large schema | The schema has many optional attributes. |

### Mitigations

1. Validate attributes against the type. See `internal/resources/check/validate.go:28`.
2. Document the applicable types for each attribute.
3. Put related attributes in groups.

## Implementation

### Attribute groups

| Group | Attributes |
|---|---|
| Universal | `id`, `type`, `target`, `label`, `enabled`, `interval`, `threshold`, `sens`, `notifications`, `tags` |
| Location | `runlocations`, `homeloc` |
| Content match | `contentstring`, `regex`, `invert` |
| HTTP | `follow`, `method`, `statuscode`, `sendheaders`, `receiveheaders`, `data`, `postdata` |
| DNS | `dnstype`, `dnstoresolve`, `dnssection`, `dnsrd`, `transport` |
| SSL/TLS | `warningdays`, `servername`, `verify`, `secure` |
| Authentication | `username`, `password`, `password_wo`, `sshkey`, `clientcert` |
| Database | `database`, `query`, `fields`, `namespace` |
| Network | `port`, `ipv6` |

### Validation

`ValidateConfig` (`internal/resources/check/validate.go:28`) calls one helper for each rule:

| Helper | Rule |
|---|---|
| `validateCredentialRules` | A Redis Sentinel check needs `sentinelname`. `password_wo_version` needs `password_wo`. |
| `validateTargetPresent` | A type that needs a target must have one. |
| `validateApplicability` | An attribute set for a type that does not use it gives a warning. A boolean flag set to true for such a type gives an error. |
| `validateMinInterval` | WHOIS and RDAP intervals must be at or above the NodePing minimum. |
| `validateHostCredentials` | REDIS host credentials must not conflict. |

## Consequences

- One `nodeping_check` resource manages all check types.
- `ValidateConfig` does the type-specific validation.
- The documentation shows the check types for each attribute.
- The schema contains all possible attributes as optional.

## Alternatives that we rejected

### Many resources

- It needs more than 30 resource implementations.
- It causes much duplicate code.
- It is harder to maintain when the API changes.
- It does not agree with the design of the NodePing API.

### Hybrid with nested blocks

- The schema has many nested blocks.
- Users cannot easily know which block to use.
- It still needs much validation logic.

## References

- [NodePing check types](https://nodeping.com/docs-api-checks.html)
- [Plugin Framework: handling data](https://developer.hashicorp.com/terraform/plugin/framework/handling-data)
