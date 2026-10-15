# Releasing and repository settings

For maintainers. Releases are built and signed by GitHub Actions
(`.github/workflows/release.yml`) when a `v*` tag is pushed.

## Release signing key (one time)

The OpenTofu and Terraform registries verify each release by checking a detached
GPG signature of its `SHA256SUMS` file against a public key registered for the
`atlanticbt` namespace. Use a dedicated key for this, not a personal one.

The Terraform Registry accepts only RSA and DSA keys; the OpenTofu registry takes
any ASCII-armored PGP public key. An RSA 4096 key works for both:

```sh
gpg --quick-generate-key "Atlantic BT NodePing provider releases" rsa4096 sign 3y
gpg --list-secret-keys --keyid-format=long "Atlantic BT NodePing provider releases"
gpg --armor --export-secret-keys <KEY_ID> > release-key.asc   # for the Actions secret
gpg --armor --export <KEY_ID> > release-key.pub               # for the registries
```

In **Settings > Secrets and variables > Actions**, add:

| Secret | Value |
|---|---|
| `GPG_PRIVATE_KEY` | contents of `release-key.asc` |
| `PASSPHRASE` | the key's passphrase |

Then delete `release-key.asc` locally and keep the key and passphrase in the
password manager. Rotating the key means registering the new public key with both
registries before the next release.

## Cutting a release

1. Merge everything for the release to `main` and update `CHANGELOG.md`.
2. Tag with your own signing key (configure `user.signingkey` once):

   ```sh
   git tag -s v1.2.0 -m "v1.2.0"
   git push origin v1.2.0
   ```

3. The **Release** workflow runs the unit tests, builds the six platform archives,
   signs `SHA256SUMS` and publishes a GitHub release with the archives, the
   checksums, the signature and `terraform-provider-nodeping_<version>_manifest.json`.

Both registries pick up new GitHub releases on their own once the provider is
registered.

## Registering with the registries (one time, after going public)

**OpenTofu** (`registry.opentofu.org/atlanticbt/nodeping`): submissions go through
the issue forms in `opentofu/registry`, not pull requests or the API.

1. Make your membership of the `atlanticbt` organization public; the key form
   requires it.
2. Open **Submit new Provider Signing Key**: namespace `atlanticbt`, provider name
   `nodeping`, and the contents of `release-key.pub`.
3. Open **Submit new OpenTofu Provider**: repository
   `atlanticbt/terraform-provider-nodeping`.

**Terraform** (optional): sign in to registry.terraform.io with GitHub, add the
same public key under the `atlanticbt` namespace's signing keys, and publish the
repository.

## Going public

The organization is on GitHub Free, so branch rulesets and secret scanning are
only available once the repository is public. Before making it public, confirm
that its history holds only the cleaned initial commit. Then, in one sitting:

1. **Settings > General > Danger Zone > Change visibility**: public.
2. **Settings > Rules > Rulesets > New branch ruleset**, targeting the default
   branch:
   - Restrict deletions; block force pushes.
   - Require a pull request before merging: 1 approval, require review from Code
     Owners, dismiss stale approvals on new commits.
   - Require status checks to pass, and the branch to be up to date: `fmt-vet`,
     `lint`, `test`, `vulncheck`, `secret-scan`.
   - Leave the bypass list empty.
3. **Settings > Code security**: enable secret scanning and push protection,
   Dependabot alerts and Dependabot security updates, and private vulnerability
   reporting (which `SECURITY.md` points reporters to).
4. **Settings > Actions > General**: workflow permissions read-only by default.
5. Confirm `@atlanticbt/nodeping-maintainers` exists with write access, so
   `CODEOWNERS` resolves.

## CI status checks

`.github/workflows/ci.yml` runs on every pull request and on pushes to `main`. Its
job names are the required status checks:

| Check | What it runs |
|---|---|
| `fmt-vet` | `gofmt -l` and `go vet` |
| `lint` | golangci-lint v1.62.0 |
| `test` | unit tests with the race detector and coverage |
| `vulncheck` | govulncheck v1.8.0 |
| `secret-scan` | gitleaks over the commits the change adds |

Acceptance tests are not part of CI: they need a live NodePing account and run by
hand with `make test-acc`.
