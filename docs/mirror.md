# Provider Mirror

This provider is not published to a public registry. It is distributed through an ABT
private **network mirror** on S3 behind CloudFront.

This is not a preference. `goreleaser` cannot publish to Bitbucket (it targets
GitHub/GitLab/Gitea only), and `registry.opentofu.org` accepts public GitHub repositories
only — there is no private publishing path. The mirror is the distribution mechanism, so
until it exists the only way to install the provider is `make install` to a local
filesystem mirror.

## Status

**The pipeline is written; the AWS infrastructure is not.** Everything in
`bitbucket-pipelines.yml` and `tools/mirror-index/` is complete and tested. The steps below
are what remains, and none of it is in code.

## What the mirror has to serve

The [network mirror protocol](https://opentofu.org/docs/internals/provider-network-mirror-protocol/)
is two static JSON documents plus the archives. There is no discovery step — the URL is
given directly in the CLI config, must be `https:`, and must end in a trailing slash.

```
<base>/atlanticbt.com/abt/nodeping/index.json
    {"versions": {"1.0.0": {}, "1.1.0": {}}}

<base>/atlanticbt.com/abt/nodeping/1.0.0.json
    {"archives": {"linux_amd64": {"url": "...zip", "hashes": ["h1:..."]}}}

<base>/atlanticbt.com/abt/nodeping/terraform-provider-nodeping_1.0.0_linux_amd64.zip
```

Uploading the zips alone does nothing — without the index documents OpenTofu cannot
enumerate or resolve any version.

`hashes` is optional in the protocol, and **omitting it means archives install with no
verification whatsoever**. `tools/mirror-index` always emits `h1:` hashes, computed with the
same dirhash algorithm OpenTofu uses, so a tampered or truncated archive fails the install.
Those hashes are what land in a consumer's `.terraform.lock.hcl`.

Archive URLs are emitted relative to the version document, so the mirror can be rehosted or
moved behind a different CDN without regenerating anything.

## Remaining work

### 1. S3 bucket

Private, no public access. CloudFront reaches it through an Origin Access Control; nothing
else should.

- Block all public access
- Versioning on — a bad publish should be recoverable
- SSE-S3 is sufficient; the artifacts are not secret, only non-public

### 2. CloudFront distribution

The protocol requires `https:`, which is the main reason for the CDN.

- Origin: the S3 bucket via OAC (not a public website endpoint)
- Viewer protocol policy: redirect HTTP to HTTPS
- Restrict access to the ABT network or require a signed cookie if the artifacts should not
  be world-readable. Note that credentials configured for a network mirror are sent on the
  **JSON endpoints only, not on the `.zip` fetch** — so if the archives themselves must be
  protected, use signed URLs rather than relying on mirror credentials.
- Cache `index.json` and `<version>.json` briefly (60s, set by the pipeline via
  `Cache-Control`). The archives are immutable and can cache indefinitely.

### 3. Bitbucket OIDC to an AWS role

No AWS access keys as repository variables. The pipeline step declares `oidc: true` and
exchanges `$BITBUCKET_STEP_OIDC_TOKEN` for temporary credentials.

Get the workspace OIDC issuer and audience from **Repository settings → OpenID Connect**,
then create the identity provider and role:

```
Provider URL: https://api.bitbucket.org/2.0/workspaces/atlanticbt/pipelines-config/identity/oidc
Audience:     ari:cloud:bitbucket::workspace/<workspace-uuid>
```

Trust policy, scoped to this repository so any other repo in the workspace cannot assume it:

```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": { "Federated": "arn:aws:iam::<ACCOUNT_ID>:oidc-provider/api.bitbucket.org/2.0/workspaces/atlanticbt/pipelines-config/identity/oidc" },
    "Action": "sts:AssumeRoleWithWebIdentity",
    "Condition": {
      "StringEquals": {
        "api.bitbucket.org/2.0/workspaces/atlanticbt/pipelines-config/identity/oidc:aud": "ari:cloud:bitbucket::workspace/<WORKSPACE_UUID>"
      },
      "StringLike": {
        "api.bitbucket.org/2.0/workspaces/atlanticbt/pipelines-config/identity/oidc:sub": "{<REPOSITORY_UUID>}:*"
      }
    }
  }]
}
```

The `sub` condition is what stops another repository in the workspace from publishing to
the mirror. Do not omit it.

Permission policy — scoped to the one prefix, no bucket-wide write:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:PutObject", "s3:GetObject"],
      "Resource": "arn:aws:s3:::<BUCKET>/atlanticbt.com/abt/nodeping/*"
    },
    {
      "Effect": "Allow",
      "Action": "cloudfront:CreateInvalidation",
      "Resource": "arn:aws:cloudfront::<ACCOUNT_ID>:distribution/<DISTRIBUTION_ID>"
    }
  ]
}
```

### 4. Repository variables

| Variable | Required | Value |
|---|---|---|
| `PROVIDER_MIRROR_BUCKET` | yes | S3 bucket name |
| `PROVIDER_MIRROR_ROLE_ARN` | yes | Role ARN from step 3 |
| `PROVIDER_MIRROR_REGION` | no | Defaults to `us-east-1` |
| `PROVIDER_MIRROR_CF_DISTRIBUTION_ID` | no | Skips CDN invalidation if unset |

None are secrets. No GPG variables are needed — see below.

### 5. First publish

Tag a release and run the manual `Publish to Provider Mirror` step:

```bash
git tag v1.0.0 && git push origin v1.0.0
```

Then verify from a clean machine that resolution actually works, which is the only
meaningful test:

```bash
tofu init
tofu providers lock \
  -net-mirror=https://<distribution>/ \
  -platform=linux_amd64 -platform=darwin_arm64 \
  atlanticbt.com/abt/nodeping
