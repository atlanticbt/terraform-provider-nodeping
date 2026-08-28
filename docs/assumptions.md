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

## A-007 — Optional + Computed suppresses attribute-removal diffs

**Assumption:** Deleting an attribute from configuration produces no diff for attributes
marked Optional + Computed without a schema default.

**Why:** This is the accepted cost of eliminating "Provider produced inconsistent result
after apply" errors. Any attribute the API normalises or echoes back must be Optional +
Computed, and the framework then substitutes prior state when config goes null. The
alternative — Optional-only — makes every server-side normalisation a hard apply failure,
which is strictly worse.

**Mitigation:** Attributes with schema defaults are unaffected. To clear such an attribute,
set it explicitly to its zero value rather than removing the line.

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

## A-013 — REDIS host passwords are never read back from the API

**Assumption:** `hosts[*].password` is written but never mapped from API responses, and is
restored from prior state on refresh.

**Why:** Applying the "Read must never copy a credential into a regular attribute" rule
consistently. NodePing returns these in plaintext like check passwords.

**Trade-off, accepted:** a REDIS host password changed outside OpenTofu is not detected as
drift. Host and port on the same entries *are* still refreshed, so structural drift
surfaces normally.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-08-27

---

## A-014 — `fields.match` is exposed on all field entries

**Assumption:** The `match` sub-attribute appears on every `fields` entry, though NodePing
honours it only for PGSQL, MYSQL, and MONGODB.

**Why:** A per-check-type nested attribute shape is not expressible in a single schema. The
attribute description and resource documentation state where it takes effect.

**Recorded by:** Claude (Atlantic BT) · **Date:** 2026-08-27
