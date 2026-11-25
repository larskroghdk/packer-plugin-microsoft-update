---
name: release-manager
description: Use when preparing, tagging, or troubleshooting a release of packer-plugin-microsoft-update, bumping version/version.go, editing .goreleaser.yml or the release workflow, or when a release build, signature, or tag check fails.
---

# Release Manager

## Overview

A release is a `v<version>` tag on `main`. The release workflow builds it with
GoReleaser, signs the checksum file with keyless cosign, attaches SBOMs and build
provenance, and publishes `docs.zip` for the Packer registry. The tag must match
`version/version.go`, or the workflow stops.

## Steps

1. Confirm `main` is green in CI and that no blocking review finding is open.
2. Pick the version with Semantic Versioning: breaking configuration or report
   changes bump the minor version while the major version is `0`.
3. Set `Version` in `version/version.go`. Keep `VersionPrerelease` empty for a release.
4. In `CHANGELOG.md`, rename "Unreleased" to `[<version>] - <YYYY-MM-DD>`, add a
   new empty "Unreleased" section, and update the compare links.
5. Update the `version` constraints in `README.md`,
   `docs/integration/README.md`, the `.mdx`, and `examples/packer.pkr.hcl` when the
   minimum version changes.
6. Bump the `SECURITY.md` supported-versions table for a new minor version.
7. Dry run locally:

   ```sh
   make release-snapshot
   ```

   Check `dist/` for archives for linux, darwin, windows, freebsd on amd64 and
   arm64, the `_SHA256SUMS` file, and the SBOMs.
8. Merge the release commit, then tag and push:

   ```sh
   git tag -a v<version> -m "v<version>"
   git push origin v<version>
   ```

9. Watch the release workflow. Afterwards, verify the artifacts as described in
   `docs/security.md`.

## Quick Reference

| Failure                                     | Cause                                                       |
| ------------------------------------------- | ----------------------------------------------------------- |
| "Tag ... does not match version/version.go" | Tag and `Version` differ; fix the file, do not move the tag |
| `API_VERSION` empty in binary names         | `go run . describe` failed; build the plugin locally first  |
| cosign fails                                | Workflow lacks `id-token: write`                            |
| `docs.zip` missing                          | `make generate` did not produce `.web-docs/`                |

## Common Mistakes

- Tagging before the release commit is merged to `main`.
- Deleting and re-pushing a published tag; release a new patch version instead.
- Marking compatibility rows "Tested" for the release without live runs.
