<!--
doc: roadmap
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# Roadmap

The items are in order of what blocks production use of this provider.

## Status

| Release | Status | Main content |
|---|---|---|
| v1.0.0 | Done | Complete API coverage, write-only credentials, resource identity |
| v1.1.0 | Done | Live acceptance tests, fixes for defects that the live API showed |
| v2.0.0 | In progress | First public release on the OpenTofu registry. Go module path `/v2`. |
| Next | Planned | Generated docs |
| Later | Not scheduled | See [Later](#later) |

## Done: v1.0.0

The provider covers all 7 NodePing CRUD (create, read, update, delete) resources.

| Area | What was done |
|---|---|
| Resources | `nodeping_check`, `nodeping_contact`, `nodeping_contactgroup`, `nodeping_schedule`, `nodeping_notificationprofile`, `nodeping_maintenance`, `nodeping_subaccount` |
| Data sources | A singular and a plural data source for each resource, plus `nodeping_account` and `nodeping_accounts` |
| Write-only credentials | `password_wo` and `password_wo_version` keep check passwords out of state and out of saved plan files. REDIS `hosts[*].password` uses the same method. |
| Check schema | Added `data`, `fields`, `hosts`, `redistype`, `sentinelname`, `checktoken`, `oldresultfail`, `edns`, `dohdot`, `whoisserver`, `rdapurl`, `ignore`, `verifyvolume` and `volumemin` |
| Correctness | The `mapForRead` and `mapForApply` split, contact address order and identity, deletes that you can repeat, no retry of `POST`, decoding that accepts mixed types, `customer_id` on each resource |
| Resource identity | All seven resources support `import { identity = {...} }` |
| Provider configuration | `api_url` must use `https` outside loopback. Numeric tuning values have bounds. `default_tags` rejects empty strings. |
| Shared response handling | One detector for an error in a `200` response. One classifier for "not found". These replace sixteen copies of the same code. |
| Ad-hoc maintenance | `scripts/nodeping-adhoc-maintenance.sh` and a `terraform_data` recipe. Both are outside provider state on purpose. |
| Quality | Unit test coverage went from 11.3% to more than 95%. All `govulncheck` findings are fixed. |
| Release pipeline | Reproducible builds, secret scanning and a coverage gate |

The shared response handling fixed a hazard. Before the fix, the provider read
`200 {"error": "permission denied"}` as "resource absent". OpenTofu then removed the
resource from state and created the live object again.

An unencrypted endpoint sent the API token in cleartext. The `api_url` validation stops this.

## Done: v1.1.0

- The acceptance tests run against the live NodePing API for each resource. A table test
  creates each supported check type. The tests found defects in each resource type, and
  these defects are fixed. [assumptions.md](assumptions.md) records the API behavior, from
  A-016.
- The shared client handles responses. Each resource handles `{}` with `200` and
  `{"error"}` with `200` in the same way.
- An import of a large existing account tree causes no drift. This includes disabled
  checks, owner and login contacts, schedules with days off, and subaccounts (A-028).

## Next

| Item | Description |
|---|---|
| Public registry releases | CI builds each tagged release, signs the checksums and publishes to the OpenTofu Registry and the Terraform Registry. See [releasing.md](releasing.md). |
| Generated docs | Maintainers write the pages in `docs/resources/` and `docs/data-sources/` manually. `internal/provider/docs_drift_test.go` finds schema drift. Generation with `tfplugindocs` would remove the manual work. |

## Later

| Item | Reason it is not done |
|---|---|
| Check token regeneration (A-011) | It needs a separate resource or action. An attribute cannot do it. |
| Upper bounds for `threshold` and `sens` | We could not confirm the maximum values that NodePing documents. We did not invent a validator. |
| Removal of the deprecated `password` | Users must first move to `password_wo`. |
| Drift detection for subaccount `contactname` and `email` (A-005) | NodePing does not return these fields. A possible method is in the next paragraph. |
| First-class ad-hoc maintenance (A-003) | OpenTofu has no equivalent of Terraform `action` blocks. OpenTofu has said that it may not add one (opentofu#3309). |

A possible method for subaccount drift detection is not verified. NodePing creates a primary
contact when it creates a subaccount. Thus `GET /contacts?customerid=X` may show the values.
But a user can edit or delete that contact independently of the account. This method is
therefore a heuristic only.

Until OpenTofu adds an equivalent of `action` blocks, use the `terraform_data` and
provisioner recipe for ad-hoc maintenance.

## Known limitations

[assumptions.md](assumptions.md) has the full list. These limitations are the most likely to
surprise you:

| Limitation | Entry | Effect |
|---|---|---|
| Optional + Computed attributes do not show a diff when you remove them | A-007 | Set the attribute to its zero value. Do not delete it from the configuration. |
| Ad-hoc maintenance is not manageable | A-003 | The provider enforces this. It is not only documented. |
| Subaccount `contactname` and `email` are never refreshed | A-005, A-033 | NodePing does not return them. The provider refuses a change to either on an existing subaccount. |
| REDIS host passwords have no drift detection | A-013 | The provider never reads a credential back into state. |
| NodePing stores check passwords in plaintext | A-001 | This provider cannot change this. |
