# 🌐 Update sources

The `update_source` setting chooses where the guest looks for updates. The
provisioner reads the update policies on the guest before it searches, and it never
changes them.

| `update_source`    | Windows Update Agent setting                              | Offers                                       |
| ------------------ | --------------------------------------------------------- | -------------------------------------------- |
| `default`          | `ServerSelection = ssDefault`                             | Whatever the guest is configured for         |
| `windows-update`   | `ServerSelection = ssWindowsUpdate`                       | Windows updates                              |
| `microsoft-update` | `ServerSelection = ssOthers`, Microsoft Update service ID | Windows and other Microsoft products         |
| `wsus`             | `ServerSelection = ssManagedServer`                       | Updates approved on the WSUS server          |
| `offline`          | None                                                      | Only the configured `offline_package` blocks |

## ⭐ default

The guest decides. The report shows the effective source:

- `wsus` when the `WUServer` policy is set and `UseWUServer` is `1`,
- `microsoft-update` when the Microsoft Update service is registered,
- `windows-update` otherwise.

Use `default` when the image is built inside a managed network and the source is
set by Group Policy.

## 🪟 windows-update

Searches Windows Update directly, even when WSUS is configured. In that case the
report and the output contain a warning, and the WSUS policy stays in place.

## 🏢 microsoft-update

Searches Microsoft Update, which also offers updates for Microsoft 365 Apps, SQL
Server, .NET, Visual Studio, and other Microsoft products.

If the Microsoft Update service (`7971f918-a847-4430-9279-4a52d1efe18d`) is not
registered, the provisioner registers it with
`IUpdateServiceManager2::AddService2`, allowing online and pending registration
and registering the service with Automatic Updates.
If the registration completes only after a restart, the provisioner restarts the
guest first. The registration is a persistent change: the finished image keeps
Microsoft Update as a registered service, which matches the "Receive updates for
other Microsoft products" setting.

Windows Update Agent versions without `AddService2` cannot register the service;
the run ends with status `unsupported`. Use `windows-update` on those systems.

## 🏭 wsus

Searches the WSUS server set by policy. The provisioner requires both the
`WUServer` value under `HKLM\SOFTWARE\Policies\Microsoft\Windows\WindowsUpdate` and
`UseWUServer = 1` under its `AU` key. Without them the run ends with status
`unsupported`.

The provisioner does not configure WSUS. Set the policies with Group Policy, in
the unattended answer file, or with an earlier provisioner. The
[`wsus` example](../examples/wsus.pkr.hcl) does it with a `powershell` provisioner.

Only updates approved on the WSUS server are offered. The selection settings then
narrow those down further.

## 💾 offline

Installs only the packages from `offline_package` blocks and searches no update
service. Use it for air-gapped builds, or to apply a specific servicing stack or
cumulative update from media.

`offline_package` blocks also work with the online sources. The packages are then
installed before the first search, which is how legacy systems get the servicing
stack and SHA-2 prerequisites they need before Windows Update works. See the
[`windows-server-2012` example](../examples/windows-server-2012.pkr.hcl); the same
`offline_package` blocks work for Windows 7 SP1.

For each package the guest:

1. checks that the file exists,
2. computes its SHA-256 checksum and compares it with the configured or uploaded checksum,
3. requires a valid Authenticode signature whose signer is `O=Microsoft Corporation`,
4. installs `.msu` files with `wusa.exe /quiet /norestart` and `.cab` files with
   `DISM.exe /Online /Add-Package /Quiet /NoRestart`.

A package that fails a check is `rejected` and not run. Exit codes are mapped as follows:

| Exit code                            | Result                        |
| ------------------------------------ | ----------------------------- |
| `0`                                  | `installed`                   |
| `3010`, `1641`                       | `installed`, restart required |
| `2359302` (`WU_S_ALREADY_INSTALLED`) | `already-installed`           |
| `0x80240017`, `0x800F081E`           | `not-applicable`              |
| Anything else                        | `failed`                      |

## 🚧 Policies that block a source

The provisioner stops with status `unsupported`, and changes nothing, when:

| Condition                                          | Blocks                               |
| -------------------------------------------------- | ------------------------------------ |
| `DisableWindowsUpdateAccess = 1`                   | `windows-update`, `microsoft-update` |
| `DoNotConnectToWindowsUpdateInternetLocations = 1` | `windows-update`, `microsoft-update` |
| `WUServer` or `UseWUServer` missing                | `wsus`                               |
| The `wuauserv` service is disabled                 | All sources except `offline`         |

A disabled BITS service and less than 10 GB of free space on the system drive are
reported as missing prerequisites, but do not stop the run.

## 📡 Network access

The guest, not the machine that runs Packer, connects to the update service. It
connects as the account the scheduled task runs as, `SYSTEM` by default, so a
proxy must be configured for WinHTTP (`netsh winhttp set proxy`) or be transparent.
Proxies that require user authentication return `0x80244017` and are not supported
for the `SYSTEM` account.
