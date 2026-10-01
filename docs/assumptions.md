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
unset; `target`, `label`, and `customer_id`, which the API returns unconditionally;
and `schedule.time1`/`time2` and `contactgroup.name`/`members`, whose mapping overwrites
the model from the API regardless of plan. A wrong demotion costs a hard apply failure, so
uncertainty resolves in favour of keeping Computed.

**Mitigation for what remains:** to clear one of the still-Computed attributes, set it
explicitly to its zero value rather than deleting the line.

**Amended 2026-09-22 (SYS-79):** `tags` was on the kept-Computed list because `ModifyPlan`
wrote a merged value into the plan. That was itself the bug recorded in A-020: Terraform
rejects a planned value that does not match a known configuration value. With the merge
moved to the new computed `tags_all`, `tags` carries nothing the API supplies and is now
plain `Optional`, covered by the removal test table like the other twenty-four.

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

## A-019 — Every maintenance response keys the windows by ID, and the key is the only identifier

**Assumption:** Four maintenance shapes exist and one decoder recognises all of them:

- `GET /maintenance` returns `customer_id`, `schedules` and `ad-hoc` at the top level. Both
  groups are flattened into one ID-keyed map, and a window from the `ad-hoc` group is marked so
  `IsAdHoc` reports it regardless of the identifier it carries.
- `GET /maintenance/:id`, `POST /maintenance` and `PUT /maintenance/:id` all return the window
  filed under its generated ID, beside the scalars `ok` and `message`.
- A bare window document with no wrapper is still accepted.

The envelope key is the authoritative identifier and is stamped onto any window that arrives
without one.

**Why:** The wrapper was confirmed with curl on two accounts, and the previous code decoded the
listing as a bare ID-keyed map, so the data source failed on every account. `IsAdHoc` previously
matched the identifier against the `ad-hoc` sentinel, which is what a create accepts, not what a
listing returns: a live ad-hoc window has its own generated ID. The sentinel check is kept for
the bare-document form, which has no wrapper to consult.

The write and single-read paths were each decoding into a bare `Maintenance` and so found
nothing. The consequences differed and all three were wrong: create failed with "API returned no
id" *after* NodePing had created the window, leaking one per attempt; read found an empty
document and reported not-found, which would have deleted the resource from state on every
refresh; update silently returned a record carrying only the ID, writing an empty name, a zero
duration and no checklist into state. Recognising the envelope in one shared decoder rather than
per call site is what keeps these three from drifting apart again.

An update that returns no window at all is now an error rather than a backfilled empty record,
because a hollow window in state plans the real settings away on the next run.

**Observed inner shape (2026-09-22, live):** a window carries `name`, `enabled`, `processing`,
`cron`, `duration`, `history`, `nextrun`, `checklist` and `update`. `nextrun` is epoch
milliseconds. Only the single-window `GET` includes an `id` inside the document; the listing,
create and update responses do not, which is why the key has to be trusted. The earlier caveat
that no reachable account had a window no longer applies.

**Residual:** `history` was `{}` and `update.user` was `""` on every window observed, so neither
is fully characterised — `MaintenanceRun` and the non-empty `update.user` form remain inferred
from the documentation. No ad-hoc window was created during this work, so the `ad-hoc` group has
still only been seen empty; its grouping behaviour is taken from the documentation.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-09-22

---

## A-020 — `default_tags` is merged into a computed `tags_all`, not into `tags`

**Assumption:** `nodeping_check.tags` holds only what the configuration declares and always
matches it exactly. The provider's `default_tags` are merged into a new computed
`tags_all`, and `tags_all` is the value sent to NodePing.

**Why:** The first live acceptance run failed with "Provider produced invalid plan: planned
value [managed-by-tofu, team-devops, tf-acc] does not match config value [team-devops]".
Terraform requires the planned value of an attribute to equal a known configuration value,
so `ModifyPlan` could not write a merged set into `tags` — the feature broke for any check
that declared tags of its own, which is the normal case. Moving the merge into `Read` only
converts the invalid plan into an inconsistent-result-after-apply, the same rule one step
later. The `tags` / `tags_all` split is what the AWS provider does for this exact problem,
and it is a user-visible schema change rather than a fix behind the scenes.

