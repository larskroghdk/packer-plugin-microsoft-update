# Troubleshooting

## Get more detail

1. Set `log_level = "debug"` to see skipped and excluded updates, guest output,
   and heartbeats in the Packer output.
2. Run Packer with `PACKER_LOG=1` and `PACKER_LOG_PATH=packer.log`. The log always
   contains the debug messages, prefixed with `microsoft-update:`.
3. Set `report_path`. The report lists every update with its decision reason and
   every attempt with its HRESULT, and every error with its phase and class.
4. On the guest, read the Windows Update log (`Get-WindowsUpdateLog` on Windows 10
   and later, `%SystemRoot%\WindowsUpdate.log` on older releases) and
   `%SystemRoot%\Logs\CBS\CBS.log`.

Error messages include the HRESULT, its name, a description, and, for known
problems, a hint. For example:

```text
install failed: 0x800F0922 (CBS_E_INSTALLERS_FAILED): Processing advanced installers
and generic commands failed. Check free space on the System Reserved or EFI
partition and review CBS.log.
```

## The run ends with `unsupported`

The status reason names the cause.

| Reason mentions                                | Fix                                                                                                                                            |
| ---------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| `WUServer and UseWUServer`                     | Configure WSUS before the provisioner, or use `update_source = "default"`.                                                                     |
| `DisableWindowsUpdateAccess`                   | Remove the policy in the image, or use `wsus` or `offline`.                                                                                    |
| `DoNotConnectToWindowsUpdateInternetLocations` | Remove the policy, or use `wsus` or `offline`.                                                                                                 |
| `wuauserv) is disabled`                        | Set the Windows Update service to manual start before the provisioner.                                                                         |
| `AddService2`                                  | The Windows Update Agent is too old to register Microsoft Update. Use `windows-update`, or update the agent first.                             |
| `0x80244019` or `0x80072F8F`                   | The guest cannot reach a supported update endpoint. On legacy systems, install the SHA-2 and servicing stack prerequisites and enable TLS 1.2. |

## The run ends with `partial`

Read the `status_reason` and the warnings at the end of the output.

- **`reached max_update_passes`** or **`reached max_reboots`**: the guest needed
  more passes or restarts. Raise the limits. A freshly installed older release can
  need 20 or more restarts.
- **Updates in state `remaining`**: they were selected but still offered at the
  end. Usually the limits were reached first.
- **Updates in state `failed`**: see the attempts in the report. An update that
  installs "successfully" but is offered again in every pass is marked failed after
  `max_install_attempts`; exclude it with `exclude_kbs` and report it to Microsoft.

## Common HRESULTs

| HRESULT                                  | Name                                | What to do                                                                                                            |
| ---------------------------------------- | ----------------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| `0x80070005`                             | `E_ACCESSDENIED`                    | Check `username` and `password`; the account must be an administrator.                                                |
| `0x80070070`                             | `ERROR_DISK_FULL`                   | Give the build VM a larger system disk. Cumulative updates need several GB.                                           |
| `0x80070422`                             | `ERROR_SERVICE_DISABLED`            | Enable `wuauserv` and BITS.                                                                                           |
| `0x80072EE2`, `0x80072EFD`, `0x80072EE7` | Network errors                      | Check DNS, proxy (`netsh winhttp show proxy`), and firewall. These are retried.                                       |
| `0x80244017`, `0x80244018`               | Proxy denied                        | The `SYSTEM` account cannot authenticate to the proxy. Use a transparent proxy or allow the Windows Update endpoints. |
| `0x80240032`                             | `WU_E_INVALID_CRITERIA`             | Fix `search_criteria`.                                                                                                |
| `0x800F081F`                             | `CBS_E_SOURCE_MISSING`              | Repair the component store: `DISM /Online /Cleanup-Image /RestoreHealth`.                                             |
| `0x800F0922`                             | `CBS_E_INSTALLERS_FAILED`           | Free space on the System Reserved or EFI partition; read `CBS.log`.                                                   |
| `0x80073712`                             | `ERROR_SXS_COMPONENT_STORE_CORRUPT` | Repair the component store, or rebuild from fresh media.                                                              |

The full list is in [`internal/wuerror/hresult.go`](../internal/wuerror/hresult.go).

## The provisioner seems to hang

Phases are bounded by the timeouts, and long phases are normal: a cumulative update
can take more than an hour to install on a slow disk. With `log_level = "debug"`
the provisioner prints a heartbeat with CPU and memory load every minute while a
guest action runs, which shows whether the guest is still working.

If an update can request user input, the output contains a warning naming it.
Such an update can wait forever for input that never comes; exclude it.

## A restart times out

- Check that the guest really restarts: watch the VM console.
- Raise `reboot_timeout` for slow hosts or for feature updates.
- If the builder or a script changes the guest clock or restores a snapshot, the
  boot time comparison can fail. See [Restart handling](reboot-handling.md).

## Leftover scheduled tasks

Tasks named `packer-microsoft-update-*` are left behind only when a build is killed
abruptly. The next run removes them. To remove them by hand:

```powershell
Get-ScheduledTask -TaskName 'packer-microsoft-update-*' | Unregister-ScheduledTask -Confirm:$false
```

## Legacy systems

Windows 7 SP1, Windows Server 2008 R2, Windows 8.1, Windows Server 2012, and
Windows Server 2012 R2 are past the end of support.

- Updates published after the end of support install only with a valid Extended
  Security Updates (ESU) license. The plugin neither checks nor bypasses ESU.
- Windows 7 SP1 and Windows Server 2008 R2 need the servicing stack update
  KB4490628 and the SHA-2 code signing update KB4474419 before Windows Update can
  install current packages. The plugin reports them as missing prerequisites.
  Install them in the base image, or with `offline_package` blocks as in the
  [`windows-server-2012` example](../examples/windows-server-2012.pkr.hcl).
- Enable TLS 1.2 for WinHTTP on those systems, or searches fail with `0x80072F8F`.
- Expect long searches. The examples raise `search_timeout`, `install_timeout`, and
  `reboot_timeout` for these releases.
