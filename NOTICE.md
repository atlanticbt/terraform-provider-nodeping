<!--
doc: notice
standard: ASD-STE100 (about 80%)
last-refreshed: 2026-10-14
generated-by: doc-refresh skill
-->

# Provenance and Attribution

## Origin

This provider is an Atlantic BT fork of a third-party open-source project:

| Item | Value |
|---|---|
| Upstream | `github.com/phizzl/terraform-provider-nodeping` |
| Upstream author | Phillip Schleicher |
| Forked at | commit `8fdc0e1` ("feat: add full support for notification dependency (dep) property") |
| Upstream license | Mozilla Public License 2.0 |
| Fork date | 2026-08-27 |

**We did not write the original work, and we do not claim authorship of it.** Phillip
Schleicher wrote the first implementation: the plugin framework resource layout, the
NodePing API client, and the check and contact resources. Our fork starts from that work
and is different from it now.

We made the fork because we use this provider to manage production monitoring. We needed
control of its release schedule, its security and its test coverage. The fork is not a
criticism of the upstream project.

## License

The upstream project uses MPL-2.0, and this repository also uses MPL-2.0. The `LICENSE`
file is not changed. This repository publishes the complete source under MPL-2.0. This
includes each file that we changed from upstream.

MPL-2.0 is copyleft at the file level:

- Files from upstream, and changes to them, stay under MPL-2.0.
- Upstream copyright notices stay in the files that we did not write.

## Relationship to Upstream

This repository and upstream do not share git history. This repository starts with one
initial commit. That commit contains the forked source at the upstream commit named
above. The upstream history stays in the upstream repository.

Upstream is a public repository with one maintainer. We do not merge from it
automatically:

1. We review each upstream change as a diff.
2. We take it through the normal pull request process.
3. A change that adds a dependency, touches token handling or changes the release
   pipeline also needs a security review.
4. We record each adopted change here, with the upstream commit it came from.

### Adopted from Upstream Since the Fork

_None yet._
