# Provenance and Attribution

## Origin

This provider is an Atlantic BT fork of a third-party open-source project:

| | |
|---|---|
| Upstream | `github.com/phizzl/terraform-provider-nodeping` |
| Upstream author | Phillip Schleicher |
| Forked at | commit `8fdc0e1` ("feat: add full support for notification dependency (dep) property") |
| Upstream license | Mozilla Public License 2.0 |
| Fork date | 2026-08-27 |

**We did not author the original work and do not claim authorship of it.** The initial
implementation — the plugin-framework resource layout, the NodePing API client, and the
check and contact resources — is Phillip Schleicher's. Our fork begins from that work and
diverges from it.

The fork exists because we depend on this provider to manage production monitoring and
needed to control its release cadence, security posture and test coverage. It is not a
criticism of the upstream project.

## License

Upstream is MPL-2.0 and this repository remains MPL-2.0. The `LICENSE` file is
unmodified. The complete source, including every file modified from upstream, is
published in this repository under MPL-2.0. MPL-2.0 is file-level copyleft: files that
came from upstream, and changes to them, stay under MPL-2.0, and upstream copyright
notices in files we did not write are retained.

## Relationship to upstream

There is no shared git history. This repository begins at a single initial commit that
contains the forked source as of the commit named above; upstream's history remains in
the upstream repository.

Upstream is a single-maintainer public repository. We do not merge from it automatically.
Changes are reviewed as a diff and adopted through the normal PR path, and any commit that
adds a dependency, touches token handling, or changes the release pipeline additionally
requires a security review. Adopted upstream changes are recorded here with the upstream
commit they came from.

### Adopted from upstream since the fork

_None yet._
