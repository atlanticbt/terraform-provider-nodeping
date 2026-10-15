<!--
doc: assumptions
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# Assumptions and Accepted Risks

This document records the design decisions of this provider that are not obvious. Each entry
gives the decision and the reason for it. Record here any decision that can surprise a
reviewer.

When an entry says "observed against the live API", we sent direct calls to NodePing and saw
the result. We did not take it from the documentation.

References to files in other projects show the project name first. For example,
"terraform-plugin-framework `internal/fwserver/server_createresource.go`" is a file in the
upstream framework, not in this repository.

## Index

| Entry | Subject | Status |
|---|---|---|
| [A-001](#a-001-nodeping-stores-check-credentials-in-plaintext) | NodePing stores check credentials in plaintext | Accepted risk |
| [A-002](#a-002-the-client-corrects-responses-that-do-not-follow-http-conventions) | The client corrects non-standard responses | Active |
| [A-003](#a-003-ad-hoc-maintenance-is-not-manageable-as-a-resource) | Ad-hoc maintenance is not a resource | Active |
| [A-004](#a-004-accountsuppressall-is-not-exposed) | `accountsuppressall` is not exposed | Active |
| [A-005](#a-005-subaccount-fields-that-only-post-accepts-come-from-configuration) | Subaccount POST-only fields | Partly superseded by A-033 |
| [A-006](#a-006-lam-is-accepted-for-subaccount-location) | `lam` for subaccount `location` | Active |
| [A-007](#a-007-optional--computed-is-used-only-where-necessary) | Optional + Computed only where necessary | Active |
| [A-008](#a-008-cross-field-validation-is-lenient-where-the-documentation-contradicts-itself) | Lenient cross-field validation | Active |
| [A-009](#a-009-protocol-6-support-in-opentofu-is-observed-not-promised) | Protocol 6 in OpenTofu | Accepted risk |
| [A-010](#a-010-acceptance-tests-run-against-the-live-api) | Acceptance tests run against the live API | Resolved |
| [A-011](#a-011-checktoken-is-read-only) | `checktoken` is read-only | Active |
| [A-012](#a-012-type-specific-defaults-are-sent-only-for-the-correct-type) | Type-specific defaults | Active |
| [A-013](#a-013-redis-host-credentials-are-write-only) | REDIS host credentials are write-only | Resolved |
| [A-014](#a-014-fieldsmatch-is-on-all-field-entries) | `fields.match` on all entries | Active |
| [A-015](#a-015-header-query-string-and-address-values-are-not-sensitive) | Headers and addresses are not sensitive | Accepted risk |
| [A-016](#a-016-warningdays-has-no-lower-bound) | `warningdays` has no lower bound | Active |
| [A-017](#a-017-applicability-validation-ignores-boolean-flags-set-to-false) | Flags set to `false` are ignored | Active |
| [A-018](#a-018-get-accounts-returns-one-document-when-scoped-to-a-customer) | `GET /accounts` with `customerid` | Active |
| [A-019](#a-019-maintenance-responses-key-the-windows-by-id) | Maintenance responses keyed by ID | Active |
| [A-020](#a-020-default_tags-is-merged-into-tags_all) | `default_tags` goes into `tags_all` | Active |
| [A-021](#a-021-target-comparison-follows-the-normalization-of-nodeping) | Target normalization | Active |
| [A-022](#a-022-an-empty-customer_id-is-an-error) | Empty `customer_id` is an error | Active |
| [A-023](#a-023-a-failed-create-records-the-identifier) | A failed create records the identifier | Active |
| [A-024](#a-024-suppress_diag-accepts-only-false) | `suppress_diag` accepts only `false` | Active |
| [A-025](#a-025-contact-updates-always-send-addresses) | Contact updates always send `addresses` | Active |
| [A-026](#a-026-a-contact-without-a-name-is-null-in-state) | A contact without a name is null | Active |
| [A-027](#a-027-the-address-order-after-import-is-not-the-configuration-order) | Address order after import | Active |
| [A-028](#a-028-the-provider-accepts-all-shapes-found-in-a-large-import) | Shapes found in a large import | Active |
| [A-029](#a-029-a-check-without-tags-plans-a-null-tags_all) | A check without tags plans null `tags_all` | Active |
| [A-030](#a-030-integer-zeros-are-sent-as-strings) | Integer zeros are sent as strings | Active |
| [A-031](#a-031-whois-and-rdap-intervals-follow-the-nodeping-minimums) | WHOIS and RDAP minimum intervals | Active |
| [A-032](#a-032-spec10dns-is-not-a-dns-type-for-attribute-rules) | SPEC10DNS attribute rules | Active |
| [A-033](#a-033-contactname-and-email-cannot-change-on-an-existing-subaccount) | Subaccount `contactname` and `email` are fixed | Active |

---

## A-001: NodePing stores check credentials in plaintext

**Assumption:** NodePing stores credentials of authenticated checks in cleartext. It also
returns them in cleartext. This applies to `FTP`, `IMAP4`, `MYSQL`, `POP3`, `SMTP` and `SSH`
checks. This provider cannot change that.

**Why:** The NodePing API needs the plaintext to authenticate to the monitored target. The
NodePing documentation tells users to use credentials with the least privilege for this
reason.

**Mitigation:** `password_wo` is a write-only attribute. We verified on OpenTofu 1.12.5 that
the value is not in state and not in saved plan files. When the value comes from a secret
store, use an `ephemeral` block. A regular `data` source keeps the secret in state, although
the check attribute does not. REDIS `hosts[*].password` uses the same method (see A-013).

**Residual risk:**

- The deprecated `password` attribute writes the plaintext to state. It stays only for
  migration.
- `Sensitive: true` hides the value in CLI output only. It does **not** protect state.
- Enable OpenTofu state encryption in all cases.
- Use monitoring-only credentials with minimum privilege. NodePing stores them in
  cleartext.

**Recorded by:** Atlantic BT · **Date:** 2026-08-27

---

## A-002: The client corrects responses that do not follow HTTP conventions

**Assumption:** NodePing responses do not always follow REST conventions. The client layer
makes them consistent.

**Why:** We saw these behaviors on many endpoints:

| Behavior | Example |
|---|---|
| An absent resource can return HTTP 200 | `{}` with status 200, not 404 |
| An error can return HTTP 200 | `{"error": "..."}` with status 200, not 4xx |
| Numbers can be strings | `interval`, `sens`, `threshold`, `port`, `warningdays` |
| Booleans have many forms | `true`, `1` or `"1"` |
| Identifier keys change by endpoint | `id` or `_id`, `customer_id` or `customerid` |

At first, a string value of `interval` stopped the decode of the full check struct. This
broke `ListChecks` completely.

**Mitigation:** The client layer does the decoding and the "not found" classification. Thus
all resources get the same behavior, and there is one place to fix a problem. In
`internal/client`:

| Function | Purpose |
|---|---|
| `errorMessageFromBody` | The only detector for an error in a 2xx response. `executeRequest` runs it on each successful response. A match becomes an `*APIError`. Thus no endpoint decodes an error document into a resource struct. |
| `isNotFoundMessage` | The only classifier for the "not found" text of NodePing. Only `APIError.IsNotFound` calls it. |
| `notFoundOrWrap` | Changes a "not found" signal from the API into `*NotFoundError`. It wraps all other errors. |
| `absentAsNotFound` | Handles a document that decodes correctly but is empty. Each resource supplies its own test for "empty". |

**Accepted risk:** The text match is conservative on purpose:

- It uses a short list of message fragments.
- It does not apply to statuses that the client can retry (429 and 5xx).

A temporary failure can contain the words "not found". It must stay a real error. If the
provider reads it as "absent", OpenTofu removes the resource from state. The next apply then
creates the live object again. The short list has a cost: NodePing can use a different
phrase for an absent object. That phrase then shows as an ordinary error. This is the safe
result.

**Recorded by:** Atlantic BT · **Date:** 2026-08-27

---

## A-003: Ad-hoc maintenance is not manageable as a resource

**Assumption:** `nodeping_maintenance` supports scheduled (cron) windows only. You cannot
create or import ad-hoc windows.

**Why:** NodePing deletes an ad-hoc maintenance record when the run ends. A declarative
resource then sees the record go. It removes the record from state, and the next plan
creates it again. This loop disables the monitoring of the customer again and again. The
plugin framework cannot show "deleted is expected and final". The resource would stay in
state for an object that does not exist. Later edits to `duration` or `checklist` would do
nothing, and no message would tell you.

**Mitigation:** The provider enforces this. It is not only documented:

- `cron` is Required.
- The create request cannot contain `id: "ad-hoc"`.
- Import rejects the ad-hoc ID before any API call.
- Update refuses an ad-hoc ID.
- Both data sources show ad-hoc windows. The plural data source has an `ad_hoc` boolean.

**Alternative:** `scripts/nodeping-adhoc-maintenance.sh` sends the API call directly. Run it
from a `terraform_data` resource with a `local-exec` provisioner. The recipe is in
`docs/resources/nodeping_maintenance.md`. The script is outside provider state on purpose,
and this makes it safe. OpenTofu never tries to reconcile the window, so the loop cannot
occur. The script:

- validates and limits each input
- rejects check IDs that contain characters other than letters, digits and hyphens
- requires an `https` endpoint outside loopback
- reads the token from the environment, so that the token is not in `ps` output

**Why this is not clearly temporary:** The declarative solution is Terraform `action`
blocks (Terraform 1.14). OpenTofu has not added them. It has said that it may use a
different design (opentofu#3309). Thus we have no date to replace this alternative with a
resource.

**Recorded by:** Atlantic BT · **Date:** 2026-08-27

---

## A-004: `accountsuppressall` is not exposed

**Assumption:** `nodeping_subaccount` does not have the account-wide notification
suppression flag.

**Why:**

- No read operation returns the flag. The provider could never refresh it, and it would
  always drift.
- When you enable notifications again, NodePing does not restore contacts that were
  suppressed one by one. Thus `false` does not describe a state that the provider can
  guarantee.
- The flag is a switch for incident response. It is not configuration.

**Recorded by:** Atlantic BT · **Date:** 2026-08-27

---

## A-005: Subaccount fields that only POST accepts come from configuration

**Status:** A-033 replaces the replacement behavior in this entry. The rest stays correct.

**Assumption:** The provider never maps `contactname`, `email` and
`autodiagnotifications` from API responses.

**Why:** NodePing accepts these fields on create. It does not return them on read. A map
from the response would cause permanent drift.

**Original mitigation:** The fields used `RequiresReplaceIf`, which read a null prior state
as "no change". A plain `RequiresReplace` was not safe. Import cannot recover these values.
A plain modifier would read `null -> "value"` as a change, and it would **destroy a
subaccount that you just imported**.

**Now:** A change of `contactname` or `email` on an existing subaccount is a plan-time
error. On an imported subaccount, the provider records the configured value. See A-033.
Since A-028, the provider reads `emailme` from the API and takes `location` from
`default_locations` on import.

**Recorded by:** Atlantic BT · **Date:** 2026-08-27

---

## A-006: `lam` is accepted for subaccount `location`

**Assumption:** The Latin America region code `lam` is valid when you create a subaccount.

**Why:** The NodePing documentation for account `POST` lists `nam`, `eur`, `eao` and `wlw`.
It does not list `lam`. The documentation for account `PUT` and for check `runlocations`
lists `lam`. We think that the omission is a documentation error. A rejection would block
valid configurations.

**Recorded by:** Atlantic BT · **Date:** 2026-08-27

---

## A-007: Optional + Computed is used only where necessary

**Assumption:** An attribute is `Optional + Computed` only when NodePing supplies a value
that the practitioner did not set. This occurs in three cases:

- a documented default on the server
- a schema `Default`
- a field that the API always returns

These attributes do not show a diff when you remove them from the configuration.

**Why:** When the configuration value becomes null on an Optional + Computed attribute, the
framework uses the prior state. The alternative for a field with a server default is
Optional only. Then each normalization on the server causes the error "inconsistent result
after apply" on *each* apply. That is worse than a removal without a diff.

**Scope decreased:** At first, too many attributes were Optional + Computed. These 24
`nodeping_check` attributes are now plain `Optional`, and a removal gives a correct diff:
`description`, `contentstring`, `sendheaders`, `receiveheaders`, `postdata`, `data`,
`fields`, `hosts`, `edns`, `username`, `email`, `query`, `database`, `namespace`,
`whoisserver`, `rdapurl`, `ignore`, `servername`, `sshkey`, `clientcert`, `sentinelname`,
`dnstoresolve`, `dep` and the deprecated `password`. NodePing returns each of them only when
it is set. For each one, we examined the map functions. The model becomes null when the API
does not return the field.

**Kept as Computed, on purpose:**

| Attribute | Reason |
|---|---|
| `regex`, `invert`, `follow`, `ipv6`, `verify`, `dnsrd`, `oldresultfail`, `verifyvolume` | Booleans without a schema default. We do not know how NodePing handles null and `false`. |
| `secure` | NodePing returns it as a boolean and as a string. |
| `homeloc`, `runlocations` | NodePing sets them when they are not set. |
| `target`, `label`, `customer_id` | The API always returns them. |
| `schedule.time1`, `schedule.time2`, `contactgroup.name`, `contactgroup.members` | The map function writes the API value over the plan value. |

A wrong change to Optional causes a hard failure on each apply. Thus we keep Computed when we
are not sure.

**Mitigation:** To clear one of the remaining Computed attributes, set it to its zero value.
Do not delete the line.

**Change on 2026-09-22:** `tags` was on the list of Computed attributes, because `ModifyPlan`
wrote a merged value into the plan. That was the defect in A-020: Terraform rejects a planned
value that is different from a known configuration value. The merge now goes into the new
computed attribute `tags_all`. Thus `tags` contains nothing that the API supplies, and it is
now plain `Optional`. The removal test table covers it, as it covers the other 24.

**Recorded by:** Atlantic BT · **Date:** 2026-08-27

---

## A-008: Cross-field validation is lenient where the documentation contradicts itself

**Assumption:** Some type-specific fields are permitted on more check types than the field
table of NodePing shows.

**Why:** The NodePing table "fields by check type" contradicts the text and the examples of
the same documentation.

- `contentstring` is permitted on DNS and DOHDOT, because the documented DNS example uses it.
- `CLUSTER` is in the set of types that do not need a target.

A validator that is too strict blocks valid configurations. That is a worse failure than a
lenient validator that fails at apply.

**Recorded by:** Atlantic BT · **Date:** 2026-08-27

---

## A-009: Protocol 6 support in OpenTofu is observed, not promised

**Assumption:** It is safe to ship a protocol 6 provider for OpenTofu.

**Why:**

- The published compatibility promise of OpenTofu v1.x names **protocol 5 only**.
- The OpenTofu source supports protocol 6. OpenTofu `internal/getproviders/registry_client.go`
  accepts versions `>= 5, < 7`.
- We verified that protocol 6 operates on OpenTofu 1.12.5.
- terraform-plugin-framework uses protocol 6. Thus every modern framework provider depends
  on this support.

**Mitigation:** If a contractual guarantee becomes necessary, `terraform-plugin-mux` can
serve both protocol versions.

**Recorded by:** Atlantic BT · **Date:** 2026-08-27

---

## A-010: Acceptance tests run against the live API

**Status:** Resolved. At first, this entry recorded that we wrote the acceptance tests but
did not run them. No test account was available.

**Now:** Each resource has acceptance tests. They create, read, import and destroy real
objects in a dedicated test account. Each change to a resource must pass them. The tests
confirmed or corrected the behaviors that this entry listed as unverified:

- A root-account token accepts `customerid=<own account id>`. NodePing ignores the selector.
- To clear all tags, the provider must send an empty array (A-029).
- Where NodePing is different from its documentation, we corrected the response shapes in
  A-002 (A-016 and later).

**Recorded by:** Atlantic BT · **Date:** 2026-08-27

---

## A-011: `checktoken` is read-only

**Assumption:** The token of an AGENT or PUSH check is a Computed, Sensitive attribute. You
cannot regenerate it through the provider.

**Why:** The NodePing documentation contradicts itself. It says that `checktoken` is
read-only. It also says that you set it to the text `"reset"` to request a new token. As an
Optional + Computed attribute, this gives one of two failures:

- a permanent diff (the configuration says `"reset"`, but state has the real token)
- the error "inconsistent result after apply"

The API never returns the value that the user wrote.

**Mitigation:** Regenerate tokens in the NodePing user interface. If this becomes necessary,
use a separate resource for one purpose, or an ephemeral action. Do not use an attribute on
the check.

**Recorded by:** Atlantic BT · **Date:** 2026-08-27

---

## A-012: Type-specific defaults are sent only for the correct type

**Assumption:** `redistype` and `dohdot` have schema defaults. The framework applies these
defaults to all check types. Thus the outbound request removes them, unless the check type
uses them.

**Why:** Framework defaults always apply. Without this filter, an HTTP check plans
`redistype = "standalone"` and sends it to the API. The map function applies the same default
when the API does not return the field. Without this, a refresh maps null, and each plan
after it shows a permanent diff.

**Recorded by:** Atlantic BT · **Date:** 2026-08-27

---

## A-013: REDIS host credentials are write-only

**Status:** Resolved. `hosts[*].password_wo` replaces the accepted risk.

**Original problem:** The provider wrote `hosts[*].password` but never mapped it from API
responses. Thus it did not detect a change to a host password outside OpenTofu.

**Resolution:**

- `hosts[*].password_wo` and `hosts[*].password_wo_version` keep the credential completely
  out of state. Thus there is nothing that can drift.
- terraform-plugin-framework v1.19.0 supports `WriteOnly` in a `MapNestedAttribute`. We
  verified this in the framework source (terraform-plugin-framework
  `map_nested_attribute.go:317-323`). We did not assume it.
- The containing map attribute **must not be `Computed`**. The change of `hosts` to plain
  `Optional` (A-007) made this possible. The two changes depend on each other.
- A custom validator enforces the conflict rules for each host.
  `resourcevalidator.Conflicting` cannot express a path through a map.

**Residual risk:** `hosts[*].password` stays for migration and is deprecated. It still
writes the plaintext to state. The provider still never reads it back. The original risk
applies to users who did not migrate.

**Recorded by:** Atlantic BT · **Date:** 2026-08-27

---

## A-014: `fields.match` is on all field entries

**Assumption:** Each `fields` entry has the `match` sub-attribute. NodePing uses it only for
PGSQL, MYSQL and MONGODB.

**Why:** One schema cannot have a nested attribute shape for each check type. The attribute
description and the resource documentation tell where `match` has an effect.

**Recorded by:** Atlantic BT · **Date:** 2026-08-27

---

## A-015: Header, query-string and address values are not sensitive

**Assumption:** These attributes show in plan output and in generated configuration:

- `nodeping_check.sendheaders`
- `nodeping_contact.address.address`
- `address.headers`
- `address.querystrings`

In return, you can import the objects.

**Why:** `tofu plan -generate-config-out` writes `null # sensitive` for each attribute that
has `Sensitive`. Against a live account, this caused two failures:

1. You could not import any contact. `address` is Required, and a required attribute set to
   `null` is a configuration error.
2. Each check with custom headers planned an update in place. The update would delete its
   live `sendheaders`. You could not see this, because the diff showed `(sensitive value)`
   on both sides.

This is a real trade-off. Headers often contain credentials, for example an access-proxy
service token. Without `Sensitive`, the first plan shows them in cleartext, and generated
configuration contains them. We made the decision with this knowledge, and it stays.

`Sensitive` only hides CLI output. It never kept the header out of state. Anyone who can read
the check in the NodePing user interface can see the value. What `Sensitive` gave was an
import path that deletes production configuration on apply. A contact address is PII
(personally identifiable information), not a credential. The same reasoning applies, with
less risk.

`address.headers` and `address.querystrings` are Optional. Thus `Sensitive` did not stop the
import of a contact. It made the import lose data: each webhook contact that used them
planned `(sensitive value) -> null`. This is the same failure, but harder to see. The
provider uses one rule for all attributes that config generation must reproduce.

These credential attributes stay sensitive: `password`, `password_wo`, `snmpcom`,
`clientcert`, `sshkey`. They are write-only or Computed. Thus config generation never must
reproduce them.

> **SECURITY:** A file that `tofu plan -generate-config-out` writes for an account with
> header authentication contains live credentials. Remove the credentials before the file
> goes into version control. Use a variable reference, not the literal value, in your root
> module.

**Recorded by:** Doug Eubanks · **Date:** 2026-09-21

---

## A-016: `warningdays` has no lower bound

**Assumption:** The provider has no minimum for `warningdays`.

**Why:** The schema had `AtLeast(1)`. A live SSL check stores `warningdays = 0`. Thus its
import failed on a rule that NodePing does not have. The real minimum of NodePing is not
documented. The same reasoning applies to the upper bounds of `threshold` and `sens`: a bound
that the API does not enforce makes existing resources unmanageable. A different invented
minimum would cause the same problem.

**Recorded by:** Doug Eubanks · **Date:** 2026-09-21

---

## A-017: Applicability validation ignores boolean flags set to false

**Assumption:** The provider rejects `regex`, `invert` and `dnsrd` on a check type that does
not support them only when the value is `true`.

**Why:** NodePing returns these flags as `false` on check types that do not use them. A
survey of live checks found:

- `invert` was present and `false` on PUSH, HTTPADV and HTTPCONTENT checks.
- `ipv6` was present and `false` on PING and PUSH checks.
- The presence is not consistent. Most PUSH checks did not have `invert`. One PUSH check had
  `invert = false`, and that check failed.

The read path maps what the API sends. Thus the refreshed PUSH check had `invert = false`,
and the validator of the provider then rejected the output of the provider. The same rule
applies to `dnsrd` by analogy. No live check in the survey had it.

The rule keeps the useful part of the guardrail. A flag that someone set to `true` for a
check type that ignores it is still a mistake, and the provider reports it.

We did not change the read path. To set inapplicable attributes to null would change the
value of an Optional + Computed attribute. That can change a hidden diff into the error
"inconsistent result after apply" on each apply (see A-007 and A-012).

**Recorded by:** Doug Eubanks · **Date:** 2026-09-21

---

## A-018: `GET /accounts` returns one document when scoped to a customer

**Assumption:** When the request has `customerid`, NodePing returns the one account
document for `GET /accounts`. `ListAccounts` changes it into a listing with one entry, keyed
by its `_id`. The provider does not map the `parent` field of the document.

**Why:** We confirmed this with curl against the live API.

- The shared client adds `customerid` to each request when the provider has one. This is the
  usual method to manage a subaccount. Thus `nodeping_accounts` failed to decode on each read
  in that configuration.
- The document uses `customer_name` for the account name. The listing uses `name`.
- In the document, `parent` contains the ID of the parent account as a string.
  `AccountSummary.Parent` has a different meaning: the number of subaccounts, which only the
  primary account has. A copy of the ID would report each subaccount as the primary account.
- The document has no count of checks. Thus `active_checks` is 0 for this shape.

**Recorded by:** Atlantic BT · **Date:** 2026-09-21

---

## A-019: Maintenance responses key the windows by ID

**Assumption:** There are four maintenance response shapes. One decoder reads all of them:

| Request | Shape |
|---|---|
| `GET /maintenance` | `customer_id`, `schedules` and `ad-hoc` at the top level. The decoder puts both groups into one map keyed by ID. A window from the `ad-hoc` group is marked, so `IsAdHoc` reports it, whatever its identifier is. |
| `GET /maintenance/:id`, `POST /maintenance`, `PUT /maintenance/:id` | The window, keyed by its generated ID, next to the scalar fields `ok` and `message`. |
| A bare window document | The decoder still accepts it. |

The key of the wrapper is the authoritative identifier. The decoder puts it on each window
that has no identifier.

**Why:** We confirmed the wrapper with curl on two accounts.

- The old code decoded the listing as a map keyed by ID, without the wrapper. Thus the data
  source failed on each account.
- `IsAdHoc` compared the identifier with the text `ad-hoc`. Create accepts that value, but a
  listing does not return it: a live ad-hoc window has its own generated ID. The text check
  stays for the bare-document form, which has no wrapper.

The write path and the single-read path decoded into a bare `Maintenance`, and thus found
nothing. The three results were different, and all were wrong:

| Operation | Result |
|---|---|
| Create | Failed with "API returned no id" *after* NodePing created the window. Each attempt left one window. |
| Read | Found an empty document and reported "not found". This would delete the resource from state on each refresh. |
| Update | Returned a record with only the ID. It wrote an empty name, a zero duration and no checklist into state. |

One shared decoder for the wrapper keeps these three paths the same.

An update that returns no window is now an error. It does not create an empty record. An
empty window in state plans the real settings away on the next run.

**Observed inner shape (2026-09-22, live):** A window has `name`, `enabled`, `processing`,
`cron`, `duration`, `history`, `nextrun`, `checklist` and `update`. `nextrun` is epoch
milliseconds. Only the single-window `GET` has an `id` in the document. The listing, create
and update responses do not. Thus the provider must trust the key.

**Residual risk:** `history` was `{}`, and `update.user` was `""`, on each window that we
saw. Thus we do not know their full shapes. `MaintenanceRun` and the non-empty form of
`update.user` come from the documentation. We created no ad-hoc window during this work. We
saw the `ad-hoc` group only when it was empty. Its grouping behavior comes from the
documentation.

**Recorded by:** Atlantic BT · **Date:** 2026-09-22

---

## A-020: `default_tags` is merged into `tags_all`

**Assumption:** `nodeping_check.tags` contains only the tags in the configuration. It always
agrees with the configuration exactly. The provider merges its `default_tags` into a new
computed attribute, `tags_all`. The provider sends `tags_all` to NodePing.

**Why:** The first live acceptance run failed with this error: "Provider produced invalid
plan: planned value [managed-by-tofu, team-devops, tf-acc] does not match config value
[team-devops]".

- Terraform requires the planned value of an attribute to agree with a known configuration
  value. Thus `ModifyPlan` could not write a merged set into `tags`.
- The feature failed for each check that had its own tags. This is the usual case.
- A merge in `Read` only changes the invalid plan into "inconsistent result after apply".
  This is the same rule, one step later.
- The AWS provider uses the same `tags` and `tags_all` split for this problem.

This is a schema change that users can see. It is not an internal fix.

**Consequence for reads:** NodePing stores only the merged set. `Read` takes `tags_all` from
the API. It then calculates `tags`: it removes the configured `default_tags`. If the
practitioner declared a tag that is also a default, `tags` keeps it. The provider uses the
prior state to decide this. A simple removal of all defaults would drop the tag, and each run
would plan its removal. Import has no prior state. It always removes the defaults.

**Accepted limitation:** The calculation uses the *current* `default_tags`. Consider a tag
that was a default when you created the check. If you later remove it from the provider
configuration, the next refresh puts it in `tags`. This is the only reading that the API
response gives. It is also the safe one: the tag stays on the check. The provider does not
delete it.

**Recorded by:** Atlantic BT · **Date:** 2026-09-22

---

## A-021: Target comparison follows the normalization of NodePing

**Assumption:** `normalizeTarget` makes only the changes that we saw the live API make, for
each check type:

| Check types | Normalization |
|---|---|
| All types | Remove whitespace at the start and the end. |
| `HTTP`, `HTTPADV`, `HTTPCONTENT`, `HTTPPARSE`, `SSL`, `AUDIO`, `MONGODB`, `PGSQL`, `REDIS` | Change the scheme and the host to lower case. Change an empty path to `/`. Percent-encode characters that are not legal in a path. |
| All other types | No change. |

The function does not change the port (it keeps a default port), duplicate slashes, dot
segments, user information, the query or the fragment.

**Why:** The first live acceptance run failed an import round trip. NodePing stores
`https://example.com` as `https://example.com/`. The old `normalizeURL` removed a trailing
slash from all targets. That was wrong in two ways:

- It made `https://example.com/path/` and `https://example.com/path` equal, but NodePing
  keeps them different. This hides a real change.
- It applied to check types that NodePing does not normalize. This reports a change that
  does not exist.

**Evidence:** We sent each type in `ValidCheckTypes` to the live API in the test subaccount.
Then we read the stored target.

- `WEBSOCKET`, `FTP`, `RDAP`, `SIP` and `WHOIS` accept a URL and store it with no change.
  Thus the shape of the scheme cannot decide the type list. The list is explicit for this
  reason.
- `PING`, `PORT`, `DNS`, `RBL`, `SMTP` and `IMAP4` keep the case of the host.
- Do not add a check type to the list without the same test.

**Not done:** NodePing changes a non-ASCII host to punycode. For example, it stores
`https://例え.com` as `https://xn--r8jz45g.com/`. We do not know how it percent-encodes
non-ASCII path bytes. The first change needs an IDNA dependency for a case that we have not
seen. For a target that has either form, the provider compares only as far as the rules
above go. Thus it can report a difference that it could remove. This is the safe direction:
the diff disappears after one apply. It does not hide a real change.

**Recorded by:** Atlantic BT · **Date:** 2026-09-22

---

## A-022: An empty `customer_id` is an error

**Assumption:** `customer_id = ""` is a configuration error. The attribute has
`stringvalidator.LengthAtLeast(1)`. To use the account of the token, do not set the
attribute.

**Why:**

1. `clientFor` reads an empty string as "use the client of the provider". Thus the provider
   creates the check in the account of the token.
2. `mapForApply` keeps each value that the plan knows. Thus state keeps `""`, but the API
   has the check under a real account ID.
3. The first refresh reads that ID. `customer_id` has `RequiresReplace`, so the difference
   causes a destroy and a create on each plan after that.
4. With resource identity, the framework stops before this. It stops the refresh with
   "Unexpected Identity Change" and reports a provider bug.

**Evidence:** We reproduced this against a test account without the validator. The refresh
plan after the apply failed. The identity changed from `customer_id: ""` to the real customer
ID of the test account, on the same check ID.

**Rejected alternative:** Change an empty `customer_id` to the account that the API reports.
This changes a value that the practitioner wrote, without a message, on an attribute that
forces replacement. Also, `customer_id = ""` would then mean different things for different
tokens.

**Recorded by:** Atlantic BT · **Date:** 2026-09-22

---

## A-023: A failed create records the identifier

**Assumption:** Sometimes NodePing creates an object, and then `Create` must return an error.
In this case, the provider writes the identifier of the object into state. It still returns
the error. It does not write the planned values. It does not write identity data.

**Why:** Each `Create` has the same steps: call the API, do more work, then call
`resp.State.Set`. A return between these steps leaves an object in NodePing that no state
file records. OpenTofu cannot refresh it or destroy it. The next apply creates a second
object.

During acceptance tests, we deleted three `nodeping_schedule` objects and five
`nodeping_maintenance` objects manually for this reason. `nodeping_subaccount` has the
highest risk: the POST creates a real account, and two more API calls occur before the
provider writes state.

**Evidence that partial state stays:**

- terraform-plugin-framework v1.19.0 (`internal/fwserver/server_createresource.go`) rejects a
  null create state only when there are no error diagnostics. It does not check identity
  when there is an error.
- OpenTofu apply (`internal/tofu/node_resource_abstract_instance.go`) saves any non-null
  object that the provider returns when apply reports errors (the
  `diags.HasErrors() && !newVal.IsNull()` branch). In the same case, it does not run
  `AssertObjectCompatible`. Thus it accepts required attributes that are null.
- The unit tests in `check`, `schedule` and `subaccount` make sure that `resp.State` contains
  the identifier after a forced failure after create.

**Why only the identifier:** OpenTofu changes each unknown value in an applied object to
null. It reports each one as a provider bug. The planned model usually has unknowns for
computed attributes. Thus a write of the full model gives many incorrect errors in addition
to the real one. The provider writes `customer_id` with the ID when the practitioner set it,
because `Read` needs it to find the object in the correct subaccount. When it is not set, the
object is in the account of the token, and null means the same thing.

**Why identity stays null:** `customer_id` is often still unknown at this point. If the
stored identity is different from the identity that the next `Read` calculates, the framework
rejects it with "Unexpected Identity Change" (terraform-plugin-framework
`internal/fwserver/server_readresource.go`). That would block the recovery that the partial
state makes possible. The framework does not do this check when the stored identity is fully
null. `Read` sets the identity on the next refresh.

**Where the provider does not do this:** `contact`, `contactgroup`, `maintenance` and
`notificationprofile` have no step that can fail at that point.

- `notificationprofile.Create` has no return between the API call and `State.Set`.
- In the other three, the only step converts plain Go slices and maps into framework
  collections (`types.SetValueFrom` or `types.MapValueFrom` over `[]string` or
  `map[string]string`). This cannot give a diagnostic for any response that the client
  returns.

A guard there would be code that never runs. If someone adds a step that can fail to one of
these `Create` methods, that step needs the same guard.

**Limitation: the identifier is not always known.** When the create call itself returns an
error, the provider has no ID. For `check`, `contact`, `contactgroup`, `maintenance`,
`notificationprofile` and `subaccount`, only the response body contains the ID. Thus a
request that NodePing received and applied, but whose response was lost or could not be
decoded, still leaves an object that state does not record. The API has no idempotency key
for these types. It also has no create form where the caller supplies the ID. A search by
name after a failure would be a guess: many objects can have similar names.

`nodeping_schedule` is different. The practitioner selects its ID, and the endpoint is a PUT.
Thus the ID is known, whether the call succeeds or not. The provider records it on any
failure after it gives the request to the client.

**Recorded by:** Atlantic BT · **Date:** 2026-09-22

---

## A-024: `suppress_diag` accepts only `false`

**Assumption:** `suppress_diag = true` on a `nodeping_contact` address is a configuration
error. The attribute stays in the schema, and the provider accepts `false`. Thus existing
configurations and state do not change.

**Why:** NodePing accepts `suppressdiag` in a contact address. It stores nothing and never
returns it. Each similar flag (`suppressup`, `suppressdown`, `suppressfirst`, `suppressall`)
does a full round trip. Thus the provider planned `true` and read back `false`. OpenTofu then
stopped the apply with "Provider produced inconsistent result after apply … This is a bug in
the provider". That message tells the practitioner to look at the provider for a limitation
of the API.

**Evidence:**

- We sent `POST /contacts` with `suppressdiag: true` on email, SMS and webhook addresses, in
  the test subaccount. The created document did not have the field in any of the three
  cases. A similar flag in the same request came back set.
- `PUT /contacts/:id` has the same behavior.
- We reproduced it through the provider before the validator existed. The first step of
  `TestAccContact_suppressDiagRejected` failed with `.address[0].suppress_diag: was cty.True,
  but now cty.False`.

**Rejected alternative:** Keep the planned `true` in state. State would then show a
suppression that is not in effect. That is worse than a rejection.

**Recorded by:** Atlantic BT · **Date:** 2026-09-22

---

## A-025: Contact updates always send addresses

**Assumption:** `ContactUpdateRequest.Addresses` has no `omitempty`.

| Case | The provider sends |
|---|---|
| The update keeps no existing address | `"addresses": {}` |
| The contact has no addresses and gets none | `"addresses": null` |
| The update would leave the contact with zero addresses | Nothing. The provider refuses the update before the request. |

**Why:** NodePing reads `addresses` as the complete set. It deletes each address that is not
in it.

With `omitempty`, an update where all planned addresses were new removed the map from the
request. An example is the replacement of the only email of a contact with an SMS, where
nothing agrees by content, ID or position. NodePing then added the new address and kept the
old address. The apply failed with "block count changed from 1 to 2". The contact kept an
address that the configuration had removed.

The two empty forms are not the same:

- NodePing refuses `{}` with "Account must have at least one 'owner' contact" when it would
  leave the contact empty. This also occurs for a contact that is already empty.
- NodePing reads `null` as "not specified". It does not change the addresses.

Thus the provider sends `{}` only when `newaddresses` gives the set new members.

**Evidence:** We tested against the test subaccount:

| Request | Result |
|---|---|
| `PUT` with `addresses: {}` and a `newaddresses` entry | NodePing replaces the set. |
| `PUT` with `addresses: {}` only | `{"error": "Account must have at least one 'owner' contact."}`, whether the contact has addresses or not |
| `PUT` with `addresses: null` | Success. NodePing keeps the addresses. |

**Rejected alternative:** Let the API refuse the update to zero addresses. The practitioner
would see a message about account owners, then an "inconsistent result" error that names the
provider. NodePing does not support the operation, and the provider should say so.

**Recorded by:** Atlantic BT · **Date:** 2026-09-22

---

## A-026: A contact without a name is null in state

**Assumption:** `Read` and `ImportState` map an absent or empty contact name to
`types.StringNull()`.

**Why:** `name` is Optional and not Computed. Thus a contact that you create without a name
has null in state. NodePing does not return the `name` key for such a contact. The provider
wrote the resulting `""` back, and each refresh then planned an update from null to `""`. No
apply could fix it: the create request removes an empty name (`omitempty`). Thus the API never
gets the value that the plan asks for.

**Evidence:** `POST /contacts` without `name` returns a document without a `name` key.
Through the provider, the refresh plan after the apply was not empty on any run.
`TestAccContact_withoutName` is the regression test. Its `PlanOnly` step is the assertion.

**Limitation:** You cannot now tell an explicit `name = ""` from an absent name. It still
shows a diff. NodePing cannot store an empty name. Thus the provider cannot give that
configuration a meaning.

**Recorded by:** Atlantic BT · **Date:** 2026-09-22

---

## A-027: The address order after import is not the configuration order

**Assumption:** The acceptance tests verify the `address` blocks of an imported contact with
`ImportStateCheck`. They compare them with the set of blocks that apply wrote. They exclude
`address` from `ImportStateVerify`.

**Why:** NodePing returns addresses as a map keyed by an ID that the server gives.

- Without a plan, `mapAddressesToModel` puts them in order by that ID. The ID is random for
  each address.
- An apply puts them in the order of the configuration.
- `ImportStateVerify` compares by list position. Thus it would pass or fail on the IDs that
  NodePing gave. That is a flaky test, not a real signal.

The set comparison checks a stronger property: each block, with its server ID, comes back
from the API with no change.

**Consequence for practitioners:** After you import a contact with more than one address, the
first plan can show the blocks in a new order. The apply changes nothing in the API: the
provider matches addresses by content, so no ID moves. The next plan is empty. Sorting cannot
fix this. No order from the API response can agree with any order in a configuration.

**Recorded by:** Atlantic BT · **Date:** 2026-09-22

---

## A-028: The provider accepts all shapes found in a large import

**Assumption:** The provider accepts each value that NodePing returned during the import of
a large production account tree on 2026-09-30. It does not reject unexpected data.

**Why:** That import failed on many objects, for eight causes. All causes were in the
provider:

| Cause | Fix |
|---|---|
| `queue` is `false` on each disabled check, not a queue name. A disabled check could not be read. | The provider never used the field. It does not decode it now. |
| A schedule day with no hours has `time1` and `time2` set to `false`. | `ScheduleTime` reads `false` and `null` as empty. |
| Validators that were stricter than NodePing refused `sens = 0` (on a PUSH check) and `statuscode = 0` (on HTTPADV checks). | The validators accept zero. |
| A type-specific value stayed on a check after a change of type, for example `contentstring` on SSL checks. This was an error. | It is now a warning. A boolean flag set to true for the wrong type is still an error, because no live check has one. |
| The provider refused `custrole = "owner"`. | The provider accepts it. |
| Owner and login contact addresses have no `type`. `GET /contacts/:id` returns the addresses of a login contact as an array without keys. | `type` is Optional, null when absent, and still required on a new address. The array form is keyed by the address, as `GET /contacts` keys it. |
| A webhook query-string value can be a JSON object. | `FlexStringMap` keeps it as JSON text. It sends text that parses as an object or an array back as JSON. Thus an update does not change the object into a string. A literal string that is a valid JSON object would go as an object. No live value is like that. |
| `nodeping_subaccount` required `contactname`, `email` and `location`. NodePing never returns them. No subaccount could be imported. | They are Optional. `ModifyPlan` requires them when you create a subaccount. Import takes `location` from `default_locations`. The provider reads `emailme` from the API, which returns it as a boolean. |

**Recorded by:** Atlantic BT · **Date:** 2026-09-30

---

## A-029: A check without tags plans a null `tags_all`

**Assumption:** When the configuration and the provider defaults give a check no tags, and
state has no tags (null), the provider plans `tags_all` as null.

**Why:** NodePing does not return the `tags` key for a check that never had tags. `Read`
stores null. A plan of `[]` against that null was an update in place on almost each check of
an account that was imported for the first time. The provider still plans `[]` when state has
tags, because `[]` tells NodePing to clear them.

**Recorded by:** Atlantic BT · **Date:** 2026-09-30

---

## A-030: Integer zeros are sent as strings

**Assumption:** The provider sends `sens`, `statuscode`, `warningdays`, `volumemin` and
`port` as the string `"0"` when the value is zero. It sends other values as JSON numbers.

**Why:** NodePing reads a JSON number `0` as empty.

- A create with `sens: 0` fails with "Unable to create check".
- NodePing stores `statuscode: 0` or `warningdays: 0` as `""`. Thus the value never becomes
  stable.
- NodePing stores the string `"0"` as zero.

The acceptance tests found this on 2026-09-30. Direct API calls against the test subaccount
confirmed it.

**Recorded by:** Atlantic BT · **Date:** 2026-09-30

---

## A-031: WHOIS and RDAP intervals follow the NodePing minimums

**Assumption:** For `WHOIS` and `RDAP` checks, an unset `interval` plans `1440`. The provider
rejects an explicit interval below `240` (WHOIS) or `60` (RDAP) at plan time.

**Why:** NodePing replaces a shorter interval on these types with `1440`, on create and on
update. Thus the default of `15` caused drift on each plan.

**Recorded by:** Atlantic BT · **Date:** 2026-09-30

---

## A-032: SPEC10DNS is not a DNS type for attribute rules

**Assumption:** `dnstype`, `dnstoresolve`, `dnssection` and `dnsrd` do not apply to
`SPEC10DNS` checks.

**Why:** NodePing keeps none of them on a SPEC10DNS check. A configured value never came
back, and each plan showed a change.

**Recorded by:** Atlantic BT · **Date:** 2026-09-30

---

## A-033: `contactname` and `email` cannot change on an existing subaccount

**Assumption:** A change of `contactname` or `email` on a subaccount that already has a value
is a plan-time error. On an imported subaccount without a value, the provider records the
configured value. It makes no API call.

**Why:** NodePing does not return or update these fields. The old behavior forced a
replacement. A replacement of a subaccount deletes each check and contact in it. That is
never an acceptable method to change a contact name. This entry replaces the
`requiresReplaceIfWasSet` design in A-005.

**Recorded by:** Atlantic BT · **Date:** 2026-09-30

## A-034: CI enforces statement coverage only

**Assumption:** CI fails when total statement coverage is below 90%. It does not measure
branch, function or line coverage.

**Why:** Go's coverage tool reports statements only. The ABT floors for branches (75%),
functions (80%) and lines (80%) have no standard Go measurement, and a third-party tool
for them would add a dependency to CI. The gate is 90%, above the ABT 80% floor, because
coverage was 95.3% when the gate was added. A lower gate would let it fall a long way
without a failure.

**Recorded by:** Atlantic BT · **Date:** 2026-10-15
