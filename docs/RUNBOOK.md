<!--
doc: runbook
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# Runbook: OpenTofu Provider for NodePing

> **A plan or apply failed. Start here.**

The provider is not a service. It is a plugin that OpenTofu starts for each command and
stops after the command. There is no process to restart and no health endpoint. Most
problems come from the token, the NodePing API or the configuration.

## Is the Provider Working?

1. Show the OpenTofu version and the installed providers:

   ```bash
   tofu version
   tofu providers
   ```

   The output must show `registry.opentofu.org/atlanticbt/nodeping`.

   > **SECURITY:** The next step uses the API token. Do not type the token on the command
   > line, because other users can see command arguments in the process list. Pass it
   > through standard input.

2. Test the token with a read-only API request:

   ```bash
   printf 'user = "%s:"\n' "$NODEPING_API_TOKEN" \
     | curl -sS -K - https://api.nodeping.com/api/1/accounts -o /dev/null -w '%{http_code}\n'
   ```

   | Result | Meaning |
   |---|---|
   | `200` | The token works. |
   | `401` or `403` | The token is wrong, revoked or has no access. |
   | No result, or a timeout | A network problem between you and NodePing. |

   > **SECURITY:** Debug logs can contain request and response bodies, which can include
   > check credentials and webhook tokens. Do not attach debug logs to a public issue.
   > Remove secrets first.

3. Run the failed command again with provider debug logs:

   ```bash
   TF_LOG_PROVIDER=DEBUG tofu plan 2> provider-debug.log
   ```

   Use `TF_LOG=DEBUG` to also log OpenTofu core.

## Provider Overview

| Property | Value |
|---|---|
| Type | OpenTofu provider plugin, plugin protocol 6 |
| Provider address | `registry.opentofu.org/atlanticbt/nodeping` |
| API endpoint | `https://api.nodeping.com/api/1` (`internal/client/client.go:27`) |
| Authentication | API token as the HTTPS basic-auth user name (`internal/client/client.go:411`) |
| Rate limit | 10 requests each second by default (`internal/client/client.go:28`) |
| Retries | 3 by default, with exponential backoff from 1 to 30 seconds (`internal/client/client.go:29`) |
| Request timeout | 30 seconds (`internal/client/client.go:32`) |
| Maximum response size | 16 MiB (`internal/client/client.go:39`) |
| Logs | OpenTofu writes them to standard error when `TF_LOG` or `TF_LOG_PROVIDER` is set. |
| Released by | GitHub Actions, on a signed `v*` tag (`.github/workflows/release.yml`) |

## Known Failure Modes

