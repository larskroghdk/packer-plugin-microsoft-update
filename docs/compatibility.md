# Compatibility

## Windows guests

The plugin is built for the releases below. **No release has been tested in a live
build yet.** A release moves to "Tested" only after a live Hyper-V run of its
integration scenario in [`test/integration/scenarios.psd1`](../test/integration/scenarios.psd1)
ends with the expected status; the date and plugin version are then recorded here.

| Windows release                                    | Integration scenario                   | Example                 | Status        |
| -------------------------------------------------- | -------------------------------------- | ----------------------- | ------------- |
| Windows 11 Enterprise 25H2                         | `windows-11-microsoft-update`          | `microsoft-update`      | Not tested    |
| Windows 11 Enterprise 25H2, already updated        | `windows-11-fully-updated`             | `basic`                 | Not tested    |
| Windows 10 Enterprise LTSC 2021                    | `windows-10-ltsc-basic`                | `basic`                 | Not tested    |
| Windows Server 2025 Datacenter, Server Core        | `windows-server-2025-core`             | `windows-server-modern` | Not tested    |
| Windows Server 2022 Datacenter, Desktop Experience | `windows-server-2022-desktop`          | `all-updates`           | Not tested    |
| Windows Server 2019 Datacenter                     | `windows-server-2019-json-report`      | `json-reporting`        | Not tested    |
| Windows Server 2016 Datacenter                     | `windows-server-2016-upstream-filters` | `upstream-filters`      | Not tested    |
| Windows Server 2012 R2                             | `windows-server-2012-r2`               | `windows-server-2012`   | Not tested    |
| Windows Server 2012                                | `windows-server-2012`                  | `windows-server-2012`   | Not tested    |
| Windows 8.1                                        | `windows-8-1`                          | `basic`                 | Not tested    |
| Windows 7 SP1                                      | `windows-7-sp1`                        | `basic`                 | Not tested    |
| Windows Server 2022, WSUS source                   | `wsus`                                 | `wsus`                  | Not tested    |
| Windows Server 2022, offline packages              | `offline-packages`                     | `offline`               | Not tested    |
| Windows 11, KB exclusion                           | `kb-exclusion`                         | `kb-exclude`            | Not tested    |
| Windows Insider Preview, client and server         | None                                   | None                    | Not supported |

Releases from Windows 8.1 and Windows Server 2012 R2 back are past the end of
support. They receive updates published after that date only with an Extended
Security Updates license. See [Troubleshooting](troubleshooting.md#legacy-systems).

## Build machine

| Component        | Requirement                                                                                                           |
| ---------------- | --------------------------------------------------------------------------------------------------------------------- |
| Packer           | 1.11.0 or later (plugin API `x5.0`). Development uses 1.16.2.                                                         |
| Operating system | Any platform Packer runs on. Release binaries exist for Linux, macOS, Windows, and FreeBSD on amd64 and arm64.        |
| Builder          | Any builder with a WinRM communicator to a Windows guest. Only Hyper-V is used in the examples and integration tests. |

## Guest requirements

- Windows PowerShell 2.0 or later.
- Task Scheduler service running.
- Windows Update (`wuauserv`) not disabled, except for `update_source = "offline"`.
- An administrator account for the communicator.
- 10 GB or more free space on the system drive is recommended; less is reported as
  a missing prerequisite.
- Windows 7 SP1: KB4490628 and KB4474419, and TLS 1.2 enabled for WinHTTP.

## How a release becomes "Tested"

1. Prepare the base VM named in the scenario.
2. Run the scenario:
   `pwsh ./test/integration/Invoke-IntegrationTest.ps1 -Scenario <name>`.
3. Check the result in `test/integration/results/`.
4. Update the row with "Tested", the date, the plugin version, and the build
   number the guest ended at.
