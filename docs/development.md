# Development

## Prerequisites

- Go, at the version in [`go.mod`](../go.mod).
- PowerShell 7 with Pester 5.7.1 and PSScriptAnalyzer 1.24.0, for the guest script
  tests and linting.
- Packer 1.11.0 or later, to run the examples.
- GNU Make and a POSIX shell. On Windows, Git Bash works.
- For live tests: a Windows host with Hyper-V and prepared base VMs.

Install the Go tools at the pinned versions:

```sh
make tools
```

## Repository layout

| Path                  | Content                                                                             |
| --------------------- | ----------------------------------------------------------------------------------- |
| `main.go`             | Plugin entry point.                                                                 |
| `provisioner/`        | Configuration, validation, and the Packer provisioner.                              |
| `internal/engine/`    | Update loop, restarts, retries, final status.                                       |
| `internal/guest/`     | Guest client, wire protocol, and the embedded guest scripts in `scripts/`.          |
| `internal/selection/` | Update selection rules.                                                             |
| `internal/report/`    | Report model and JSON Schema.                                                       |
| `internal/wuerror/`   | HRESULT table and classification.                                                   |
| `version/`            | Plugin version.                                                                     |
| `docs/`               | Documentation. `docs/integration/` is rendered for the Packer integration registry. |
| `examples/`           | Example templates for Hyper-V and `Invoke-Example.ps1`.                             |
| `test/guest/`         | Pester tests for the guest scripts.                                                 |
| `test/integration/`   | Live Hyper-V scenarios.                                                             |

See [Architecture](architecture.md) for how the pieces fit together.

## Build and test

| Command                 | Does                                                                                            |
| ----------------------- | ----------------------------------------------------------------------------------------------- |
| `make build`            | Builds `packer-plugin-microsoft-update` in the repository root.                                 |
| `make install`          | Builds and installs the plugin into Packer's plugin directory.                                  |
| `make test`             | Runs the Go tests and the Pester tests.                                                         |
| `make test-go`          | Runs the Go tests.                                                                              |
| `make test-powershell`  | Runs the Pester tests in `test/guest/`.                                                         |
| `make lint`             | Runs golangci-lint and PSScriptAnalyzer.                                                        |
| `make vulncheck`        | Runs govulncheck.                                                                               |
| `make generate`         | Regenerates `provisioner/config.hcl2spec.go` and renders `docs/integration/` into `.web-docs/`. |
| `make release-snapshot` | Builds release archives in `dist/` without publishing.                                          |

CI runs formatting, generated-file, lint, test, vulnerability, cross-build, and
PowerShell jobs on every pull request. Run `make generate` and commit the result
whenever you change `provisioner/config.go` or `docs/integration/`; CI fails when
the generated files differ.

## Code style

- The Packer configuration reference is maintained by hand in
  [`docs/integration/provisioners/microsoft-update.mdx`](integration/provisioners/microsoft-update.mdx),
  because `packer-sdc` cannot generate it from this configuration. Update it with every
  configuration change.
- PowerShell uses Stroustrup braces: `}` on its own line, then `elseif`, `else`,
  `catch`, or `finally` on the next line.
- The guest scripts must run on Windows PowerShell 2.0.
  `test/guest/Compatibility.Tests.ps1` rejects newer cmdlets, type accelerators,
  operators, and methods.

## Changing the guest scripts

The scripts in `internal/guest/scripts/` are embedded into the plugin binary at
build time. After changing them:

1. Run `make test-powershell`. The tests parse the scripts, load their functions,
   and run actions against test doubles.
2. Run `make test-go`. The client tests check the wire protocol against the records
   the scripts write.
3. Rebuild the plugin; the change takes effect only in a new binary.

A new record type needs a change in both `microsoft-update.ps1` and
`internal/guest/client.go`, and the report schema when it adds report fields.

## Running the examples

The examples clone a prepared Hyper-V VM with WinRM enabled. `Invoke-Example.ps1`
runs `packer init`, installs a local plugin build when `-PluginPath` is given, and
asks for the WinRM credential when it builds, unless `-Credential` or
`PKR_VAR_winrm_password` is set. Validation needs no password; `winrm_password`
defaults to an empty string.

Validation needs no VM. It validates the example against a `null` source:

```powershell
./examples/Invoke-Example.ps1 -Example basic -Action Validate -PluginPath ./packer-plugin-microsoft-update.exe
```

A build needs an elevated PowerShell session, or membership in Hyper-V
Administrators:

```powershell
./examples/Invoke-Example.ps1 -Example microsoft-update -BaseVmName mu-base-windows-11
```

Reports are written to `examples/artifacts/`, Packer logs to `examples/logs/`.

### With an isolated plugin directory

`test/Invoke-LocalPluginExample.ps1` builds the plugin from the working tree and
runs an example with `PACKER_CONFIG_DIR` and `PACKER_PLUGIN_PATH` pointing to
`dist/test/`, so the local build never replaces a plugin installed in your Packer
plugin directory. It takes the same `-Example`, `-BaseVmName`, `-SwitchName`,
`-Credential`, and `-VariableFile` parameters as `Invoke-Example.ps1`, and validates
by default:

```powershell
./test/Invoke-LocalPluginExample.ps1 -Example basic
./test/Invoke-LocalPluginExample.ps1 -Example microsoft-update -Action Build -BaseVmName mu-base-windows-11
```

The Packer log is written to `dist/test/`.

## Integration tests

`test/integration/Invoke-IntegrationTest.ps1` builds the plugin, runs the selected
scenarios from `scenarios.psd1` against their base VMs, and checks each report
for the expected status, source, and limits. Results go to
`test/integration/results/`.

```powershell
./test/integration/Invoke-IntegrationTest.ps1 -Scenario windows-11-microsoft-update
```

The `Hyper-V integration` workflow runs the same script on a self-hosted runner
labeled `windows` and `hyperv`, when started manually.

## Releasing

1. Update `Version` in [`version/version.go`](../version/version.go) and the
   [changelog](../CHANGELOG.md).
2. Merge to `main`.
3. Tag the commit `v<version>` and push the tag.

The release workflow checks that the tag matches `version/version.go`, then
GoReleaser builds the archives, the SBOMs, the signed checksum file, and
`docs.zip`, and publishes a GitHub release.