| Symptom (error summary or message) | Root cause | Immediate fix |
|---|---|---|
| `Missing NodePing API Token` (`internal/provider/provider.go:333`) | Neither `api_token` nor `NODEPING_API_TOKEN` is set. | Set `NODEPING_API_TOKEN`. The provider does not read `NODEPING_API_KEY`. |
| `Unknown NodePing API Token` (`internal/provider/provider.go:296`) | `api_token` comes from a value that OpenTofu knows only after apply. | Use a value that is known at plan time, or set `NODEPING_API_TOKEN`. |
| `Invalid NodePing API URL` (`internal/provider/provider.go:347`) | `api_url` is not an absolute HTTPS URL. HTTP is allowed only for a loopback host. | Remove `api_url`, or correct it. |
| `Invalid Retry Backoff Window` (`internal/provider/provider.go:219`) | `retry_wait_min` is larger than `retry_wait_max`. | Swap the two values. |
| `NodePing API error (status 401, ...)` or `(status 403, ...)` | The token is wrong or revoked, or it cannot reach the account in `customer_id`. | Make a new token in NodePing. Check `customer_id`. |
| Applies are slow, with many retries | NodePing returns 429 (rate limit). The client waits and tries again. | Lower `rate_limit` in the provider block. |
| `NodePing API error (status 5xx, ...)` | A NodePing server error. The client retries reads. It does not send a received POST again. | Run the command again later. Check the NodePing status page. |
| `response from <path> exceeds 16777216 bytes; refusing to parse a truncated body` (`internal/client/client.go:440`) | A list response is larger than 16 MiB. | Open a GitHub issue. Give the endpoint and the approximate object count. |
| A resource disappears from state, and the plan creates it again | The object was deleted in NodePing outside OpenTofu. The provider removes a missing object from state. | Make sure the deletion was intended. Apply to create it again, or remove it from the configuration. |
| `Provider produced inconsistent result after apply` | A provider defect. The value after apply is different from the plan. | Open a GitHub issue. Add the resource configuration with all secrets removed. |
| `Attribute Not Applicable To Check Type` (warning) (`internal/resources/check/validate.go:123`) | The check has a value that NodePing ignores for its type. | Remove the attribute, or ignore the warning. |
| `Interval Below NodePing Minimum` (`internal/resources/check/validate.go:139`) | The interval is below the minimum for that check type, for example WHOIS or RDAP. | Increase `interval`. |
| `Missing Address Type` (`internal/resources/contact/resource.go:378`) | A new contact address has no `type`. | Add `type` to the address block. |
| `Cannot Remove Every Contact Address` (`internal/resources/contact/resource.go:210`) | The change removes the last address of a contact. NodePing refuses this. | Keep one address, or delete the contact. |
| `Cannot Change After Creation` (`internal/resources/subaccount/schema.go:88`) | The change sets a new `contactname` or `email` on an existing subaccount. NodePing cannot change these values. | Change the value in the NodePing web interface. Put the same value in the configuration. |
| `Missing Attribute For New Subaccount` (`internal/resources/subaccount/resource.go:400`) | A new subaccount has no `contactname`, `email` or `location`. | Add the missing attribute. |
| `Ad-hoc Maintenance Cannot Be Imported` (`internal/resources/maintenance/resource.go:286`) | The import ID is an ad-hoc window. NodePing deletes ad-hoc windows when they finish. | Import a scheduled window only. |
| `Invalid Import ID` | The import ID has the wrong format. | Use `ID` or `CUSTOMER_ID:ID`. See the resource page in `docs/resources/`. |
| `Instance cannot be destroyed` | The configuration has `lifecycle { prevent_destroy = true }` and the plan destroys the resource. This is an OpenTofu safety control. | Make sure that the destroy is intended. Remove `prevent_destroy` in a reviewed change. |
| Acceptance test fails, and objects stay in the test account | A test stopped before cleanup, or the NodePing API changed. | Delete objects with labels that start with `acctest-` in the test account. Run the failed test again. |

## Environment Variables

> **SECURITY:** Never log, print or commit these values. Rotate a token immediately if it is
> exposed.

| Variable | Required | Description | Where to find it |
|---|---|---|---|
| `NODEPING_API_TOKEN` | Yes, if `api_token` is not set | The NodePing API token. | NodePing web interface, account settings. |
| `NODEPING_CUSTOMER_ID` | No | The default subaccount ID. | NodePing web interface, subaccount list. |
| `NODEPING_API_URL` | No | A different API base URL, for tests only. | Not needed for normal use. |
| `TF_LOG`, `TF_LOG_PROVIDER` | No | OpenTofu log level, for example `DEBUG`. | OpenTofu documentation. |

[docs/ENV_VARS.md](ENV_VARS.md) lists all variables, including the test variables.

## Rollback

### Roll Back the Provider Version

1. Find the last version that worked. Look at the `.terraform.lock.hcl` file in version
   control, or at the release list on GitHub.
2. Pin that version in `required_providers`:

   ```hcl
   nodeping = {
     source  = "atlanticbt/nodeping"
     version = "= 1.1.0"
   }
   ```

3. Download the pinned version:

   ```bash
   tofu init -upgrade
   ```

4. Run `tofu plan`. Make sure that the plan shows only the changes that you expect.

### Roll Back a Configuration Change

1. Show the recent commits:

   ```bash
   git log --oneline -10
   ```

2. Revert the commit. This makes a new commit and does not rewrite history:

   ```bash
   git revert <commit>
   ```

3. Run `tofu plan` and `tofu apply` through your normal review process.

A revert restores configuration, not deleted NodePing data. A check that is created again
gets a new ID and loses its result history.

## Escalation Path

| Problem | Where to go |
|---|---|
| A provider defect | Open a GitHub issue in `atlanticbt/terraform-provider-nodeping`. Remove all secrets. |
| A security vulnerability | Follow [SECURITY.md](../SECURITY.md). Do not open a public issue. |
| No response in a reasonable time | Contact the code owners, `@atlanticbt/nodeping-maintainers` (`.github/CODEOWNERS`). |
| A NodePing service problem | Contact NodePing support. |