**Consequence for reads:** NodePing only stores the merged set, so `Read` takes `tags_all`
from the API and derives `tags` by removing the configured `default_tags`. A tag the
practitioner declared that is also a default is kept in `tags`, decided by consulting prior
state — subtracting the defaults blindly would drop it and plan its removal on every run.
Import has no prior state and removes the defaults unconditionally.

**Accepted limitation:** because the derivation depends on the *current* `default_tags`, a
tag that was a default when the check was created and is later removed from the provider
configuration becomes part of `tags` on the next refresh. That is the only reading available
from the API response, and it is the conservative one: it keeps the tag on the check rather
than silently deleting it.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-09-22

---

## A-021 — Target equality follows the normalisation NodePing was observed to perform

**Assumption:** `normalizeTarget` reproduces only the rewriting confirmed against the live
API, per check type. Every type has surrounding whitespace trimmed. For `HTTP`, `HTTPADV`,
`HTTPCONTENT`, `HTTPPARSE`, `SSL`, `AUDIO`, `MONGODB`, `PGSQL` and `REDIS` the scheme and
host are folded to lower case, an empty path becomes `/`, and characters illegal in a path
are percent-encoded. Everything else is left alone, including the port (a default port is
kept), duplicate slashes, dot segments, userinfo, query and fragment.

**Why:** The first live acceptance run failed an import round trip because NodePing stores
`https://example.com` as `https://example.com/`. The previous `normalizeURL` trimmed a
trailing slash from any target, which was both too loose and too broad: it treated
`https://example.com/path/` and `https://example.com/path` as the same target when NodePing
keeps them distinct, and it applied to check types the API does not normalise at all. Both
directions are bugs — one hides a real change, the other reports one that does not exist.

**Evidence:** every type in `ValidCheckTypes` was submitted to the live API in the test
subaccount and the stored target read back. `WEBSOCKET`, `FTP`, `RDAP`, `SIP` and `WHOIS`
accept a URL and store it verbatim, so scheme-shape alone cannot decide membership; the type
list is explicit for that reason. `PING`, `PORT`, `DNS`, `RBL`, `SMTP` and `IMAP4` keep host
case. Adding a check type to the list without re-running that probe would be a guess.

**Not reproduced:** NodePing punycodes a non-ASCII host (`https://例え.com` is stored as
`https://xn--r8jz45g.com/`), and its percent-encoding of non-ASCII path bytes was not
characterised. Implementing the first would mean taking an IDNA dependency for a case that
has not come up. A target using either is compared only as far as the rules above reach, so
the provider reports a difference it could have collapsed. That direction is the safe one:
it shows a diff that settles after one apply instead of hiding a real change.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-09-22

---

## A-022 — An empty `customer_id` is rejected rather than treated as "no subaccount"

**Assumption:** `customer_id = ""` is a configuration error. The attribute carries
`stringvalidator.LengthAtLeast(1)`; a practitioner who wants the token's own account omits
the attribute instead.

**Why:** `clientFor` reads an empty string as "use the provider level client", so the check
is created in whichever account the token belongs to. `mapForApply` preserves every value
the plan already knew, so state keeps `""` while the API owns the check under a real account
ID. The first refresh reads that ID back, and because `customer_id` carries
`RequiresReplace` the disagreement is a destroy and recreate on every subsequent plan. Under
resource identity the framework does not even get that far: it aborts the refresh with
"Unexpected Identity Change", reported as a provider bug.

**Evidence:** reproduced against the test subaccount with the validator removed. The
post-apply refresh plan failed with the identity going from `customer_id: ""` to
`customer_id: "20260922144954LK9ZD3"` on the same check ID.

**Rejected alternative:** coercing an empty `customer_id` to the account the API reports.
That silently rewrites a value the practitioner wrote, on an attribute that forces
replacement, and it would make `customer_id = ""` mean something different depending on
which token the provider was configured with.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-09-22

