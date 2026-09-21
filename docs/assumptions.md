# Assumptions and Accepted Risks

Non-obvious decisions taken in this provider, with the reasoning. Per the ABT golden
rules, anything that would surprise a reviewer belongs here.

---

## A-001 — NodePing stores check credentials in plaintext on their side

**Assumption:** Credentials on authenticated checks (`FTP`, `IMAP4`, `MYSQL`, `POP3`,
`SMTP`, `SSH`) are stored and returned in cleartext by NodePing, and nothing this provider
does can change that.

**Why:** Their API requires the plaintext to authenticate against the monitored target, and
their own documentation advises least-privilege credentials for exactly this reason.

**Mitigation, in force:** `password_wo` is a write-only attribute — verified on OpenTofu
1.12.5 to keep the value out of both state and saved plan files. Pair it with an
`ephemeral` block when sourcing from a secret store; a regular `data` source would persist
the secret in state even though the check attribute does not. REDIS `hosts[*].password`
gets the same treatment (see A-013).

**Residual risk:** the deprecated `password` attribute still writes to state in plaintext
and is retained only for migration. `Sensitive: true` redacts CLI output and does **not**
protect state — it is not a mitigation on its own. Consumers should enable OpenTofu state
encryption regardless, and must use monitoring-only credentials with minimum privilege,
because NodePing's own storage remains cleartext.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-08-27

---

## A-002 — NodePing violates HTTP conventions; the client compensates

**Assumption:** API responses cannot be trusted to follow REST conventions, so the client
layer normalises them.

**Why:** Observed across multiple endpoints during implementation:

- Absent resources may return `{}` with **HTTP 200** rather than 404
- Errors may return `{"error": "..."}` with **HTTP 200** rather than 4xx
- Numbers (`interval`, `sens`, `threshold`, `port`, `warningdays`) are returned as strings
  in some responses and numbers in others
- Booleans appear as `true`, `1`, or `"1"` interchangeably
- Identifier keys vary by endpoint (`id` vs `_id`, `customer_id` vs `customerid`)

A string-valued `interval` originally failed JSON decoding for the entire check struct,
which broke `ListChecks` outright.

**Mitigation:** Type-tolerant decoding and not-found classification live in the client
layer so every resource inherits the same behaviour and there is one place to fix it.
Concretely, in `internal/client`:

- `errorMessageFromBody` is the only detector for an error carried by a 2xx.
  `executeRequest` runs it on every successful response and converts a hit into an
  `*APIError`, so no endpoint can decode an error document into a resource struct.
- `isNotFoundMessage`, reachable only through `APIError.IsNotFound`, is the only
  classifier for NodePing's not-found wording.
- `notFoundOrWrap` maps an API not-found signal to `*NotFoundError` and wraps everything
  else; `absentAsNotFound` handles the "decoded fine but the document is empty" case
  against a per-resource emptiness predicate.

**Accepted risk, deliberately conservative:** message matching is restricted to a short
fragment list and is skipped for retryable statuses (429 and 5xx). A transient upstream
failure whose text happens to contain "not found" must stay a real error — classifying it
as absence would make OpenTofu drop the resource from state and recreate live
infrastructure on the next apply. The cost of the narrow list is that an unrecognised
absence phrasing surfaces as an ordinary error, which is the safe direction to fail.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-08-27

---

## A-003 — Ad-hoc maintenance is not manageable as a resource

**Assumption:** `nodeping_maintenance` supports scheduled (cron) windows only. Ad-hoc
windows cannot be created or imported.

**Why:** NodePing auto-deletes ad-hoc maintenance records once the run completes. A
declarative resource would see the record vanish, remove it from state, and recreate it on
the next plan — an infinite loop that repeatedly disables the customer's monitoring. There
is no honest way to express "gone is expected and terminal" in the plugin framework: the
resource would sit in state describing an object that does not exist, and later edits to
`duration` or `checklist` would silently do nothing.

**Mitigation:** Enforced structurally rather than documented — `cron` is Required, the
create request cannot express `id: "ad-hoc"`, import rejects the ad-hoc ID before any API
call, and update refuses an ad-hoc ID. Ad-hoc windows remain visible through both data
sources, and the plural one exposes an `ad_hoc` boolean.

**Escape hatch:** `scripts/nodeping-adhoc-maintenance.sh` performs the API call
imperatively, intended to be driven from a `terraform_data` resource with a `local-exec`
provisioner (recipe in `docs/resources/nodeping_maintenance.md`). This is deliberately
outside provider state, which is what makes it safe — OpenTofu never tries to reconcile
the window, so it cannot loop. The script validates and bounds every input, rejects check
IDs containing anything but alphanumerics and hyphens rather than escaping them, requires
an `https` endpoint outside loopback, and reads the token from the environment so it never
appears in `ps` output.

