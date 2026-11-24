# Restart handling

The provisioner restarts the guest itself and waits for it. You do not need a
`windows-restart` provisioner before or after it.

## When the provisioner restarts

- **Before the first pass**, when a restart is already pending.
- **After Microsoft Update registration**, when the registration completes only
  after a restart.
- **After offline packages**, when a package or Windows reports that a restart is needed.
- **After an update pass**, when an installed update needs a restart, the installer
  reports that a restart is needed before it can install, or Windows reports a
  pending restart.
- **After a restart**, when Windows still reports a pending restart once servicing
  is idle. Some cumulative and servicing stack updates need two or three restarts.

Each restart counts against `max_reboots`. When a restart is needed and the limit
is reached, the run ends with status `partial`.

## Pending restart detection

A restart is pending when any of these is true:

| Reason                                       | Source                                                                                    |
| -------------------------------------------- | ----------------------------------------------------------------------------------------- |
| `windows-update-agent`                       | `Microsoft.Update.SystemInfo.RebootRequired`                                              |
| `component-based-servicing`                  | `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending`  |
| `component-based-servicing-packages-pending` | Subkeys under `...\Component Based Servicing\PackagesPending`                             |
| `windows-update-auto-update`                 | `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired` |
| `update-exe-volatile`                        | `HKLM\SOFTWARE\Microsoft\Updates\UpdateExeVolatile` is not `0`                            |

`PendingFileRenameOperations` is recorded as informational only. Many installers
leave entries there, and a restart does not always clear them, so treating them as
a pending restart would cause restart loops.

## How a restart works

1. The provisioner records the guest's last boot time.
2. It runs `shutdown.exe /r /f /t 5 /d p:2:17`. The reason code marks the restart
   as planned, for an operating system hotfix.
3. Every 10 seconds it asks the guest for its last boot time. The restart is
   complete when the boot time differs from the recorded one. A lost connection
   while the guest goes down is expected and not an error.
4. If the boot time does not change within `reboot_timeout`, the restart fails.
5. After the restart the provisioner waits until no `TiWorker.exe` process
   (Windows Modules Installer Worker) has run for 15 seconds, or until
   `servicing_timeout` expires. If servicing is still busy at the timeout, it
   prints a warning and continues.
6. It checks for a pending restart again and restarts once more if needed.

Comparing boot times, rather than waiting for the communicator to reconnect,
avoids mistaking a guest that has not gone down yet for a guest that is back up.

## Unexpected restarts

Windows sometimes restarts on its own while updates install. When a guest
operation fails because the connection is lost or a phase times out, the
provisioner compares the boot time before it retries. If the guest restarted, the
report records an unexpected restart event, the restart counts against
`max_reboots`, and the operation is retried.

## Interaction with Packer

The restart happens inside the provisioner, through the communicator Packer
already uses. The communicator reconnects on the next command, so later
provisioners run against the restarted guest without extra steps.

The guest's boot time comes from `Win32_OperatingSystem.LastBootUpTime`. A
builder that restores a snapshot or changes the guest clock during the build can
confuse the comparison; the provisioner then waits until `reboot_timeout`.
