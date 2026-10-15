# Changelog

All notable changes to this provider are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versioning is
[SemVer](https://semver.org/).

## [Unreleased]

First public release from `github.com/atlanticbt/terraform-provider-nodeping`, published
to the OpenTofu registry as `atlanticbt/nodeping`.

### Breaking changes

- The provider no longer reads the API token from `NODEPING_API_KEY`. Set `api_token`
  or `NODEPING_API_TOKEN`. The fallback was never documented, and a credential source
  users do not know about can pick up a more privileged token than intended.

### Security

- Built with Go 1.26.9 and `golang.org/x/net` v0.60.0, which fix nine vulnerabilities
  reachable from the provider's HTTP client (GO-2026-6603 to GO-2026-6617).

### Fixed

- `max_retries = 0`, `retry_wait_min = 0` and `retry_wait_max = 0` are honoured instead
  of being replaced by the defaults.
- Responses over 16 MiB fail with a clear error naming the endpoint, instead of a
  truncated-JSON error; the limit was 1 MiB.
- API error messages are no longer cut in the middle of a multi-byte character.

### Added

- Length limits on check `label`, `target`, `contentstring` and `postdata`, and contact
  `name`, `address` and `data`, checked at plan time. NodePing enforces none, so the
  limits are the provider's own, set well above values seen in practice.
- GitHub Actions CI (format, vet, golangci-lint with security linters, race tests,
  govulncheck, gitleaks), signed releases, and a weekly live acceptance run.

### Changed

- Resource identity and create-recovery logic is shared in one package, and the check
  resource is split into files by concern. No behaviour change.

## [1.1.0] - 2026-10-05

Verified against the live NodePing API. Every resource now has acceptance tests, and the
defects they and a large production import found are fixed. Observed API behaviour is
recorded in `docs/assumptions.md` (A-016 to A-033).

### Fixed

- `nodeping_schedule` could not create a schedule: the identifier was taken from the
  response body, which carries the account ID instead.
- `nodeping_maintenance` could not create, update or read windows, which NodePing returns
  keyed by ID inside a wrapper.
- `default_tags` produced an invalid plan for checks with tags of their own; the merged set
  is now the computed `tags_all`.
- Untagged checks planned a `tags_all` change on every run.
- Disabled checks were unreadable because NodePing returns `queue` as `false`.
- Schedules with days off were unreadable because NodePing returns `time1` as `false`.
- Owner and login contacts, contacts with object-valued webhook query strings, and
  subaccounts could not be imported.
- Zero values of `sens`, `statuscode`, `warningdays`, `volumemin` and `port` were stored
  as empty, because NodePing treats a numeric `0` as empty; they are now sent as `"0"`.
- WHOIS and RDAP intervals below NodePing's minimum drifted forever.
- `suppress_diag = true`, replacing a contact's whole address set, and nameless contacts
  produced inconsistent results.
- A failed create could leave an object in NodePing with nothing in state pointing at it.

### Changed

- Changing `contactname` or `email` on an existing subaccount is now a plan-time error
  instead of a replacement, which would have deleted everything in the subaccount.
- `sendheaders`, contact `address`, `headers` and `querystrings` are no longer sensitive,
  so generated configuration can round-trip them (A-015).

## [1.0.0] - 2026-08-27

First Atlantic BT release. Completes API coverage, keeps credentials out of state, and
brings test coverage from 11.3% to 95.2%.

Forked from `github.com/phizzl/terraform-provider-nodeping` at commit `8fdc0e1` (MPL-2.0).
See [NOTICE.md](NOTICE.md) for attribution and provenance.

### Added

**Five new resources: API coverage goes from 2 of 7 to 7 of 7**

- `nodeping_contactgroup`: groups of contact addresses for notification routing.
  Members are contact **address** IDs, not contact IDs; a contact has many addresses and it
  is the address that joins a group.
- `nodeping_schedule`: named notification windows. NodePing has no `POST` for schedules;
  `PUT` performs both create and update, and the ID is caller-chosen, so renaming forces
  replacement. Days are modelled as seven optional nested attributes rather than a map, so
  a typo is a plan-time error instead of a silent no-op.
- `nodeping_notificationprofile`: reusable notification recipient sets. Reuses the check
  resource's notification reconciliation, so ordering is deterministic and repeated blocks
  for the same contact are not collapsed.
- `nodeping_maintenance`: scheduled maintenance windows.
- `nodeping_subaccount`: subaccount provisioning. `POST` creates subaccounts only; the
  parent account is not manageable as a resource.

**Ten new data sources:** singular and plural forms of each new resource, plus
`nodeping_account` / `nodeping_accounts`.

**Write-only check credentials**

- `password_wo` and `password_wo_version` on `nodeping_check`. The plaintext reaches
  NodePing but is never written to state or to a saved plan file. Verified against OpenTofu
  1.12.5: schema loads, `req.Config` carries the value, state holds `null`, and the secret
  does not appear in `tofu plan -out`.
- Read from `req.Config` only; `req.Plan` and `req.State` are always null for write-only
  attributes.
- `Read` no longer maps the password NodePing returns into any regular attribute, so the
  API's plaintext cannot leak back into state.
- REDIS host credentials get their own write-only path; see below.
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
import. These compile and are `TF_ACC`-guarded but **have not been run**; see Known
Limitations.

**`docs/assumptions.md`:** fourteen recorded decisions and accepted risks.

**Resource identity** (OpenTofu 1.12)

All seven resources implement `resource.ResourceWithIdentity`, enabling
`import { identity = { ... } }` blocks instead of string-parsing import IDs. Identity is
`id` + `customer_id` on the six scoped resources and `id` alone on `nodeping_subaccount`,
which *is* the account. Safe in both directions: a pre-1.12 core never calls the identity
RPCs, and the string import forms (`ID` and `CUSTOMERID:ID`) continue to work unchanged.

Identity is set in Create, Read, **Update**, and ImportState. Update is not optional: the
framework raises a hard "Missing Resource Identity After Update" error if the identity is
fully null after an update, which is reachable for a resource created on a pre-identity
core and then applied with `-refresh=false`.

**Provider configuration validation**

- `api_url` must use `https` unless the host is loopback. A plaintext endpoint sends the
  API token as HTTP basic auth in cleartext on every request; there was previously no
  guardrail at all. Checked both on the attribute and again in `Configure`, because
  `NODEPING_API_URL` is invisible to attribute validators and the environment variable is
  where the footgun actually lives. No `allow_insecure_url` escape hatch: a config flag
  survives copy-paste into production, and an SSH tunnel to `127.0.0.1` covers the
  legitimate cases without leaving a permanent artifact.
- `rate_limit` must be greater than zero; `max_retries`, `retry_wait_min`, and
  `retry_wait_max` must be non-negative; `retry_wait_min` must not exceed `retry_wait_max`.
  Previously any value `<= 0` was silently coerced to a default, so `rate_limit = -1` gave
  you 10 req/s with no warning.
- `default_tags` rejects empty strings. A zero-length tag cannot be searched or filtered in
  NodePing and is almost always an unset variable.

**Write-only REDIS host credentials**

`hosts[*].password_wo` and `hosts[*].password_wo_version` keep REDIS host passwords out of
state entirely, resolving what was previously an accepted drift-detection gap. `WriteOnly`
inside a `MapNestedAttribute` is supported in terraform-plugin-framework v1.19.0 (verified
in the framework source, not assumed), but only when the containing map attribute is not
`Computed`, so this depended on the `hosts` demotion below. `hosts[*].password` remains for
migration and is deprecated.

**Documentation drift check**

A test walks every resource and data source schema and fails the build when an attribute
exists in a schema but is not mentioned in its documentation page. It found three real gaps
on its first run (`email` on the check resource, `dep` on both check data sources). The
pages remain hand-written rather than generated, because tfplugindocs would discard the
prose they carry; migrating with templates that preserve it is tracked in the roadmap.

**Ad-hoc maintenance escape hatch**

`scripts/nodeping-adhoc-maintenance.sh` performs the API call imperatively, driven from a
`terraform_data` resource with a `local-exec` provisioner (recipe in the maintenance
resource docs). Deliberately outside provider state, which is what makes it safe. OpenTofu
never tries to reconcile the window, so the recreate loop that makes ad-hoc unmanageable
cannot occur. The script validates and bounds every input, rejects check IDs containing
anything but alphanumerics and hyphens rather than escaping them, requires an `https`
endpoint outside loopback, and reads the token from the environment so it never reaches
`ps` output.

**Release tooling** for publishing builds. It was replaced in 1.1.x by tag-triggered CI
releases to the public registries.

### Changed

- `.gitignore` broadened to cover plan files (binary and JSON), state including backups and
  lock metadata, `*.tfvars` (which routinely holds secrets), and local CLI config
  (`.tofurc`, `.terraformrc`). The comment explains why, so the patterns survive future
  tidying.
- `CheckCreateRequest.Tags` is now `*[]string`. A plain slice with `omitempty` dropped an
  empty array, making "remove the last tag" a silent no-op.
- `terraform-plugin-testing` v1.16.0 added as a real dependency.
- **GPG signing temporarily disabled in the release pipeline.** Registry releases sign
  their checksums again.

- **Attribute removal now produces a diff for 24 check attributes.** `Optional + Computed`
  was over-applied. It is only required where NodePing supplies a value the practitioner
  never set; for attributes the API merely echoes back, plain `Optional` works and removal
  diffs correctly. Demoted: `description`, `contentstring`, `sendheaders`, `receiveheaders`,
  `postdata`, `data`, `fields`, `hosts`, `edns`, `username`, `email`, `query`, `database`,
  `namespace`, `whoisserver`, `rdapurl`, `ignore`, `servername`, `sshkey`, `clientcert`,
  `sentinelname`, `dnstoresolve`, `dep`, and the deprecated `password`. Attributes with
  documented server defaults, schema defaults, or unconditional API responses keep
  `Computed`: a wrong demotion is a hard apply failure on every apply, so uncertainty
  resolved in favour of keeping it. See A-007 for the kept-with-reason list.

### Fixed

- **A 200 response carrying `{"error": ...}` could delete a resource from state and
  recreate live infrastructure.** NodePing returns errors with HTTP 200 on some endpoints.
  `nodeping_contactgroup`, `nodeping_notificationprofile`, `nodeping_maintenance`, and
  `nodeping_subaccount` checked only whether the decoded document was empty, and an error
  document decodes to an empty struct. A transient `permission denied` or `rate limit
  exceeded` was therefore reported as "resource absent", so OpenTofu would drop it from
  state and rebuild it on the next apply. Not-found classification is now conservative: a
  short fragment list, skipped entirely for retryable statuses, with explicit tests
  asserting that unrecognised error messages surface as real errors.
- **`nodeping_check` and `nodeping_contact` did not tolerate NodePing's non-standard
  responses at all**, so a `{}`-with-200 was written into state as an empty object rather
  than removing the resource. Both layers of tolerance now live in the shared client, so
  every resource inherits identical behaviour: one error-in-200 detector, one not-found
  classifier, replacing sixteen duplicated blocks.
- **`ListChecks` and the other list methods turned a 200-carried error into an empty
  collection**, which is indistinguishable from an empty account. They now return the error.
- **Provider configuration rejected valid tokens sourced from another resource.**
  `Configure` checked only `IsNull()`, so an attribute whose value was not yet known
  (`api_token` wired from another resource's output, for example) was read as an empty
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
  tests exist for every resource but have never run; that requires a token for a dedicated
  test subaccount. This is the gate on production use, and it is the most significant item
  on this list by a wide margin. See A-010.
- **Ad-hoc maintenance cannot be a resource.** NodePing auto-deletes the record after the
  run, which would produce an infinite recreate loop that repeatedly disables monitoring.
  Blocked structurally rather than documented, with an imperative escape hatch via
  `terraform_data` and `scripts/nodeping-adhoc-maintenance.sh`. Not expected to change:
  the declarative answer would be Terraform's `action` blocks, which OpenTofu has signalled
  it may deliberately not adopt. See A-003.
- **Some attributes still do not diff on removal.** Narrowed substantially this release:
  24 check attributes were demoted to plain `Optional` and now behave correctly. What
  remains is attributes where NodePing supplies a value the practitioner never set; for
  those, set the zero value explicitly rather than deleting the line. See A-007 for the
  list and the reason each was kept.
- **Subaccount `contactname` and `email` are never refreshed**: NodePing does not return
  them, so drift in those fields is invisible. See A-005.
- **`checktoken` regeneration is unsupported.** NodePing's documentation is
  self-contradictory on this attribute; the correct shape is a separate resource, not an
  attribute. See A-011.

---

## [0.1.0] - 2026-08-27

Initial Atlantic BT fork. Correctness, security, and build fixes carried over from the
upstream audit; no new resources. See the initial commit message for detail.
