<!--
doc: changelog
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# Changelog

This file records all important changes to this provider. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). The version numbers follow
[SemVer](https://semver.org/) (Semantic Versioning).

## [Unreleased]

This is the first public release from `github.com/atlanticbt/terraform-provider-nodeping`.
The OpenTofu registry publishes it as `atlanticbt/nodeping`.

### Breaking changes

- The provider does not read the API token from `NODEPING_API_KEY` now. Set `api_token`
  or `NODEPING_API_TOKEN`. The documentation never described this fallback. An unknown
  credential source can give a token with more permissions than you expect.

### Security

- The build uses Go 1.26.9 and `golang.org/x/net` v0.60.0. These versions fix nine
  vulnerabilities that the HTTP client of the provider can reach (GO-2026-6603 to
  GO-2026-6617).

### Fixed

- The provider now uses `max_retries = 0`, `retry_wait_min = 0` and `retry_wait_max = 0`
  as written. Before, it replaced them with the default values.
- A response larger than 16 MiB now fails with a clear error that names the endpoint.
  Before, the limit was 1 MiB and the error was about truncated JSON.
- API error messages do not stop in the middle of a multi-byte character now.

### Added

- Length limits for check `label`, `target`, `contentstring` and `postdata`, and for
  contact `name`, `address` and `data`. The provider checks them at plan time. NodePing
  has no limits. Thus these limits come from the provider. They are much larger than
  usual values.
- GitHub Actions CI (continuous integration): format, vet, golangci-lint with security
  linters, race tests, govulncheck and gitleaks. Also signed releases and a weekly live
  acceptance test run.

### Changed

- One package now contains the resource identity logic and the logic that records a
  created object after a failed create. The check resource is now in a set of files, one
  for each concern. The behavior did not change.

## [1.1.0] - 2026-10-05

This release was verified against the live NodePing API. Each resource now has acceptance
tests. The tests and a large production import found defects. This release fixes them.
`docs/assumptions.md` records the API behavior (A-016 to A-033).

### Fixed

- `nodeping_schedule` could not create a schedule. The provider took the identifier from
  the response body, but that field contains the account ID.
- `nodeping_maintenance` could not create, update or read windows. NodePing returns them
  in a wrapper, keyed by ID.
- `default_tags` gave an invalid plan for checks that have their own tags. The merged set
  is now the computed attribute `tags_all`.
- Checks without tags planned a `tags_all` change on each run.
- The provider could not read disabled checks. NodePing returns `queue` as `false` for
  them.
- The provider could not read schedules with days off. NodePing returns `time1` as `false`
  for those days.
- These objects could not be imported: owner and login contacts, contacts with an object
  value in a webhook query string, and subaccounts.
- Zero values of `sens`, `statuscode`, `warningdays`, `volumemin` and `port` were stored
  as empty. NodePing reads a numeric `0` as empty. The provider now sends them as `"0"`.
- WHOIS and RDAP intervals below the NodePing minimum caused drift on each run.
- These cases gave inconsistent results: `suppress_diag = true`, a replacement of all the
  addresses of a contact, and contacts without a name.
- A failed create could leave an object in NodePing that state did not record.

### Changed

- A change of `contactname` or `email` on an existing subaccount is now a plan-time error.
  Before, it replaced the subaccount. A replacement deletes all objects in the subaccount.
- `sendheaders`, contact `address`, `headers` and `querystrings` are not sensitive now.
  Thus generated configuration contains their values (A-015).

## [1.0.0] - 2026-08-27

This is the first Atlantic BT release. It completes the API coverage. It keeps credentials
out of state. It increases test coverage from 11.3% to 95.2%.

The provider is a fork of `github.com/phizzl/terraform-provider-nodeping` at commit
`8fdc0e1` (MPL-2.0). See [NOTICE.md](NOTICE.md) for attribution and origin.

### Added

**Five new resources. API coverage is now 7 of 7.**

- `nodeping_contactgroup`: groups of contact addresses for notifications. The members are
  contact **address** IDs, not contact IDs. A contact has many addresses. An address joins
  a group, not a contact.
- `nodeping_schedule`: named notification windows. NodePing has no `POST` for schedules.
  `PUT` creates and updates. The caller selects the ID. Thus a new name replaces the
  schedule. Each day is one of seven optional nested attributes, not a map key. Thus a
  spelling error in a day name is a plan-time error.
- `nodeping_notificationprofile`: notification recipient sets that you can use again. It
  uses the notification reconciliation of the check resource. The order is always the
  same. Repeated blocks for the same contact stay separate.
- `nodeping_maintenance`: scheduled maintenance windows.
- `nodeping_subaccount`: subaccount creation. `POST` creates subaccounts only. You cannot
  manage the parent account as a resource.

**Ten new data sources:** a singular and a plural data source for each new resource, plus
`nodeping_account` and `nodeping_accounts`.

**Write-only check credentials**

- `password_wo` and `password_wo_version` on `nodeping_check`. NodePing gets the
  plaintext. The provider never writes it to state or to a saved plan file. This was
  verified with OpenTofu 1.12.5: the schema loads, `req.Config` contains the value, state
  contains `null`, and the secret is not in the output of `tofu plan -out`.
- The provider reads the value from `req.Config` only. For write-only attributes,
  `req.Plan` and `req.State` are always null.
- `Read` does not put the password that NodePing returns into any regular attribute. Thus
  the plaintext from the API cannot go back into state.
- REDIS host credentials have their own write-only path. See below.
- The `password` attribute is deprecated but it operates. You cannot set both. A
  configuration validator enforces this.

**Fourteen check attributes.** The client supported them, but the schema did not show
them: `data`, `fields` (with `match`), `hosts`, `redistype`, `sentinelname`, `checktoken`,
`oldresultfail`, `edns`, `dohdot`, `whoisserver`, `rdapurl`, `ignore`, `verifyvolume`,
`volumemin`. You can now fully manage REDIS, AUDIO, WHOIS, RDAP, DOHDOT, PUSH and AGENT
checks. Before, you could only create them with default values.

**Validation**

Plan-time validators for `redistype`, `dohdot`, `volumemin` (−90 to 0), REDIS host ports,
`rdapurl` and EDNS keys (they must be positive integers). Cross-field validation:
`redistype = "sentinel"` needs `sentinelname`, and `password_wo_version` needs
`password_wo`.

**Acceptance tests** for each resource. They include `ImportStateVerify` round trips. One
test makes sure that the write-only password is not in state after create, rotation and
import. These tests compile, and `TF_ACC` controls them. **They did not run for this
release.** See Known limitations.

**`docs/assumptions.md`:** fourteen recorded decisions and accepted risks.

**Resource identity** (OpenTofu 1.12)

All seven resources implement `resource.ResourceWithIdentity`. Thus you can use
`import { identity = { ... } }` blocks. You do not need to parse import ID strings. The
identity is `id` and `customer_id` for the six scoped resources. It is `id` only for
`nodeping_subaccount`, because that resource is the account. This is safe for old and new
cores: a core before 1.12 never calls the identity RPCs. The string import forms (`ID` and
`CUSTOMERID:ID`) operate as before.

The provider sets the identity in Create, Read, **Update** and ImportState. Update is
necessary. The framework gives the error "Missing Resource Identity After Update" if the
identity is null after an update. This can occur for a resource that a core without
identity support created, when you then apply with `-refresh=false`.

**Provider configuration validation**

- `api_url` must use `https`, except for a loopback host. An unencrypted endpoint sends
  the API token as HTTP basic authentication in cleartext on each request. Before, there
  was no protection. The provider checks the attribute and also checks again in
  `Configure`. Attribute validators cannot see `NODEPING_API_URL`, and that variable is
  where the risk is. There is no `allow_insecure_url` option. A configuration flag can go
  into production when someone copies a file. An SSH tunnel to `127.0.0.1` covers the
  correct uses and leaves nothing permanent.
- `rate_limit` must be more than zero. `max_retries`, `retry_wait_min` and
  `retry_wait_max` must not be negative. `retry_wait_min` must not be more than
  `retry_wait_max`. Before, the provider changed any value `<= 0` to a default without a
  warning. For example, `rate_limit = -1` gave 10 requests per second.
- `default_tags` rejects empty strings. You cannot search or filter an empty tag in
  NodePing. An empty tag is usually an unset variable.

**Write-only REDIS host credentials**

`hosts[*].password_wo` and `hosts[*].password_wo_version` keep REDIS host passwords
completely out of state. This removes an accepted gap in drift detection.
terraform-plugin-framework v1.19.0 supports `WriteOnly` in a `MapNestedAttribute`. We
verified this in the framework source. The containing map attribute must not be
`Computed`. Thus this change needed the `hosts` change below. `hosts[*].password` stays for
migration and is deprecated.

**Documentation drift check**

A test examines each resource and data source schema. The build fails when the
documentation page does not mention an attribute of the schema. The first run found three
real gaps: `email` on the check resource, and `dep` on the two check data sources. The
pages are still written manually. tfplugindocs would delete the text in them. The roadmap
has an item to move to templates that keep the text.

**Ad-hoc maintenance alternative**

`scripts/nodeping-adhoc-maintenance.sh` sends the API call directly. A `terraform_data`
resource with a `local-exec` provisioner runs it. The maintenance resource documentation
has the recipe. The script is outside provider state on purpose, and this makes it safe.
OpenTofu never tries to reconcile the window. Thus the loop of creation and deletion that
makes ad-hoc maintenance unmanageable cannot occur. The script:

- validates and limits each input
- rejects check IDs that contain characters other than letters, digits and hyphens
- requires an `https` endpoint outside loopback
- reads the token from the environment, so that the token is not in `ps` output

**Release tooling** to publish builds. CI releases to the public registries, started by a
tag, replaced it in 1.1.x.

### Changed

- `.gitignore` now covers plan files (binary and JSON), state with backups and lock
  metadata, `*.tfvars` (these files often contain secrets) and local CLI configuration
  (`.tofurc`, `.terraformrc`). A comment gives the reason for each pattern.
- `CheckCreateRequest.Tags` is now `*[]string`. A plain slice with `omitempty` removed an
  empty array. Thus "remove the last tag" did nothing.
- `terraform-plugin-testing` v1.16.0 is now a real dependency.
- **GPG signatures were temporarily disabled in the release pipeline.** Registry releases
  sign their checksums again.
- **A removal of any of 24 check attributes now gives a diff.** Before, too many
  attributes were `Optional + Computed`. That combination is necessary only when NodePing
  supplies a value that the practitioner did not set. For attributes that the API only
  returns, plain `Optional` is correct, and a removal gives a correct diff. Changed to
  `Optional`: `description`, `contentstring`, `sendheaders`, `receiveheaders`,
  `postdata`, `data`, `fields`, `hosts`, `edns`, `username`, `email`, `query`,
  `database`, `namespace`, `whoisserver`, `rdapurl`, `ignore`, `servername`, `sshkey`,
  `clientcert`, `sentinelname`, `dnstoresolve`, `dep` and the deprecated `password`.
  Attributes with documented server defaults, schema defaults or values that the API
  always returns stay `Computed`. A wrong change causes a hard failure on each apply. Thus
  we kept `Computed` when we were not sure. A-007 lists the attributes that we kept and
  the reasons.

### Fixed

- **A 200 response with `{"error": ...}` could delete a resource from state and create the
  live object again.** NodePing returns errors with HTTP 200 on some endpoints.
  `nodeping_contactgroup`, `nodeping_notificationprofile`, `nodeping_maintenance` and
  `nodeping_subaccount` checked only for an empty decoded document. An error document
  decodes to an empty struct. Thus a temporary `permission denied` or `rate limit
  exceeded` meant "resource absent". OpenTofu then removed the resource from state and
  created it again on the next apply. Now the classification of "not found" is
  conservative. It uses a short list of message fragments. It does not apply to statuses
  that the client can retry. Tests make sure that unknown error messages show as real
  errors.
- **`nodeping_check` and `nodeping_contact` did not accept the non-standard responses of
  NodePing.** They wrote a `{}` with 200 into state as an empty object, and did not remove
  the resource. The shared client now handles both cases. Thus all resources have the same
  behavior: one detector for an error in a 200 response, and one classifier for "not
  found". These replace sixteen copies of the same code.
- **`ListChecks` and the other list methods changed an error in a 200 response into an
  empty collection.** You could not tell this from an empty account. They now return the
  error.
- **The provider configuration rejected valid tokens that came from a different
  resource.** `Configure` checked only `IsNull()`. Thus it read an unknown value (for
  example, `api_token` from the output of a different resource) as an empty string. The
  practitioner got "Missing NodePing API Token", and the provider ignored a correct
  `NODEPING_API_TOKEN` in the environment. Now an unknown value gives the diagnostic that
  the framework recommends: "apply the source first". This applies to `api_token`,
  `customer_id` and `api_url`. Unknown tuning values use the client defaults. They are not
  read as zero.
- **You could not turn off contact `mute`.** The update path did not send the field unless
  it was true. Thus the change never got to the API, and the attribute had drift on each
  run.
- **The check resource had type assertions on `NotFoundError`.** They now use `errors.As`.
  Thus they continue to operate if a future change wraps the error.

### Security

- When you use `password_wo`, check credentials do not go into state (see Added).
- `snmpcom`, `sendheaders`, REDIS host passwords and webhook `headers` and
  `querystrings` are marked `Sensitive`.
- The Go toolchain is pinned to 1.26.6, and the dependencies are upgraded. `govulncheck`
  reports no findings.

### Known limitations

- **This release was not verified against the live NodePing API.** Each resource has
  acceptance tests, but they did not run. They need a token for a dedicated test
  subaccount. This limitation blocks production use. It is the most important item in
  this list. See A-010.
- **Ad-hoc maintenance cannot be a resource.** NodePing deletes the record after the run.
  This would cause a loop of creation and deletion that disables monitoring again and
  again. The provider blocks it. It is not only documented. An alternative is
  `terraform_data` with `scripts/nodeping-adhoc-maintenance.sh`. We do not expect a
  change. The declarative solution would be Terraform `action` blocks, and OpenTofu has
  said that it may not add them. See A-003.
- **Some attributes do not show a diff when you remove them.** This release fixed 24 check
  attributes. The remaining attributes are those for which NodePing supplies a value that
  the practitioner did not set. For those attributes, set the zero value. Do not delete the
  line. A-007 lists them and the reason for each.
- **The provider never refreshes subaccount `contactname` and `email`.** NodePing does not
  return them. Thus you cannot see drift in these fields. See A-005.
- **`checktoken` regeneration is not supported.** The NodePing documentation for this
  attribute contradicts itself. The correct design is a separate resource, not an
  attribute. See A-011.

---

## [0.1.0] - 2026-08-27

This is the first Atlantic BT fork. It contains correctness, security and build fixes from
an audit of the upstream code. It adds no new resources.
