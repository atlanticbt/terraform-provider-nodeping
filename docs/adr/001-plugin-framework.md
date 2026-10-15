<!--
doc: adr
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# ADR-001: Use the Terraform Plugin Framework

An ADR (Architecture Decision Record) records one design decision and its reasons.

| Field | Value |
|---|---|
| Status | Accepted |
| Date | 2026-01-27 |
| Decision makers | Provider engineering |

## Context

We had two possible methods to build the NodePing provider:

| Option | Description |
|---|---|
| Terraform Plugin SDK v2 | The older SDK (software development kit). Many providers use it. |
| Terraform Plugin Framework | The newer framework. HashiCorp recommends it for new providers. |

## Decision

We use the Terraform Plugin Framework (`github.com/hashicorp/terraform-plugin-framework`).

## Rationale

### Advantages

| Advantage | Description |
|---|---|
| Official recommendation | HashiCorp recommends the framework for all new providers. |
| Type safety | Strongly typed schemas find more errors at compile time. |
| Clear API | The framework API is easier to use correctly. |
| Active development | New features go to the framework first. |
| Diagnostics | The framework gives better errors to users. |
| Protocol 6 | The framework supports plugin protocol version 6. |

### Disadvantages

| Disadvantage | Description |
|---|---|
| Fewer examples | The community has fewer examples than for SDK v2. |
| New patterns | Engineers must learn new patterns. |

### Mitigations

1. Use the official HashiCorp provider examples as a reference.
2. Follow the terraform-plugin-framework documentation.
3. Build simple resources first. For example, build `nodeping_contact` before
   `nodeping_check`.

## Consequences

- All resources and data sources use framework patterns.
- The provider configuration implements the `provider.Provider` interface.
- Schemas use the `schema.Schema` types.
- Resources implement the `resource.Resource` interface for create, read, update and delete.

## References

- [Terraform Plugin Framework documentation](https://developer.hashicorp.com/terraform/plugin/framework)
- [Plugin Framework benefits](https://developer.hashicorp.com/terraform/plugin/framework-benefits)
