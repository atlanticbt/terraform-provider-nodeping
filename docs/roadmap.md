# Roadmap

Ordered by what blocks depending on this provider in production.

## Done — v1.0.0

**API coverage is complete: 7 of 7 NodePing CRUD resources.**

- `nodeping_check`, `nodeping_contact`, `nodeping_contactgroup`, `nodeping_schedule`,
  `nodeping_notificationprofile`, `nodeping_maintenance`, `nodeping_subaccount`, plus
  singular and plural data sources for each and `nodeping_account`/`nodeping_accounts`.
- **Write-only credentials.** `password_wo` + `password_wo_version` keep check passwords
  out of state and out of saved plan files. REDIS `hosts[*].password` gets the same
  treatment.
- Check schema gaps closed: `data`, `fields`, `hosts`, `redistype`, `sentinelname`,
  `checktoken`, `oldresultfail`, `edns`, `dohdot`, `whoisserver`, `rdapurl`, `ignore`,
  `verifyvolume`, `volumemin`.
- Correctness work carried over from the fork: the `mapForRead`/`mapForApply` split,
  contact address ordering and identity, idempotent deletes, non-idempotent `POST` no
  longer retried, type-tolerant decoding, per-resource `customer_id`.
- **Resource identity** on all seven resources, enabling `import { identity = {...} }`.
- **Provider config validation** — `api_url` must be `https` outside loopback (a plaintext
  endpoint sent the API token in cleartext with no guardrail), numeric tuning bounds, and
  `default_tags` empty-string rejection.
- **Shared response tolerance in the client** — one error-in-200 detector and one
  not-found classifier for all seven resources, replacing sixteen duplicated blocks. This
  closed a hazard where a `200 {"error": "permission denied"}` was read as "resource
  absent", which would have made OpenTofu delete from state and rebuild live
  infrastructure.
- **Ad-hoc maintenance escape hatch** — `scripts/nodeping-adhoc-maintenance.sh` plus a
  `terraform_data` recipe, deliberately outside provider state.
- Coverage 11.3% → 95%+. All `govulncheck` findings cleared.
- Release pipeline with reproducible builds, secret scanning and a coverage gate.

## Done — v1.1.0

- **Acceptance tests run against the live NodePing API** for every resource, including a
  table test that creates every supported check type. They found and fixed defects in
  every resource type; the observed API behaviour is recorded in
  [assumptions.md](assumptions.md) from A-016 onwards.
- **Response tolerance lives in the shared client**, so every resource handles
  `{}`-with-200 and `{"error"}`-with-200 the same way.
- **Imports of a large existing account tree work without drift**, including disabled
  checks, owner and login contacts, schedules with days off and subaccounts (A-028).

## Next

- **Public registry releases.** Tag-triggered CI builds, signed checksums, and publication
  to the OpenTofu Registry and the Terraform Registry. See [releasing.md](releasing.md).
- **Generated docs.** The `docs/resources/*` and `docs/data-sources/*` pages are
  hand-written; `docs_drift_test.go` catches schema drift, but `tfplugindocs` generation
  would remove the hand-maintenance.

## Later

- **Check token regeneration** (A-011) — needs a separate resource or action, not an
  attribute.
- **Upper bounds for `threshold` and `sens`** — NodePing's documented maxima could not be
  confirmed, so no validator was invented.
- **Deprecated `password` removal** — once consumers have migrated to `password_wo`.
- **Subaccount `contactname`/`email` drift detection** (A-005) — currently impossible,
  because NodePing does not return the fields. One unverified idea: creating a subaccount
  also creates a primary contact, so `GET /contacts?customerid=X` may expose them. The
  contact can be edited or deleted independently of the account, so this is a heuristic at
  best.
- **First-class ad-hoc maintenance** (A-003) — blocked on OpenTofu shipping an equivalent
  of Terraform's `action` blocks. They have signalled possible deliberate divergence
  (opentofu#3309), so this may never arrive. The `terraform_data` + provisioner recipe is
  the answer until it does.

## Known limitations

See [assumptions.md](assumptions.md) for the full register. The ones most likely to
surprise:

- **Optional + Computed suppresses removal diffs** (A-007). Deleting such an attribute from
  config produces no diff; set it to its zero value instead.
- **Ad-hoc maintenance is not manageable** (A-003) and is enforced, not just documented.
- **Subaccount `contactname`/`email` are never refreshed** (A-005) — NodePing does not
  return them, and changing either on an existing subaccount is refused (A-033).
- **REDIS host passwords are not drift-detected** (A-013) — the cost of never reading a
  credential back into state.
- **NodePing stores check passwords in plaintext** (A-001) regardless of what this provider
  does.
