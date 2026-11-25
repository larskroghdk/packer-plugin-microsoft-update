# 📊 Reporting

Set `report_path` to write a JSON report on the machine that runs Packer. The
report is written for every outcome, including failures and cancelled builds, so a
pipeline can archive it and inspect it after the build. The only exception is a
run that cannot start a command on the guest at all; the build error then
explains why.

```hcl
provisioner "microsoft-update" {
  report_path = "artifacts/updates-${source.name}.json"
}
```

The report is described by a JSON Schema (draft 2020-12) in
[`internal/report/schema.json`](../internal/report/schema.json). Every change to
the format changes `schema_version`, and the major version stays `1` until a
change removes or renames a field.

## 🧾 Top-level fields

| Field                        | Content                                                                                                                 |
| ---------------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| `schema_version`             | Report format version, currently `1.0.0`.                                                                               |
| `plugin`                     | Plugin name, version, and commit.                                                                                       |
| `status`                     | `updated`, `partial`, `failed`, `unsupported`, or `cancelled`.                                                          |
| `status_reason`              | One sentence that explains the status.                                                                                  |
| `started_at`, `completed_at` | Start and end of the run, in UTC.                                                                                       |
| `duration_seconds`           | Run time in seconds.                                                                                                    |
| `system`                     | Guest facts, or `null` when detection failed. See below.                                                                |
| `source`                     | Requested and effective update source, the Microsoft Update service ID, whether a policy manages updates, and warnings. |
| `settings`                   | The effective selection settings and limits.                                                                            |
| `passes`, `reboots`          | Number of passes and restarts.                                                                                          |
| `reboot_events`              | Each restart with its time, reason, and whether the provisioner started it (`expected`).                                |
| `summary`                    | Counts per update state.                                                                                                |
| `updates`                    | Every update the searches returned, in the order first seen.                                                            |
| `offline_packages`           | Each offline package with checksum, signature, signer, result, and exit code.                                           |
| `errors`                     | Errors with time, phase, class, HRESULT, and message.                                                                   |

`system` contains the caption, version, build and update build revision, display
version, edition, installation type, product type, architecture, UI language,
PowerShell, CLR, and Windows Update Agent versions, the Insider branch if any, the
WSUS and Windows Update access policies, whether Microsoft Update is registered,
the start mode of `wuauserv`, free space on the system drive, a lifecycle notice
for releases past the end of support, and missing prerequisites.

## 🏷️ Update states

| State            | Meaning                                                                                                            |
| ---------------- | ------------------------------------------------------------------------------------------------------------------ |
| `installed`      | Installed in a pass. The reason says which pass, and "with errors" when Windows reported `orcSucceededWithErrors`. |
| `failed`         | The download or installation failed, or the update was still offered after `max_install_attempts`.                 |
| `remaining`      | Selected, but still offered when the run ended.                                                                    |
| `skipped`        | Not selected by the selection settings.                                                                            |
| `excluded`       | Matched an `exclude_*` list.                                                                                       |
| `not-applicable` | Windows reported that the update does not apply.                                                                   |
| `superseded`     | No longer offered by the update source, usually because a newer update replaced it.                                |

`summary.not_applicable` counts both `not-applicable` and `superseded`.

Offline packages uploaded from the build machine are stored on the guest as
`<first 16 hex digits of the SHA-256>-<file name>`, so packages with the same file
name from different folders do not overwrite each other. Their `name` in the report
carries the same prefix.

Each update also lists its ID, revision, title, KB numbers, classification,
products, whether it is optional or a preview, the pass in which it was first
seen, and every attempt with phase, result code, HRESULT, HRESULT name, and
whether it required a restart.

## ❌ Error classes

| Class             | Meaning                                                       | Retried         |
| ----------------- | ------------------------------------------------------------- | --------------- |
| `transient`       | Temporary condition, such as a network error or a busy agent. | Yes             |
| `permanent`       | Retrying does not help, such as a corrupt component store.    | No              |
| `unsupported`     | The guest or a policy does not allow the work.                | No              |
| `reboot-required` | The operation needs a restart first.                          | After a restart |
| `not-applicable`  | The update does not apply.                                    | No              |

Unknown HRESULTs with the Windows Update facility (`0x8024xxxx`) are treated as
transient; other unknown codes as permanent.

## 🔗 Using the report in a pipeline

Fail a pipeline step when any update failed, with PowerShell:

```powershell
$report = Get-Content -Raw -Path artifacts/updates.json | ConvertFrom-Json
if ($report.summary.failed -gt 0) {
    $report.updates | Where-Object state -eq 'failed' | Format-Table title, reason
    exit 1
}
```

List the installed KB numbers with `jq`:

```sh
jq -r '.updates[] | select(.state == "installed") | .kbs[]' artifacts/updates.json
```