---

## A-023 — A Create that fails after the API call records the identifier before returning the error

**Assumption:** when NodePing has already created an object and the provider is about to
return an error from `Create`, the provider writes the object's identifier into state and
still returns the error. It does not write the planned values, and it does not write
identity data.

**Why:** every `Create` has the same shape — call the API, do some more work, then
`resp.State.Set`. Anything that returns in between leaves an object that exists in NodePing
and in no state file, so OpenTofu cannot refresh it, cannot destroy it, and will create a
second one on the next apply. Three `nodeping_schedule` objects and five
`nodeping_maintenance` objects were deleted by hand during acceptance testing for exactly
this reason. `nodeping_subaccount` is the sharpest case: the POST creates a billable
account and two more API calls follow before state is written.

**Evidence that partial state survives:** terraform-plugin-framework v1.19.0
(`internal/fwserver/server_createresource.go`) only rejects a null create state when there
are no error diagnostics, and it skips its identity checks on the error path entirely.
OpenTofu's apply (`internal/tofu/node_resource_abstract_instance.go`) saves whatever
non-null object the provider returned when apply reported errors — the
`diags.HasErrors() && !newVal.IsNull()` branch — and skips `AssertObjectCompatible` in the
same situation, so required attributes left null are accepted. The unit tests in
`check`, `schedule` and `subaccount` assert the identifier is in `resp.State` after a
forced post-create failure.

**Why only the identifier:** OpenTofu converts any unknown left in an applied object to
null and reports each one as a provider bug. The planned model routinely carries unknowns
for computed attributes, so writing it would produce a page of spurious errors on top of
the real one. `customer_id` is written alongside the ID where the practitioner configured
it, because `Read` needs it to scope the lookup to the right subaccount; where it was not
configured, the object lives in the token's own account and null says the same thing.

**Why identity is left null:** `customer_id` is often still unknown on this path. An
identity that disagrees with the one the next `Read` derives is rejected by the framework
as "Unexpected Identity Change" (`server_readresource.go`), which would block the recovery
the partial state exists to enable. That check is skipped when the stored identity is fully
null, and `Read` populates it on the next refresh.

**Where it is not applied, and why:** `contact`, `contactgroup`, `maintenance` and
`notificationprofile` have no reachable window. `notificationprofile.Create` has no early
return at all between the API call and `State.Set`. In the other three the only step in
between converts plain Go slices and maps into framework collections
(`types.SetValueFrom` / `types.MapValueFrom` over `[]string` / `map[string]string`), which
cannot produce a diagnostic for any response the client will return. Adding the guard there
would be code that can never run. If a step that can fail is ever added to one of those
`Create` methods, it needs the same treatment.

**Limitation — the identifier is not always knowable.** When the create call itself returns
an error, the provider has no ID: for `check`, `contact`, `contactgroup`, `maintenance`,
`notificationprofile` and `subaccount` the ID exists only in the response body, so a request
that reached NodePing and was applied but whose response was lost or could not be decoded
still orphans the object. Nothing in the API offers an idempotency key or a
create-with-caller-supplied-ID form for these types, and looking the object up by name after
a failure would be a guess at which of several similarly named objects was the new one.
`nodeping_schedule` is the exception: its ID is chosen by the practitioner and the endpoint
is a PUT, so the ID is known whether or not the call succeeded, and it is recorded on any
failure once the request has been handed to the client.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-09-22

---

## A-024 — `suppress_diag` accepts only `false`, because NodePing discards it

**Assumption:** `suppress_diag = true` on a `nodeping_contact` address is a configuration
error. The attribute remains in the schema and `false` is accepted, so existing
configurations and state are unaffected.

**Why:** NodePing accepts `suppressdiag` in a contact address payload, stores nothing, and
never returns it. Every sibling flag — `suppressup`, `suppressdown`, `suppressfirst`,
`suppressall` — round trips. The provider therefore planned `true` and read back `false`,
and OpenTofu aborted the apply with "Provider produced inconsistent result after apply …
This is a bug in the provider", which points the practitioner at the provider for a
limitation of the upstream API.

