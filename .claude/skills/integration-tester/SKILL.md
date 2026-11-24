---
name: integration-tester
description: Use when running live Hyper-V builds or integration scenarios for packer-plugin-microsoft-update, adding a scenario to test/integration/scenarios.psd1, interpreting integration results, or deciding whether a Windows release can be marked tested in docs/compatibility.md.
---

# Integration Tester

## Overview

Live tests build real Windows guests on Hyper-V with the examples in `examples/`.
They are the only evidence that a Windows release works. They need prepared base
VMs and media, take hours, and change nothing outside the cloned VMs. Run them only
when the maintainer asks and the base VMs exist.

## Before a run

- Confirm the base VM named by the scenario's `BaseVmName` exists (`Get-VM`) and has
  WinRM enabled for the credential you will use.
- Use an elevated session or membership in Hyper-V Administrators.
- Set the WinRM password through `PKR_VAR_winrm_password`; never write it to a
  file in the repository.
- Validate the example first; it needs no VM:

  ```powershell
  ./examples/Invoke-Example.ps1 -Example <example> -Action Validate
  ```

## Run

```powershell
./test/integration/Invoke-IntegrationTest.ps1 -Scenario <name>
```

`-Scenario all` runs every scenario. The script builds the plugin, runs each
example, and checks the report for `ExpectedStatus`, `ExpectedSource`,
`MinimumPasses`, and `ExpectedMaxReboots`. Results go to
`test/integration/results/`, Packer logs to `test/integration/results/logs/`,
reports to `examples/artifacts/`.

## Reading results

| Report status | Next step                                                                    |
| ------------- | ---------------------------------------------------------------------------- |
| `updated`     | Scenario passed if it is in `ExpectedStatus`.                                |
| `partial`     | Check `status_reason`: limits reached, or `failed` updates with HRESULTs.    |
| `failed`      | Read `errors[]` and the Packer log; see `docs/troubleshooting.md`.           |
| `unsupported` | A policy or prerequisite blocks the source; fix the base VM, not the plugin. |

## Recording a pass

Only a passing live run moves a row in `docs/compatibility.md` to "Tested". Record
the date, the plugin version, and the build number the guest ended at
(`system.build`.`system.update_build_revision` in the report).

## Adding a scenario

Add an entry to `test/integration/scenarios.psd1` with `Name`, `OperatingSystem`,
`BaseVmName`, `Example`, `ExpectedStatus`, and `ExpectedSource`, and a matching row
in `docs/compatibility.md` marked "Not tested". Never add Windows Insider scenarios.

## Common Mistakes

- Marking a release tested after a run that ended `partial`.
- Changing a base VM's update policies to make a scenario pass; the plugin
  respects policy by design.
- Running scenarios in parallel against the same base VM.
