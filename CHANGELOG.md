# Changelog

All notable changes to this project are documented in this file. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project
follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - Unreleased

First release of `packer-plugin-microsoft-update`, a rewrite of
[rgl/packer-plugin-windows-update](https://github.com/rgl/packer-plugin-windows-update).

### Added

- `microsoft-update` provisioner with the update sources `default`,
  `windows-update`, `microsoft-update`, `wsus`, and `offline`.
- Registration of the Microsoft Update service when it is not registered.
- Offline `.msu` and `.cab` packages with SHA-256 and Microsoft Authenticode
  signature checks, usable as prerequisites before online updates.
- Declarative selection: `profile`, `classifications`, `products`,
  `include_*_updates`, include and exclude lists for KB numbers, update IDs, and
  titles.
- Limits on passes, restarts, updates per pass, and install attempts.
- Per-phase timeouts for search, download, install, restart, and servicing.
- Retries with exponential backoff for transient errors, and detection of
  unexpected restarts.
- Pending restart detection from five sources, restarts verified by boot time, and
  a wait for Windows servicing after each restart.
- Run status (`updated`, `partial`, `failed`, `unsupported`, `cancelled`) and
  `fail_on_error`.
- JSON report with a published JSON Schema (`report_path`).
- HRESULT names, descriptions, and hints in error messages.
- Detection of update policies, legacy prerequisites, low disk space, and releases
  past the end of support.
- Hyper-V examples, Pester tests for the guest scripts, and live Hyper-V
  integration scenarios.
- Signed releases with SBOMs and build provenance attestations.

### Changed from windows-update

- The provisioner type is `microsoft-update` and the plugin source is
  `github.com/larskroghdk/microsoft-update`.
- `update_limit`, `update_max_retries`, and `restart_timeout` are aliases of
  `max_updates_per_pass`, `max_retries`, and `reboot_timeout`.
- The default restart timeout is `60m` instead of `4h`.
- The build fails when selected updates fail or remain, unless
  `fail_on_error = false`.
- `filters` cannot be combined with the declarative selection settings, except the
  `exclude_*` lists.

[Unreleased]: https://github.com/larskroghdk/packer-plugin-microsoft-update/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/larskroghdk/packer-plugin-microsoft-update/releases/tag/v0.1.0
