# Roadmap

Ordered by what blocks depending on this provider in production. Everything in "Done"
landed in the initial fork commit.

## Done

- Split `mapCheckToModel` into `mapForRead` / `mapForApply`, retiring the whole
  "inconsistent result after apply" class and deleting the accumulated per-field hacks.
- Contact address ordering and identity — addresses were ordered by Go map iteration and
  updated by list position, so editing one address could silently rewrite another.
- Delete is idempotent; a resource removed out-of-band no longer wedges state.
- `POST` is no longer retried on response-bearing failures, which was creating duplicate
  checks orphaned from state.
- Type-tolerant decoding of NodePing's string-or-number fields. A string `interval`
  previously failed the decode of the entire check, breaking `ListChecks` outright.
- `customer_id` is per-resource, so subaccount import produces state that survives the
  next plan.
- `default_tags` reads config rather than plan, so tag removal is visible.
- Coverage 11.3% → 92.1%. All nine `govulncheck` findings cleared.
- Release pipeline rebuilt on Bitbucket; versioned binaries, reproducible builds, secret
  scanning, coverage gate.

## Next — before we depend on this

**1. Write-only credentials.** Convert `password` to `password_wo` + `password_wo_version`.
Verified working on OpenTofu 1.12.5: the plaintext reaches the provider via `req.Config`,
`null` lands in state, and the value does not appear in a saved plan file. Requires
reading from `req.Config` in Create/Update, and `Read` must never copy the API's plaintext
password into a regular attribute. Until this ships, check passwords are in state in
cleartext. Track opentofu#3964 — `tofu test` with `mock_provider` wrote write-only values
to state; confirm fixed at our version floor, since consumers may mock this provider.

**2. Acceptance tests.** `terraform-plugin-testing` is not yet a dependency, so nothing
exercises the provider against a real `tofu` binary. Highest-value cases:
`ImportStateVerify: true` for both resources (would have caught most of the import and
normalization bugs on its own), the `default_tags` removal path, and address editing
against a live contact. Needs a dedicated NodePing test subaccount.

**3. Verify the root-account `customer_id` assumption.** Root-account resources now send
`customerid=<own account id>` on every call. NodePing treats `customerid` as an account
selector so this should be a no-op, but it is unverified against the live API. First
acceptance test to write.

**4. Provider mirror.** The S3 network mirror is a hard dependency, not a convenience —
goreleaser cannot publish to Bitbucket, and private publishing to `registry.opentofu.org`
is impossible (public repos only). Needs: the static index layout
(`index.json` + `<version>.json` per the network mirror protocol), a generator step in the
release pipeline, and Bitbucket OIDC to an AWS role so the upload uses no stored access
keys. Mirrors require no GPG signing, so the signing machinery is optional and could be
dropped to remove the key-management burden.

**5. Empty-tag clearing.** `CheckCreateRequest.Tags` is now a pointer so an empty slice is
transmissible, but the round trip is untested against the live API.

## Resource coverage

NodePing exposes seven full-CRUD resources. Two are implemented.

| Resource | Status | Notes |
|---|---|---|
| checks | done | |
| contacts | done | |
| **contactgroups** | missing | Members are **address IDs, not contact IDs** |
| **schedules** | missing | `PUT` creates *and* updates; there is no `POST` |
| **notificationprofiles** | missing | Entire `notifications` array must be sent on update |
| **maintenance** | missing | Ad-hoc records are auto-deleted after the run — plan for the drift |
| **accounts / subaccounts** | missing | `POST` creates subaccounts only |

Priority order is contact groups and schedules first: a check's `notifications` array
references contact *or group* IDs plus a schedule name, so without them you can express
"notify this address" but not "notify the on-call group during business hours." Maintenance
next, so deploys can suppress checks. Subaccount provisioning last — it is the least
frequent operation and the highest blast radius.

Two modelling hazards to design around before writing code:

- A contact's `addresses` map must be sent **in full** on every update; omitted entries are
  deleted. There is no separate address-delete call.
- Ad-hoc maintenance records vanish server-side once the run completes, so a naive `Read`
  will see them as deleted.

## Check schema gaps

The client supports these fields; the resource does not expose them. Each blocks the
corresponding check type from being fully managed:

`data` / `fields` (CLUSTER, HTTPPARSE, AUDIO, SNMP, MONGODB, MYSQL, PGSQL) ·
`hosts` / `redistype` / `sentinelname` (REDIS) · `checktoken` (PUSH, AGENT) ·
`oldresultfail` (PUSH, AGENT) · `edns` (DOHDOT) · `dohdot` · `whoisserver` (WHOIS) ·
`rdapurl` (RDAP) · `ignore` (RBL, SIP) · `verifyvolume` / `volumemin` (AUDIO)

All 33 check types are accepted by the `type` validator, but a type whose specific fields
are missing can only be created with defaults.

## Later

- **Resource identity** (OpenTofu 1.12) — enables `import { identity = {...} }`. Safe in
  both directions: older versions never call the RPCs, 1.12 treats `Unimplemented` as a
  warning.
- **`internal/provider` coverage** is 14.6%, the lowest package. Configure-time validation
  and env-var precedence are untested.
- **`api_url` scheme validation** — a plaintext `http://` URL downgrades the bearer token
  to cleartext. Operator-controlled rather than attacker-controlled, so it is a footgun
  rather than a vulnerability, but it deserves a guardrail.
- **Generated docs.** `docs/resources/*` and `docs/data-sources/*` are hand-written and
  already drift from the schema. Add `tfplugindocs` and a CI drift check.
- **Upper bounds for `threshold` and `sens`** — NodePing's documented maxima could not be
  confirmed, so no validator was invented.

## Known limitations

- **Optional + Computed has a cost.** Deleting an attribute from configuration produces no
  diff, because the framework substitutes prior state. This is the accepted trade for
  eliminating apply-consistency errors; attributes with schema defaults are unaffected.
- **Check passwords are plaintext on NodePing's side** regardless of what this provider
  does. Their API stores and returns them in the clear. Use least-privilege credentials.
- **Cross-field validation is deliberately lenient** where NodePing's docs contradict
  themselves — for example `contentstring` is permitted on DNS and DOHDOT because the
  documented DNS example uses it, even though the field table implies otherwise.
- **`tofu test` with `mock_provider`** may write write-only attributes to state
  (opentofu#3964). Affects consumers who mock this provider in their own module tests.
