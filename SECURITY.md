<!--
doc: security
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# Security Policy

## Supported Versions

The maintainers release security fixes for the latest minor version only. Upgrade to the
newest release before you report a problem. The newest release can already have the fix.

## Reporting a Vulnerability

> **SECURITY: Do NOT open a public GitHub issue or pull request for a security
> vulnerability.**

1. Open the **Security** tab of the repository.
2. Select **Report a vulnerability**.
3. Give this information:
   - The provider version and the OpenTofu or Terraform version.
   - What an attacker can do, and what the attacker needs to do it.
   - The steps or the configuration that show the problem. Remove all real values.

> **SECURITY:** Never put a NodePing API token, an account ID or contact details in a
> report, issue, log or example. Use placeholders. If a token is exposed, revoke it in
> NodePing first.

## What to Expect

| Step | Commitment |
|---|---|
| Acknowledgement | A maintainer acknowledges the report within three business days. |
| Updates | The maintainer tells you about progress during the investigation. |
| Fix | The fix is released as a new version. |
| Advisory | A GitHub security advisory describes the fix. It names the reporter, unless you ask for no credit. |

## Sensitive Data This Project Handles

| Data | Where it is | Protection |
|---|---|---|
| NodePing API token | `api_token` or `NODEPING_API_TOKEN` | `Sensitive` in the provider schema (`internal/provider/provider.go:147`). Sent only as the HTTPS basic-auth user name (`internal/client/client.go:411`). Never logged. |
| Check password | `password` on `nodeping_check` | `Sensitive`, but **written to state in plain text**. Deprecated. |
| Check password (write-only) | `password_wo` on `nodeping_check` | Write-only. OpenTofu never writes it to state or to a saved plan file. |
| SNMP community string | `snmpcom` on `nodeping_check` | `Sensitive`. Written to state. |
| SSH key and client certificate references | `sshkey`, `clientcert` on `nodeping_check` | IDs of keys that NodePing stores. They are not the keys. |
| HTTP request headers | `sendheaders` on `nodeping_check` | **Not** `Sensitive`. Written to state and shown in plans. |
| Webhook headers and query strings | `headers`, `querystrings` on a `nodeping_contact` address | **Not** `Sensitive`. Written to state and shown in plans. |
| Contact addresses | `address` on a `nodeping_contact` address | **Not** `Sensitive`. Email addresses, phone numbers and webhook URLs. |

### Why Some Fields Are Not Sensitive

Headers, query strings and webhook URLs often contain authentication tokens. The provider
does not mark them `Sensitive`, because `tofu plan -generate-config-out` writes `null` for
a sensitive attribute. An imported object would then plan to delete these values.
[docs/assumptions.md](docs/assumptions.md) records this decision (A-015).

> **SECURITY:** The state file contains every value in the table above that is written to
> state. Protect the state file as a secret. Use OpenTofu state encryption and a private
> state backend.

> **SECURITY:** Configuration that `tofu plan -generate-config-out` writes can contain live
> credentials from headers, query strings and webhook URLs. Remove them before you commit
> the file. Replace them with variables.

## Credential and Secret Rules

> **SECURITY:** Keep all secrets in environment variables or a secrets manager. Never
> commit secrets. If a secret is exposed, rotate it immediately.

- Local development: use a `.env` file. `.gitignore` excludes it. Copy `.env.example` to
  start.
- Pipelines: use the secrets store of the pipeline. Do not print secret values.
- Use a monitoring-only credential with the minimum permissions for each authenticated
  check. NodePing stores check passwords in plain text and returns them through its API.

## Dependency Security

1. Run `make vulncheck` (govulncheck) before each release.
2. CI runs govulncheck on each pull request and on each release tag.
3. Dependabot opens pull requests for Go module and GitHub Actions updates.

## Known Security Controls

| Control | Where |
|---|---|
| API URL must be HTTPS, except for a loopback host such as `localhost` | `internal/provider/validators.go:66` |
| Token sent only as basic auth over HTTPS, never logged | `internal/client/client.go:411` |
| Write-only password attribute | `password_wo` on `nodeping_check` |
| Response size limit of 16 MiB (mebibytes). A larger response fails with an error. | `internal/client/client.go:39` |
| Length limits on free-text attributes | `internal/resources/check/schema.go`, `internal/resources/contact/schema.go` |
| Request timeout of 30 seconds | `internal/client/client.go:32` |
| A POST that the server received is never sent again | `internal/client/client.go:298` |
| Static analysis (`gosec`) in CI | `.golangci.yml` |
| Secret scan (gitleaks) in CI | `.github/workflows/ci.yml` |
| Releases only from commits on `main`, after the full CI suite | `.github/workflows/release.yml` |
| Signed release checksums | `.github/workflows/release.yml` |
