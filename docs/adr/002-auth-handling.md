<!--
doc: adr
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# ADR-002: Authentication

An ADR (Architecture Decision Record) records one design decision and its reasons.

| Field | Value |
|---|---|
| Status | Accepted |
| Date | 2026-01-27 |
| Decision makers | Provider engineering |

## Context

The NodePing API uses token authentication. We must decide three things:

1. How users configure authentication.
2. How the provider stores and sends credentials.
3. How the provider works with more than one account.

## Decision

### Configuration

The provider reads the token from these sources, in this order:

| Order | Source | Example |
|---|---|---|
| 1 | The provider block | `api_token = var.nodeping_token` |
| 2 | The `NODEPING_API_TOKEN` environment variable | Used when the provider block has no token |

The provider reads no other variable for the token.

`NODEPING_CUSTOMER_ID` is the environment variable for the SubAccount customer ID.

```hcl
provider "nodeping" {
  api_token = var.nodeping_token
}
```

### Transmission

- The provider uses HTTP Basic Authentication. The token is the user name.
- The password field is empty. NodePing ignores it.
- The provider never puts the token in a query string or a log.

### More than one account

Use standard provider aliases:

```hcl
provider "nodeping" {
  alias       = "subaccount"
  api_token   = var.token
  customer_id = "SUBACCOUNT_ID"
}
```

## Rationale

### Provider configuration with an environment variable

| Reason | Description |
|---|---|
| Flexibility | The method supports explicit configuration and CI/CD (continuous integration and delivery) pipelines. |
| Security | An environment variable keeps the secret out of HCL files. |
| Precedence | Explicit configuration overrides the environment variable. Users expect this. |
| Standard pattern | The AWS, Google Cloud and Azure providers use the same pattern. |

### HTTP Basic Authentication

| Reason | Description |
|---|---|
| Security | The token is not in URLs or logs. |
| Simplicity | It is a standard HTTP method. |
| NodePing support | NodePing documents this method. |

### Provider aliases

| Reason | Description |
|---|---|
| Native | OpenTofu and Terraform have this pattern. The provider needs no custom logic. |
| Explicit | The configuration shows which provider manages each resource. |
| Flexible | It supports any number of accounts. |

## Security controls

1. The schema marks `api_token` as `Sensitive`.
2. The provider never writes the token to a log.
3. The provider never stores the token in state.
4. The documentation tells users how to keep environment variables secure.

## Consequences

- The provider schema has `api_token` as an optional, sensitive string.
- The client reads the configuration first. Then it reads the environment variable.
- The HTTP client sets the Basic Authentication header on all requests.
- The documentation contains security guidance.

## References

- [NodePing API authentication](https://nodeping.com/docs-api-overview.html)
- [Provider configuration](https://developer.hashicorp.com/terraform/language/providers/configuration)