```

A successful `providers lock` writing `h1:` entries for both platforms means the index
documents, the archives, and the hashes all line up.

## Consumer configuration

```hcl
provider_installation {
  network_mirror {
    url     = "https://<distribution>/"
    include = ["atlanticbt.com/*/*"]
  }
  direct {
    exclude = ["atlanticbt.com/*/*"]
  }
}
```

The `exclude` on `direct` is **required**. OpenTofu tries every matching installation method
and selects the newest version across all of them, so without it a resolution attempt still
reaches the public registry.

Note that `*/*` means `registry.opentofu.org/*/*` — three segments are needed to match an
arbitrary host, which is why the patterns above are written the way they are.

## Why there is no GPG signing

OpenTofu enforces GPG validation only for providers served from a self-hosted **origin
registry**, and there it is unconditional with no escape hatch
(`OPENTOFU_ENFORCE_GPG_VALIDATION=false` applies to the public registry only). Filesystem,
network, and OCI mirrors require no signature, no `SHA256SUMS`, and no
`terraform-registry-manifest.json`.

We publish through a network mirror, so a signature would be generated, stored, and never
checked by anything — while adding key generation, rotation, and a `GPG_PASSPHRASE`
repository variable to the operational surface. Integrity comes from the `h1:` hashes
instead, which OpenTofu does verify and does record in the consumer lock file.

If this ever moves to an origin registry, signing becomes mandatory. Restore a `signs:`
block in `.goreleaser.yml` with `--local-user` pinned to an explicit fingerprint — without
it, `gpg` signs with whatever default secret key is in the runner keyring and will silently
use the wrong one if more than one is ever imported.

## Alternative: filesystem mirror

For a single developer, no infrastructure is needed at all:

```bash
make install   # writes to ~/.local/share/opentofu/plugins/
```

See [development.md](development.md) for the matching `~/.tofurc` block. This is how to test
a local build, and it is a reasonable stopgap while the S3 mirror is being set up.
