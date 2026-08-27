# ADR-002: Authentication Handling Strategy

**Status**: Accepted  
**Date**: 2026-01-27  
**Decision Makers**: Terraform Provider Engineering

## Context

The NodePing API requires token-based authentication. We need to define:

1. How users configure authentication
2. How credentials are stored and transmitted
3. How to handle multi-account scenarios

## Decision

### Authentication Configuration

1. **Provider Block Configuration** (highest priority)
   ```hcl
   provider "nodeping" {
     api_token = var.nodeping_token
   }
   ```

2. **Environment Variable Fallback** (if not in config)
   - `NODEPING_API_TOKEN` for the API token
   - `NODEPING_CUSTOMER_ID` for SubAccount operations

### Credential Transmission

- Use **HTTP Basic Authentication** with token as username
- Password field left empty (NodePing ignores it)
- Never include token in query strings or logs

### Multi-Account Pattern

Use standard Terraform provider aliases:

```hcl
provider "nodeping" {
  alias       = "subaccount"
  api_token   = var.token
  customer_id = "SUBACCOUNT_ID"
}
```

## Rationale

### Why Provider Config + Env Var Fallback

1. **Flexibility**: Supports both explicit configuration and CI/CD patterns
2. **Security**: Env vars avoid hardcoding secrets in HCL
3. **Precedence**: Explicit config overrides env vars (expected behavior)
4. **Standard Pattern**: Matches AWS, GCP, Azure provider patterns

### Why HTTP Basic Auth

1. **Security**: Token not exposed in URLs or logs
2. **Simplicity**: Standard HTTP mechanism
3. **NodePing Support**: Documented as supported method

### Why Provider Aliases for Multi-Account

1. **Terraform Native**: Built-in pattern, no custom logic needed
2. **Explicit**: Clear which provider handles which resources
3. **Flexible**: Supports any number of accounts

## Security Controls

1. **Sensitive Marking**: `api_token` marked as `Sensitive` in schema
2. **No Logging**: Token never included in log output
3. **No State Storage**: Token not stored in Terraform state
4. **Env Var Docs**: Document secure env var handling

## Consequences

- Provider schema includes `api_token` as optional sensitive string
- Client constructor reads from config first, then env vars
- HTTP client sets Basic Auth header on all requests
- Documentation includes security best practices

## References

- [NodePing API Authentication](https://nodeping.com/docs-api-overview.html)
- [Terraform Provider Configuration](https://developer.hashicorp.com/terraform/language/providers/configuration)
