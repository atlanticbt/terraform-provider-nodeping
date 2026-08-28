# Changelog

All notable changes to this provider are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versioning is
[SemVer](https://semver.org/).

## [1.0.0] — 2026-08-27

First Atlantic BT release. Completes API coverage, keeps credentials out of state, and
brings test coverage from 11.3% to 94%.

Forked from `github.com/phizzl/terraform-provider-nodeping` at commit `8fdc0e1` (MPL-2.0).
See [NOTICE.md](NOTICE.md) for attribution and provenance.

### Added

**Five new resources — API coverage goes from 2 of 7 to 7 of 7**

- `nodeping_contactgroup` — groups of contact addresses for notification routing.
  Members are contact **address** IDs, not contact IDs; a contact has many addresses and it
  is the address that joins a group.
- `nodeping_schedule` — named notification windows. NodePing has no `POST` for schedules;
  `PUT` performs both create and update, and the ID is caller-chosen, so renaming forces
  replacement. Days are modelled as seven optional nested attributes rather than a map, so
  a typo is a plan-time error instead of a silent no-op.
- `nodeping_notificationprofile` — reusable notification recipient sets. Reuses the check
  resource's notification reconciliation, so ordering is deterministic and repeated blocks
  for the same contact are not collapsed.
- `nodeping_maintenance` — scheduled maintenance windows.
- `nodeping_subaccount` — subaccount provisioning. `POST` creates subaccounts only; the
  parent account is not manageable as a resource.

**Ten new data sources** — singular and plural forms of each new resource, plus
`nodeping_account` / `nodeping_accounts`.

**Write-only check credentials**

- `password_wo` and `password_wo_version` on `nodeping_check`. The plaintext reaches
  NodePing but is never written to state or to a saved plan file. Verified against OpenTofu
  1.12.5: schema loads, `req.Config` carries the value, state holds `null`, and the secret
  does not appear in `tofu plan -out`.
- Read from `req.Config` only — `req.Plan` and `req.State` are always null for write-only
  attributes.
- `Read` no longer maps the password NodePing returns into any regular attribute, so the
  API's plaintext cannot leak back into state.
- REDIS `hosts[*].password` receives the same treatment, restored from prior state on
  refresh.
- The existing `password` attribute is deprecated but functional. The two are mutually
  exclusive, enforced by a config validator.

**Fourteen check attributes that the client supported but the schema did not expose**

`data`, `fields` (with `match`), `hosts`, `redistype`, `sentinelname`, `checktoken`,
`oldresultfail`, `edns`, `dohdot`, `whoisserver`, `rdapurl`, `ignore`, `verifyvolume`,
`volumemin`. REDIS, AUDIO, WHOIS, RDAP, DOHDOT, PUSH, and AGENT checks can now be fully
managed rather than created with defaults.

**Validation**

Plan-time validators for `redistype`, `dohdot`, `volumemin` (−90 to 0), REDIS host ports,
`rdapurl`, and EDNS keys (must parse as positive integers). Cross-field validation requires
`sentinelname` when `redistype = "sentinel"`, and rejects `password_wo_version` without
`password_wo`.

**Acceptance tests** for every resource, including `ImportStateVerify` round trips and a
test asserting the write-only password is absent from state across create, rotate, and
import. These compile and are `TF_ACC`-guarded but **have not been run** — see Known
Limitations.

**`docs/assumptions.md`** — fourteen recorded decisions and accepted risks.

### Changed

- `.gitignore` broadened to cover plan files (binary and JSON), state including backups and
  lock metadata, `*.tfvars` (which routinely holds secrets), and local CLI config
  (`.tofurc`, `.terraformrc`). The comment explains why, so the patterns survive future
  tidying.
- `CheckCreateRequest.Tags` is now `*[]string`. A plain slice with `omitempty` dropped an
  empty array, making "remove the last tag" a silent no-op.
- `terraform-plugin-testing` v1.16.0 added as a real dependency.

### Fixed

- **Provider configuration rejected valid tokens sourced from another resource.**
  `Configure` checked only `IsNull()`, so an attribute whose value was not yet known —
  `api_token` wired from another resource's output, for example — was read as an empty
  string. The practitioner got "Missing NodePing API Token", and a perfectly good
  `NODEPING_API_TOKEN` in the environment was silently discarded in the process. Unknown
  values now produce the framework-recommended "apply the source first" diagnostic for
  `api_token`, `customer_id`, and `api_url`, and unknown tuning values fall back to client
  defaults instead of being read as zero.
- **Contact `mute` could never be turned off.** The update path omitted the field unless it
  was true, so un-muting never reached the API and drifted forever.
- **Remaining bare `NotFoundError` type assertions** in the check resource switched to
  `errors.As`, so they survive future error wrapping.

### Security

- Check credentials no longer reach state when `password_wo` is used (see Added).
- `snmpcom`, `sendheaders`, REDIS host passwords, and webhook `headers`/`querystrings`
  marked `Sensitive`.
- Go toolchain pinned to 1.26.6 and dependencies upgraded; `govulncheck` reports no
  findings.

### Known limitations

- **Nothing in this release has been verified against the live NodePing API.** Acceptance
  tests exist but have never run — that requires a token for a dedicated test subaccount.
  This is the gate on production use. See A-010.
- Ad-hoc maintenance windows cannot be managed. NodePing auto-deletes them after the run,
  which would produce an infinite recreate loop that repeatedly disables monitoring.
  Blocked structurally, not just documented. See A-003.
- Optional + Computed attributes do not diff on removal; set them to a zero value instead.
  See A-007.
- Subaccount `contactname` and `email` are never refreshed — NodePing does not return them.
  See A-005.
- REDIS host password drift is not detected, the cost of never reading a credential back
  into state. See A-013.
- `checktoken` regeneration is unsupported; NodePing's documentation is self-contradictory
  on this attribute. See A-011.
- `nodeping_check` and `nodeping_contact` do not yet share the newer resources' handling of
  NodePing's `{}`-with-HTTP-200 responses. Tracked in the roadmap.

---

## [0.1.0] — 2026-08-27

Initial Atlantic BT fork. Correctness, security, and build fixes carried over from the
upstream audit; no new resources. See the initial commit message for detail.
