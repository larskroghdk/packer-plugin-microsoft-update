# Agent instructions

Instructions for coding agents working in this repository. Read
[docs/architecture.md](docs/architecture.md) and
[docs/development.md](docs/development.md) before changing code.

## Project

A Packer provisioner plugin, `microsoft-update`, that installs Windows updates on a
guest through the Windows Update Agent API. Go code runs on the build machine;
the PowerShell scripts in `internal/guest/scripts/` are embedded into the binary
and run on the guest inside a scheduled task.

## Rules

- **Guest scripts must run on Windows PowerShell 2.0.** No cmdlets, type
  accelerators, operators, or methods newer than 2.0: no `ConvertTo-Json`,
  `Get-CimInstance`, `[pscustomobject]`, `::new()`, `-in`, `.Where()`, `$PSItem`,
  `-Raw`. `test/guest/Compatibility.Tests.ps1` enforces this.
- **PowerShell style:** Stroustrup braces (`}` on its own line, then `elseif`,
  `else`, `catch`, or `finally`), four-space indentation, aligned hashtable
  assignments, full cmdlet names. Follow `PSScriptAnalyzerSettings.psd1`.
- **The configuration reference is maintained by hand** in
  `docs/integration/provisioners/microsoft-update.mdx`. Every change to
  `provisioner/config.go` updates it in the same change, with the default and the
  validation rules.
- **Windows Insider Preview builds are out of scope.** Do not add Insider
  examples, integration scenarios, or tested claims.
- **The compatibility matrix lists a release as "Tested"** only after a live run of
  its integration scenario succeeded. Never mark a release tested from unit tests.
- **The plugin never changes update policies, service configuration, or licensing**
  on the guest. A blocked source ends the run with status `unsupported`.
- **Never put credentials** in templates, examples, tests, logs, or reports.

## Commands

| Task                             | Command                                                         |
| -------------------------------- | --------------------------------------------------------------- |
| Build                            | `make build`                                                    |
| Go tests                         | `make test-go`                                                  |
| Guest script tests               | `make test-powershell`                                          |
| Lint Go and PowerShell           | `make lint`                                                     |
| Regenerate HCL spec and web docs | `make generate`                                                 |
| Validate an example              | `./examples/Invoke-Example.ps1 -Example basic -Action Validate` |
| Validate with the local build    | `./test/Invoke-LocalPluginExample.ps1 -Example basic`           |

Run `gofmt`, the Go tests, and the Pester tests before you finish a change. Live
Hyper-V tests need prepared VMs and run only when the maintainer asks.

## Workflow

- Work on a feature branch; never commit to `main` directly.
- Write a failing test first for behavior changes, in `provisioner/`,
  `internal/*/`, or `test/guest/`.
- Every change gets an independent code review before it is committed. Resolve the
  findings and run the tests again after fixing them.
- Add an entry under "Unreleased" in `CHANGELOG.md` for user-visible changes.
