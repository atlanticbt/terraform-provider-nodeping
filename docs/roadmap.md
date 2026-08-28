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
- Coverage 11.3% → 94%+. All `govulncheck` findings cleared.
- Release pipeline on Bitbucket with reproducible builds, secret scanning, and a coverage
  gate.

## Next — the gate on trusting this in production

**1. Run the acceptance tests.** This is the single most important remaining item.
Everything in v1.0.0 is unit-verified against a mock; **nothing has run against the live
NodePing API.** The tests are written and compile. They need a token for a dedicated test
subaccount — never a client subaccount, never the main account.

Specifically unverified, per [assumptions.md](assumptions.md):
- Root-account resources send `customerid=<own account id>` on every call, assumed to be a
  no-op account selector (A-010)
- Clearing all tags via an empty array
- Every response-shape assumption in A-002 — the `{}`-with-200 and `{"error"}`-with-200
  handling was inferred from documentation and defensive coding, not observed

**2. Hoist NodePing response tolerance into the shared client.** Four workstreams
independently implemented not-found detection for `{}`-with-HTTP-200 and
`{"error":...}`-with-HTTP-200. The newer resources have it; `nodeping_check` and
`nodeping_contact` do not, so they will write an empty object into state where the newer
ones correctly remove the resource. One implementation, one place to fix it.

**3. Provider mirror.** A hard dependency, not a convenience — goreleaser cannot publish to
Bitbucket, and private publishing to `registry.opentofu.org` is impossible (public repos
only). Needs the static index layout (`index.json` + `<version>.json` per the network
mirror protocol), a generator step in the release pipeline, and Bitbucket OIDC to an AWS
role so the upload uses no stored access keys. Mirrors require no GPG signing, so the
signing machinery could be dropped to remove the key-management burden.

## Later

- **Resource identity** (OpenTofu 1.12) — enables `import { identity = {...} }`. Safe in
  both directions: older versions never call the RPCs, 1.12 treats `Unimplemented` as a
  warning.
- **`api_url` scheme validation** — a plaintext `http://` URL downgrades the bearer token
  to cleartext. Operator-controlled rather than attacker-controlled, so a footgun rather
  than a vulnerability, but it deserves a guardrail.
- **Generated docs.** The `docs/resources/*` and `docs/data-sources/*` pages are
  hand-written and drift from the schema. Add `tfplugindocs` and a CI drift check.
- **Check token regeneration** (A-011) — needs a separate resource or action, not an
  attribute.
- **Upper bounds for `threshold` and `sens`** — NodePing's documented maxima could not be
  confirmed, so no validator was invented.
- **Deprecated `password` removal** — once consumers have migrated to `password_wo`.

## Known limitations

See [assumptions.md](assumptions.md) for the full register. The ones most likely to
surprise:

- **Optional + Computed suppresses removal diffs** (A-007). Deleting such an attribute from
  config produces no diff; set it to its zero value instead.
- **Ad-hoc maintenance is not manageable** (A-003) and is enforced, not just documented.
- **Subaccount `contactname`/`email` are never refreshed** (A-005) — NodePing does not
  return them.
- **REDIS host passwords are not drift-detected** (A-013) — the cost of never reading a
  credential back into state.
- **NodePing stores check passwords in plaintext** (A-001) regardless of what this provider
  does.
