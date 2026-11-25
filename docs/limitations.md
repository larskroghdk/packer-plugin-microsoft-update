# 🚧 Limitations

## ⛔ Not supported

- **Windows Insider Preview builds**, client and server. The plugin is not built or
  tested for them, and the compatibility matrix lists them as not supported. The
  report records the Insider branch when one is configured, nothing more.
- **Windows releases older than Windows 7 SP1 and Windows Server 2008 R2.** The
  guest scripts need Windows PowerShell 2.0 and a Windows Update Agent with the
  current API.
- **Non-Windows guests.**
- **Proxies that require user authentication** for the `SYSTEM` account.
- **Bypassing policy or licensing.** The plugin never changes update policies,
  services, or Extended Security Updates licensing. A blocked source ends the run
  with status `unsupported`.
- **Configuring WSUS.** `update_source = "wsus"` uses an existing configuration;
  set it with Group Policy or an earlier provisioner.
- **Hiding or declining updates** on the guest or on WSUS. Excluded updates are
  only left uninstalled.
- **Windows Update for Business deferrals and Delivery Optimization settings.**
  The Windows Update Agent applies them as configured; the plugin does not change them.

## ⚠️ Behavior to be aware of

- **Microsoft Update registration persists.** With `update_source =
  "microsoft-update"`, the image keeps the Microsoft Update service registered.
- **Include lists bypass the other rules.** With `include_kbs`,
  `include_update_ids`, or `include_titles`, the profile, classifications, products,
  and `include_*_updates` settings are ignored.
- **`filters` runs PowerShell on the guest.** The expressions are evaluated as
  script blocks in the scheduled task, with the rights of `username`. Treat them
  like any other script in the template.
- **Feature updates replace the operating system.** With
  `include_feature_updates = true`, a build can move the guest to a newer Windows
  release, which takes long and needs a large disk.
- **Updates that request user input** can stop responding. The plugin warns about
  them but does not skip them.
- **Restart detection uses the boot time.** Changing the guest clock or restoring a
  snapshot while the provisioner runs can make a restart look incomplete.
- **One provisioner per guest at a time.** Do not run two `microsoft-update`
  provisioners against the same guest in parallel; the Windows Update Agent
  serializes installations and the runs would block each other.

## ❔ Not tested

- The SSH communicator. The plugin only uses the generic communicator interface,
  but every test so far uses WinRM.
- ARM64 guests.
- Builders other than Hyper-V.

See the [compatibility matrix](compatibility.md) for the test status of each
Windows release.