**Why this is not obviously temporary:** the declarative answer would be Terraform's
`action` blocks (1.14). OpenTofu has not implemented them and has signalled it may diverge
rather than adopt the design (opentofu#3309), so there is no dated horizon for replacing
the workaround with a first-class resource.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-08-27

---

## A-004 — `accountsuppressall` is not exposed

**Assumption:** The account-wide notification suppression flag is omitted from
`nodeping_subaccount`.

**Why:** No read operation returns it, so it could never be refreshed and would drift
permanently. Re-enabling notifications does not restore contacts that were suppressed
individually, so setting it to `false` does not describe a state the provider can
guarantee. It is an incident-response switch, not configuration.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-08-27

---

## A-005 — Subaccount POST-only fields are tracked from configuration only

**Assumption:** `contactname`, `email`, `emailme`, `autodiagnotifications`, and `location`
are never mapped from API responses.

**Why:** NodePing accepts them on create but does not return them on read. Mapping them
would produce permanent drift.

**Mitigation:** They use `RequiresReplaceIf` treating null prior state as "no change"
rather than a plain `RequiresReplace`. Import cannot recover these values, so a plain
modifier would read `null -> "value"` as a change and **destroy a freshly imported
subaccount**. With the conditional modifier, the first apply after import simply records
them.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-08-27

---

## A-006 — `lam` is accepted for subaccount `location` despite the docs omitting it

**Assumption:** The Latin America region code is valid on subaccount creation.

**Why:** NodePing's account `POST` documentation lists `nam`, `eur`, `eao`, `wlw` and omits
`lam`, but both account `PUT` and check `runlocations` document `lam`. Treated as a
documentation bug rather than a real constraint. Rejecting it would block valid configs.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-08-27

---

## A-007 — Optional + Computed suppresses attribute-removal diffs, where it is genuinely needed

**Assumption:** `Optional + Computed` is used only where NodePing supplies a value the
practitioner never set — a documented server-side default, a schema `Default`, or a field
the API returns unconditionally. Those attributes do not diff when removed from
configuration.

**Why:** The framework substitutes prior state when config goes null on an Optional +
Computed attribute. The alternative for a server-defaulted field — Optional-only — turns
every server-side normalisation into a hard "inconsistent result after apply" on *every*
apply, which is strictly worse than a suppressed removal diff.

**Scope reduced:** this was originally over-applied. Twenty-four `nodeping_check`
attributes that NodePing only echoes back when set (`description`, `contentstring`,
`sendheaders`, `receiveheaders`, `postdata`, `data`, `fields`, `hosts`, `edns`, `username`,
`email`, `query`, `database`, `namespace`, `whoisserver`, `rdapurl`, `ignore`, `servername`,
`sshkey`, `clientcert`, `sentinelname`, `dnstoresolve`, `dep`, and the deprecated
`password`) are now plain `Optional` and diff correctly on removal. Each was verified
against the mapping functions to confirm the model goes null when the API omits the field.

**Kept deliberately, conservative:** boolean attributes without a schema default (`regex`,
`invert`, `follow`, `ipv6`, `verify`, `dnsrd`, `oldresultfail`, `verifyvolume`) because
NodePing's null-versus-`false` behaviour is unconfirmed; `secure`, which NodePing returns
as both a boolean and a string; `homeloc` and `runlocations`, which NodePing assigns when
unset; `tags`, whose planned value is written by `ModifyPlan` and so may not be
non-Computed; `target`, `label`, and `customer_id`, which the API returns unconditionally;
and `schedule.time1`/`time2` and `contactgroup.name`/`members`, whose mapping overwrites
the model from the API regardless of plan. A wrong demotion costs a hard apply failure, so
uncertainty resolves in favour of keeping Computed.

**Mitigation for what remains:** to clear one of the still-Computed attributes, set it
explicitly to its zero value rather than deleting the line.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-08-27

---

## A-008 — Cross-field validation is lenient where NodePing's docs contradict themselves

**Assumption:** Some type-specific fields are permitted on more check types than the field
table implies.

**Why:** NodePing's "fields by check type" table conflicts with its own prose and worked
examples. `contentstring` is allowed on DNS and DOHDOT because the documented DNS example
uses it. `CLUSTER` is in the no-target-required set. An over-tight validator blocks valid
configurations, which is a worse failure than a permissive one that surfaces at apply.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-08-27

---

## A-009 — Protocol 6 support in OpenTofu is observed, not contractually promised

**Assumption:** Shipping a protocol-6 provider against OpenTofu is safe.

**Why:** OpenTofu's published v1.x compatibility promise names **protocol 5 only**.
Protocol 6 support exists in source (`internal/getproviders/registry_client.go` gates
`>= 5, < 7`) and was verified working on 1.12.5, and terraform-plugin-framework is
protocol-6-native, so every modern framework provider depends on it.

**Mitigation:** If a contractual guarantee is ever required, `terraform-plugin-mux` can
serve both protocol versions.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-08-27

---

## A-010 — Acceptance tests have not been executed against the live API

**Assumption:** Every resource in this provider is unit-verified but **not** API-verified.

**Why:** Acceptance tests require a NodePing API token for a dedicated test subaccount,
which was not available during implementation. The tests are written and compile, but have
never run against NodePing.

**Specifically unverified:** that root-account resources tolerate `customerid=<own account
id>` on every call (assumed to be a no-op account selector); that clearing all tags via an
empty array works; and every response-shape assumption in A-002.

**Mitigation:** Provision a dedicated test subaccount and run `make test-acc` before
depending on this provider for production monitoring. This is the gate on a real v1.0.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-08-27

---

## A-011 — `checktoken` is read-only; token regeneration is not supported

**Assumption:** The AGENT/PUSH check token is exposed as a Computed, Sensitive attribute
and cannot be regenerated through the provider.

**Why:** NodePing's documentation is self-contradictory — it describes `checktoken` as
read-only while also saying you set it to the literal string `"reset"` to request
regeneration. Modelling that as Optional + Computed produces either a permanent diff
(config `"reset"` vs the real token in state) or an inconsistent-result-after-apply error,
because the value the API returns can never equal the value the user wrote.

**Mitigation:** Regenerate tokens in the NodePing UI. If this becomes a real requirement,
the correct shape is a separate single-purpose resource or an ephemeral action, not an
attribute on the check.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-08-27

---

## A-012 — Type-specific defaults are gated on check type before being sent

**Assumption:** `redistype` and `dohdot` carry schema defaults that apply regardless of the
check's `type`, so the outbound request suppresses them unless the type matches.

**Why:** Framework defaults are unconditional. Without gating, an HTTP check would plan
`redistype = "standalone"` and send it to the API. The mapping function applies the same
default when the API omits the field — without that symmetry, refresh maps null and every
subsequent plan shows a permanent diff.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-08-27

---

## A-013 — RESOLVED: REDIS host credentials are write-only

**Status:** resolved, not accepted. Superseded by `hosts[*].password_wo`.

**Original problem:** `hosts[*].password` was written but never mapped from API responses,
so a host password changed outside OpenTofu was not detected as drift.

**Resolution:** `hosts[*].password_wo` and `hosts[*].password_wo_version` keep the
credential out of state entirely, so there is nothing to drift. `WriteOnly` inside a
`MapNestedAttribute` is supported in terraform-plugin-framework v1.19.0 — verified in the
framework source (`map_nested_attribute.go:317-323`), not assumed — **provided the
containing map attribute is not `Computed`**. Demoting `hosts` to plain `Optional` under
A-007 is what made this legal; the two changes are coupled.

Per-host conflict rules are enforced by a custom validator rather than
`resourcevalidator.Conflicting`, which cannot express a path through a map.

**Residual:** `hosts[*].password` remains for migration and is deprecated. It still writes
to state in plaintext and is still never read back, so the original trade-off applies to
anyone who has not migrated.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-08-27

---

## A-014 — `fields.match` is exposed on all field entries

**Assumption:** The `match` sub-attribute appears on every `fields` entry, though NodePing
honours it only for PGSQL, MYSQL, and MONGODB.

**Why:** A per-check-type nested attribute shape is not expressible in a single schema. The
attribute description and resource documentation state where it takes effect.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-08-27

---

## A-015 — Header, query-string and address values are not marked sensitive

**Assumption:** `nodeping_check.sendheaders`, `nodeping_contact.address.address`,
`address.headers` and `address.querystrings` are readable in plan output and in generated
configuration, in exchange for being importable.

**Why:** `tofu plan -generate-config-out` writes `null # sensitive` for every attribute marked
`Sensitive`. Against a live subaccount of 35 checks and 9 contacts that produced two failures.
All 9 contacts were unimportable, because `address` is Required and a required attribute set to
`null` is a configuration error. And 9 checks planned an in-place update that would have deleted
their live `sendheaders` — silently, since the diff shows `(sensitive value)` on both sides.

This is not a hypothetical trade-off. Four checks in the subaccount used for the live run carry
Cloudflare Access service tokens in `sendheaders`, and the first plan after the change printed
`CF-Access-Client-Secret` in cleartext and wrote it into `generated.tf`. The decision was taken
again with that in hand, and stands.

`Sensitive` only redacts CLI output. It never kept the header out of state, and the value is
visible in the NodePing UI to anyone who can read the check. What it bought was an import path
that deletes production configuration on apply. A contact address is PII rather than a
credential and the same reasoning applies with less at stake.

`address.headers` and `address.querystrings` are Optional, so a sensitive marking did not make a
contact unimportable — it made the import lossy instead, planning `(sensitive value) -> null` on
two live webhook contacts. Same failure, quieter. They are treated the same way for consistency:
one rule for anything that config generation has to round-trip.

Credential attributes stay sensitive: `password`, `password_wo`, `snmpcom`, `clientcert`,
`sshkey`. They are write-only or Computed, so generation never has to reproduce them.

**Operational consequence:** a file written by `tofu plan -generate-config-out` against an
account using header authentication contains live credentials. Treat it as secret-bearing —
scrub it before it goes near version control, and prefer a variable reference over the literal
when folding it into a root module.

**Recorded by:** Doug Eubanks · **Date:** 2026-09-21

---

## A-016 — `warningdays` has no lower bound

**Assumption:** The provider enforces no minimum on `warningdays`.

**Why:** The schema declared `AtLeast(1)`. A live SSL check stores `warningdays = 0`, so importing
it failed on a rule NodePing does not have. NodePing's real minimum is undocumented, and the same
reasoning already applied to the `threshold` and `sens` upper bounds: a bound the API does not
enforce turns existing resources into unmanageable ones. Replacing one invented floor with another
would repeat the mistake.

**Recorded by:** Doug Eubanks · **Date:** 2026-09-21

---

## A-017 — Type-applicability validation ignores boolean flags set to false

**Assumption:** `regex`, `invert` and `dnsrd` are rejected on a check type that does not support
them only when set to `true`.

**Why:** NodePing returns these flags as `false` on check types that do not use them. Across 35
live checks, `invert` was present and `false` on 19 of them — PUSH as well as HTTPADV and
HTTPCONTENT — and `ipv6` was present and `false` on PING and PUSH checks. Presence is not
uniform: 6 of the 7 PUSH checks omitted `invert` entirely and one carried `false`, which is the
one that failed. The read path maps what the API sends, so that refreshed PUSH check carried
`invert = false` and the provider's own validator then rejected its own output. `dnsrd` is
covered by the same rule by analogy; no live check in the survey carried it. Skipping false keeps the guardrail where it is useful — a flag someone
turned on for a check type that ignores it is still a mistake worth reporting.

The read path was left alone. Nulling out type-inapplicable attributes would have meant changing
what an Optional + Computed attribute resolves to, which risks turning a suppressed diff into an
inconsistent-result-after-apply failure on every apply (see A-007, A-012).

**Recorded by:** Doug Eubanks · **Date:** 2026-09-21

---

## A-018 — `GET /accounts` returns one document, not a listing, when scoped to a customer

**Assumption:** When the request carries `customerid`, NodePing answers `GET /accounts` with
that single account document, and `ListAccounts` converts it into a one-entry listing keyed by
its `_id`. The document's `parent` is dropped rather than mapped.

**Why:** Confirmed with curl against the live API. The shared client appends `customerid` to
every request whenever the provider is configured with one, which is the normal way to manage a
subaccount, so `nodeping_accounts` failed to decode on every read in that configuration. The
document names the account `customer_name` where the listing uses `name`, and its `parent` holds
the parent account's ID as a string. `AccountSummary.Parent` means something else entirely — the
subaccount count that appears only on the primary account — so copying the ID across would have
reported every subaccount as the primary one. The document carries no check count, so
`active_checks` reads 0 for this shape.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-09-21

---

## A-019 — `GET /maintenance` wraps the windows, and the wrapper decides what is ad-hoc

**Assumption:** `GET /maintenance` returns `customer_id`, `schedules` and `ad-hoc` at the top
level. Both groups are flattened into one ID-keyed map, and a window from the `ad-hoc` group is
marked so `IsAdHoc` reports it regardless of the identifier it carries.

**Why:** Confirmed with curl on two accounts. The previous code decoded the response as a bare
ID-keyed map, so the data source failed on every account. `IsAdHoc` previously matched the
identifier against the `ad-hoc` sentinel, which is what a create accepts, not what a listing
returns: a live ad-hoc window has its own generated ID. The sentinel check is kept for the
single-window `GET` and for the create response, which have no wrapper to consult.

**Residual:** no account reachable by our token has a maintenance window, so both groups were
observed empty. The wrapper is verified; the shape of the window objects inside it is not. They
are still decoded with the existing `Maintenance` struct. A response keyed directly by window ID
is still accepted, because decoding one as the wrapper would return an empty result and an empty
result reads as "every window was deleted".

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-09-21
