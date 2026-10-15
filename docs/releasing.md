<!--
doc: releasing
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# Releasing and Repository Settings

This document is for maintainers. GitHub Actions builds and signs each release when a
maintainer pushes a `v*` tag. The workflow is `.github/workflows/release.yml`.

## Summary

| Task | Frequency | Section |
|---|---|---|
| Create the release signing key | One time | [Release signing key](#release-signing-key) |
| Make a release | Each release | [Make a release](#make-a-release) |
| Register with the registries | One time, after the repository is public | [Register with the registries](#register-with-the-registries) |
| Make the repository public | One time | [Make the repository public](#make-the-repository-public) |
| Set up scheduled acceptance tests | One time | [Scheduled acceptance tests](#scheduled-acceptance-tests) |

## Release signing key

The OpenTofu and Terraform registries verify each release. They check a detached GPG
(GNU Privacy Guard) signature of the `SHA256SUMS` file. They use the public key that is
registered for the `atlanticbt` namespace.

| Fact | Value |
|---|---|
| Key type | RSA 4096. The Terraform Registry accepts only RSA and DSA keys. The OpenTofu registry accepts any ASCII-armored PGP key. |
| Key owner | A dedicated release key. Do not use a personal key. |
| Key name | `Atlantic BT NodePing provider releases` |
| Validity | 3 years |

### Create the key

> **SECURITY:** The private key signs every release. A person with this key can publish a
> release that users trust. Do not keep the private key file after you complete step 5.

1. Create the key:

   ```sh
   gpg --quick-generate-key "Atlantic BT NodePing provider releases" rsa4096 sign 3y
   ```

2. Find the key ID:

   ```sh
   gpg --list-secret-keys --keyid-format=long "Atlantic BT NodePing provider releases"
   ```

3. Export the private key for the Actions secret:

   ```sh
   gpg --armor --export-secret-keys <KEY_ID> > release-key.asc
   ```

4. Export the public key for the registries:

   ```sh
   gpg --armor --export <KEY_ID> > release-key.pub
   ```

5. In GitHub, go to **Settings > Environments > release**. Add these environment secrets.
   Do not add them as repository secrets: every workflow run can read a repository
   secret, and only `v*` tags can deploy to the `release` environment.

   | Secret | Value |
   |---|---|
   | `GPG_PRIVATE_KEY` | The contents of `release-key.asc` |
   | `PASSPHRASE` | The passphrase of the key |

6. Delete `release-key.asc` from your computer.
7. Put the key and the passphrase in the team password manager.

### Rotate the key

1. Create a new key. Use the procedure in [Create the key](#create-the-key).
2. Register the new public key with both registries.
3. Do steps 1 and 2 before the next release. If you do not, the registries reject the
   release.

## Make a release

The release workflow has three jobs. The `release` job runs only when the other two jobs
pass.

| Job | What it does |
|---|---|
| `verify-tag` | Stops the release if the tagged commit is not on `main`. |
| `ci` | Runs the full CI suite on the tagged commit. See [CI status checks](#ci-status-checks). |
| `release` | Builds the six platform archives with GoReleaser. Signs `SHA256SUMS`. Publishes a GitHub release. Runs in the `release` environment, which holds the signing secrets. It does not restore a Go build cache. |

### Procedure

> **SECURITY:** A pushed `v*` tag starts a signed public release. Tag only a commit that
> has an approved pull request on `main`.

1. Merge all changes for the release to `main`.
2. Update `CHANGELOG.md`. Merge that change to `main`.
3. Set your own signing key in Git. Do this one time only:

   ```sh
   git config --global user.signingkey <YOUR_KEY_ID>
   ```

4. Make a signed tag:

   ```sh
   git tag -s v1.2.0 -m "v1.2.0"
   ```

5. Push the tag:

   ```sh
   git push origin v1.2.0
   ```

6. Go to **Actions > Release**. Make sure that all three jobs pass.
7. Make sure that the GitHub release contains these files:
   - The six platform archives
   - `terraform-provider-nodeping_<version>_SHA256SUMS`
   - The signature file for `SHA256SUMS`
   - `terraform-provider-nodeping_<version>_manifest.json`

After you register the provider, both registries find new GitHub releases automatically.

## Register with the registries

Do this one time, after the repository is public.

### OpenTofu registry

The address is `registry.opentofu.org/atlanticbt/nodeping`. Submissions go through the issue
forms in the `opentofu/registry` repository. Do not use pull requests or the API.

1. Make your membership of the `atlanticbt` organization public. The key form requires
   this.
2. Open the **Submit new Provider Signing Key** form. Enter these values:
   - Namespace: `atlanticbt`
   - Provider name: `nodeping`
   - Key: the contents of `release-key.pub`
3. Open the **Submit new OpenTofu Provider** form. Enter the repository:
   `atlanticbt/terraform-provider-nodeping`.

### Terraform Registry (optional)

1. Sign in to registry.terraform.io with GitHub.
2. Add the same public key to the signing keys of the `atlanticbt` namespace.
3. Publish the repository.

## Repository settings

These settings work on a private repository on GitHub Free. They were applied on
2026-10-15. Check them again after any change to the organization's plan.

| Setting | Value |
|---|---|
| **Actions > General > Actions permissions** | Allow GitHub-owned actions and these patterns: `crazy-max/ghaction-import-gpg@*`, `goreleaser/goreleaser-action@*`, `opentofu/setup-opentofu@*`. Require actions to be pinned to a full-length commit SHA. |
| **Actions > General > Workflow permissions** | Read-only. Actions cannot approve pull requests. |
| **Environments > `release`** | Deployment tags: `v*` only. Holds `GPG_PRIVATE_KEY` and `PASSPHRASE`. |
| **Environments > `acceptance`** | Deployment branches: `main` only. Holds the NodePing test-account secrets. See [Scheduled acceptance tests](#scheduled-acceptance-tests). |
| **Code security** | Dependabot alerts and Dependabot security updates on. |
| **General** | Wiki and Projects off. Head branches are deleted after merge. |

To add an action from a new owner, add its pattern to the allowed list first. If you do
not, every workflow that uses it fails at startup.

## Make the repository public

The organization uses GitHub Free. Branch rulesets and secret scanning are not available for
a private repository on this plan. They become available when the repository is public.

> **SECURITY:** When you make the repository public, all of its history becomes public.
> Before step 1, scan the full history:
> `go run github.com/zricethezav/gitleaks/v8@v8.30.1 git --redact .`
> The only accepted finding is the one in `.gitleaksignore`. Then delete and recreate the
> GitHub repository and push again, or ask GitHub Support to purge unreachable commits.
> After a force-push, GitHub keeps old commits reachable by their hash.

> **SECURITY:** Until you complete step 2, anyone with write access can push to `main`.
> Do all steps in one session.

1. Go to **Settings > General > Danger Zone > Change visibility**. Select public.
2. Go to **Settings > Rules > Rulesets > New branch ruleset**. Target the default branch.
   Set these rules:
   - Restrict deletions.
   - Block force pushes.
   - Require a pull request before merging, with 1 approval.
   - Require review from Code Owners.
   - Dismiss stale approvals when new commits are pushed.
   - Require these status checks to pass: `fmt-vet`, `lint`, `test`, `vulncheck`,
     `secret-scan`, `trivy`, `workflow-audit`.
   - Require the branch to be up to date before merging.
   - Keep the bypass list empty.
3. Go to **Settings > Rules > Rulesets > New tag ruleset**. Target `v*`. Set these rules:
   - Restrict creations, updates and deletions.
   - Add the `nodeping-maintainers` team to the bypass list, for creation only.

   This ruleset stops other persons from creating a release tag. It also stops all persons
   from moving or deleting a published tag.
4. Go to **Settings > Code security**. Enable these features:
   - Secret scanning
   - Push protection
   - Private vulnerability reporting. `SECURITY.md` tells reporters to use it.
5. Add the checks that GitHub Free does not offer on a private repository:
   - **CodeQL** for Go: **Settings > Code security > Code scanning > Set up > Default**.
   - **Dependency review** on pull requests: a `pull_request` job with
     `actions/dependency-review-action`, pinned to a commit SHA, set to fail on HIGH.
     Add `dependency-review` to the required status checks in step 2.
   - **OpenSSF Scorecard**: `ossf/scorecard-action`, pinned to a commit SHA, on a weekly
     schedule. Add `ossf/scorecard-action@*` to the allowed actions first.
   - **Build provenance and SBOM** for releases: `actions/attest-build-provenance` in the
     release job, and an `sboms` section in `.goreleaser.yml`. The release job then needs
     `id-token: write` and `attestations: write`.
6. Make sure that the team `@atlanticbt/nodeping-maintainers` exists and has write access.
   If it does not, `CODEOWNERS` cannot find the code owners.

## CI status checks

`.github/workflows/ci.yml` runs on each pull request and on each push to `main`. The
release workflow also runs it. The job names are the required status checks.

| Check | What it runs |
|---|---|
| `fmt-vet` | `gofmt -l`, `go vet`, `go mod tidy -diff`, `go mod verify`, and `tofu fmt -check` on `examples/` |
| `lint` | golangci-lint v2.14.0 |
| `test` | Unit tests with the race detector, in random order (`-shuffle=on`). Fails when statement coverage is below 90%. See A-034 in [assumptions.md](assumptions.md). |
| `vulncheck` | govulncheck v1.8.0 |
| `secret-scan` | gitleaks v8.30.1 on the commits that the change adds |
| `trivy` | Trivy v0.75.0 on the working tree: HIGH and CRITICAL vulnerabilities, secrets and misconfigurations |
| `workflow-audit` | zizmor v1.30.1 on `.github/workflows`: template injection, secrets that reach untrusted code, cache poisoning, unpinned actions |

The `trivy` and `workflow-audit` jobs download a release binary and check its SHA-256
against a hash in `ci.yml`. To update either tool, verify the new release first (the
cosign signature of Trivy's checksums file, or `gh attestation verify` for zizmor). Then
change the version and the hash together.

## Scheduled acceptance tests

`.github/workflows/acceptance.yml` runs the live acceptance tests. It runs every Monday at
06:17 UTC. You can also start it manually: **Actions > acceptance > Run workflow**.

> **SECURITY:** The acceptance tests create and delete objects. Use a NodePing account
> that contains only the test fixtures. Never use a production account.

### Set up the environment

Do this one time.

1. Go to **Settings > Environments > New environment**.
2. Name the environment `acceptance`.
3. Set **Deployment branches and tags** to selected branches. Select `main` only. Pull
   requests, also from forks, then cannot read the secrets.
4. Add these environment secrets:

   | Secret | Value |
   |---|---|
   | `NODEPING_API_TOKEN` | A token for the test account only |
   | `NODEPING_TEST_CHECK_ID` | The fixture check ID. See `CONTRIBUTING.md`. |
   | `NODEPING_TEST_CONTACT_ID` | The fixture contact address ID. See `CONTRIBUTING.md`. |
   | `NODEPING_TEST_CUSTOMER_ID` | The test account ID. See `CONTRIBUTING.md`. |

A test skips when its fixture is not set.

### Subaccount test

The schedule does not run the subaccount lifecycle test (`make test-acc-subaccount`). That
test needs a parent-account token. It creates and deletes real subaccounts. Run it manually,
and only against a parent account that contains nothing else.