**Evidence:** `POST /contacts` with `suppressdiag: true` on email, SMS and webhook
addresses, against the test subaccount. The created document omits the field in all three
cases while a sibling flag sent in the same request comes back set. `PUT /contacts/:id`
behaves the same. Reproduced through the provider before the validator existed:
`TestAccContact_suppressDiagRejected`'s first step failed with `.address[0].suppress_diag:
was cty.True, but now cty.False`.

**Rejected alternative:** keeping the planned `true` in state. State would then claim a
suppression that is not in effect, which is worse than refusing the value.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-09-22

---

## A-025 — The contact `addresses` map is sent on every update, and a contact may not lose its last address

**Assumption:** `ContactUpdateRequest.Addresses` carries no `omitempty`. An update that
keeps no existing address sends `"addresses": {}`; an update to a contact that has no
addresses and gains none sends `"addresses": null`. Reducing a contact to zero addresses is
refused by the provider before the request is made.

**Why:** NodePing treats `addresses` as the complete set and deletes anything missing from
it. With `omitempty`, an update whose planned addresses were all new — replacing a contact's
only email with an SMS, where nothing matches by content, ID or position — dropped the map
from the request, so NodePing added the new address and kept the old one. The apply failed
with "block count changed from 1 to 2" and the contact was left with an address the
configuration had removed.

The two empty forms are not interchangeable. `{}` is refused with "Account must have at
least one 'owner' contact" whenever it would leave the contact empty, including on a
contact that is already empty; `null` is read as "not specified" and leaves the addresses
alone. So `{}` is sent only when `newaddresses` will repopulate the set.

**Evidence:** probed against the test subaccount. `PUT` with `addresses: {}` and a
`newaddresses` entry replaces the set; `PUT` with `addresses: {}` alone returns
`{"error": "Account must have at least one 'owner' contact."}` whether or not the contact
currently has addresses; `PUT` with `addresses: null` succeeds and preserves them.

**Rejected alternative:** letting the API refuse the zero-address update. The practitioner
would read a message about account owners, followed by an inconsistent-result error naming
the provider, for what is simply an operation NodePing does not support.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-09-22

---

## A-026 — A contact with no name is null in state, not an empty string

**Assumption:** `Read` and `ImportState` map an absent or empty NodePing contact name to
`types.StringNull()`.

**Why:** `name` is Optional and not Computed, so a contact created without one holds null
in state. NodePing omits the `name` key entirely for such a contact, and writing the
resulting `""` back made every refresh plan an update from null to `""`. No apply could
settle it: the create request drops an empty name (`omitempty`), so the API never gains the
value the plan is asking for.

**Evidence:** `POST /contacts` without `name` returns a document with no `name` key.
Through the provider, the post-apply refresh plan was non-empty on every run;
`TestAccContact_withoutName` is the regression test and its `PlanOnly` step is the
assertion.

**Limitation:** `name = ""` written explicitly is now indistinguishable from an absent name
and will still diff. NodePing has no way to store an empty name, so there is nothing for the
provider to make that configuration mean.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-09-22

---

## A-027 — An imported contact's address order is not the configuration's

**Assumption:** the acceptance tests verify an imported contact's `address` blocks with
`ImportStateCheck` against the set of blocks apply wrote, and exclude `address` from
`ImportStateVerify`.

**Why:** NodePing returns addresses as a map keyed by a server-assigned ID. With no plan to
follow, `mapAddressesToModel` orders them by that ID, which is random per address, while an
apply orders them as the configuration does. `ImportStateVerify` compares by list position,
so it would pass or fail on which IDs NodePing happened to issue — a flaky test rather than
a real signal. The set comparison asserts the stronger property anyway: every block,
including its server-assigned ID, comes back from the API unchanged.

**Consequence for practitioners:** the first plan after importing a contact with more than
one address may show the blocks being reordered. The apply is a no-op at the API — the
provider matches addresses by content, so no ID moves — and the next plan is empty. Sorting
cannot fix this; no ordering derivable from the API response can match an arbitrary
configuration order.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-09-22

## A-028 — Every shape found by importing the whole estate is accepted

**Assumption:** the provider accepts each value NodePing returned across all 42 accounts on
2026-09-30, rather than rejecting data it did not expect.

**Why:** importing the whole estate (543 objects) failed on 114 of them, across eight causes,
all in the provider:

- `queue` is `false` on every disabled check, not a queue name. The field was never used, so
  it is no longer decoded. Disabling a check had made it unreadable.
- A schedule day with no hours has `time1`/`time2` of `false`. `ScheduleTime` reads `false`
  and `null` as empty.
- `sens = 0` (one live PUSH check) and `statuscode = 0` (two HTTPADV checks) were refused by
  validators stricter than NodePing.
- A type-specific value left on a check after its type changed (`contentstring` on three SSL
  checks) was an error. It is now a warning. A boolean flag turned on for the wrong type is
  still an error, since no live check has one.
- `custrole = "owner"` was refused.
- Owner and login contact addresses have no `type`, and `GET /contacts/:id` returns a login
  contact's addresses as an array keyed by nothing. `type` is Optional, read as null when
  absent, and still required on a new address; the array form is keyed by the address
  itself, as `GET /contacts` keys it.
- A webhook query-string value can be a JSON object. `FlexStringMap` keeps it as JSON text
  and sends text that parses as an object or array back as JSON, so an update does not turn
  the object into a string. A literal string that happens to be a JSON object would be sent
  as an object; no live value is like that.
- `nodeping_subaccount` required `contactname`, `email` and `location`, which NodePing never
  returns, so no subaccount could be imported. They are Optional, `ModifyPlan` requires them
  when a subaccount is created, `location` is taken from `default_locations` on import, and
  `emailme` is read from the API, which returns it as a boolean.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-09-30

## A-029 — An untagged check plans null tags_all, not an empty set

**Assumption:** when the configuration and provider defaults give a check no tags and state
already holds none (null), `tags_all` is planned null.

**Why:** NodePing omits the `tags` key on a check that never had one, which Read stores as
null, and planning `[]` against that was an in-place update on 31 of 35 checks in the first
estate import. `[]` is still planned when state holds tags, because that is what tells
NodePing to clear them.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-09-30

## A-030 — Integer zeros are sent to NodePing as strings

**Assumption:** `sens`, `statuscode`, `warningdays`, `volumemin` and `port` are sent as the
string `"0"` when their value is zero, and as JSON numbers otherwise.

**Why:** NodePing treats a JSON number `0` as empty. Creating a check with `sens: 0` fails with
"Unable to create check", and `statuscode: 0` or `warningdays: 0` is stored as `""`, so the
value never settles. The string `"0"` is stored as zero. Found by the 2026-09-30 acceptance
tests and confirmed with direct API calls against the test subaccount.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-09-30

## A-031 — WHOIS and RDAP intervals follow NodePing's minimums

**Assumption:** an unset `interval` plans `1440` for `WHOIS` and `RDAP` checks, and an explicit
interval below `240` (WHOIS) or `60` (RDAP) is rejected at plan time.

**Why:** NodePing replaces a shorter interval on these types with `1440` on both create and
update, so the provider's default of `15` drifted on every plan.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-09-30

## A-032 — SPEC10DNS is not a DNS type for attribute rules

**Assumption:** `dnstype`, `dnstoresolve`, `dnssection` and `dnsrd` are treated as not
applicable to `SPEC10DNS` checks.

**Why:** NodePing keeps none of them on a SPEC10DNS check, so a configured value never came
back and always planned a change.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-09-30

## A-033 — contactname and email cannot be changed on an existing subaccount

**Assumption:** changing `contactname` or `email` on a subaccount that already has a value is
a plan-time error; on an imported subaccount with no value, the configured value is recorded
without an API call.

**Why:** NodePing neither returns nor updates these fields. The earlier behaviour forced a
replacement, and replacing a subaccount deletes every check and contact in it, which is never
an acceptable way to rename a contact. Supersedes the `requiresReplaceIfWasSet` design.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-09-30
