# 🔒 Security

This page describes what the plugin does on the guest and on the build machine, so
you can judge its impact on your build environment. To report a vulnerability, see
[SECURITY.md](../SECURITY.md).

## 🛡️ Rights on the guest

The plugin runs every Windows Update Agent call in a scheduled task with the
highest run level. By default the task runs as `SYSTEM`. The communicator account
must be an administrator, because only an administrator can register such a task.

With `username` and `password`, the task runs as that account with a stored
password logon. Use this only when `SYSTEM` cannot be used, for example because a
proxy must authenticate the user.

## 🔑 Credentials

- The `password` setting is passed to the guest in the runner script, which is
  uploaded to `%SystemRoot%\Temp\packer-microsoft-update-<run id>\`. The script
  deletes itself when it starts and clears the variable after it registers the
  task. The directory inherits the permissions of `%SystemRoot%\Temp`, which
  standard users cannot read.
- Task Scheduler stores the password for the lifetime of the task. The runner
  deletes the task when the action ends.
- Declare the variable that holds the password as `sensitive = true`, and pass it
  through an environment variable (`PKR_VAR_...`) rather than a file in the
  repository. Packer then masks it in its output.
- The plugin never writes the password to the report or to its log messages.

## ✏️ What the plugin changes on the guest

| Change                                                                 | Lifetime                                                          |
| ---------------------------------------------------------------------- | ----------------------------------------------------------------- |
| Installed updates and offline packages                                 | Permanent; that is the purpose.                                   |
| Microsoft Update service registration (`microsoft-update` source only) | Permanent.                                                        |
| Work directory under `%SystemRoot%\Temp`                               | Deleted at the end of the run.                                    |
| Scheduled tasks `packer-microsoft-update-*`                            | Deleted after each action; leftovers are deleted by the next run. |
| Restarts                                                               | As needed, up to `max_reboots`.                                   |

The plugin does not change update policies, service start modes, firewall rules,
proxy settings, or ESU licensing.

## 📦 Offline packages

Before installing an offline package, the guest:

- compares the SHA-256 checksum with the configured value (`guest_path` packages
  must have one) or with the checksum computed on the build machine before upload,
- requires a valid Authenticode signature,
- requires the signer's subject to contain `O=Microsoft Corporation`.

A package that fails any check is not run and is reported as `rejected`. The
signature check relies on the guest's certificate store; on legacy systems
without SHA-2 support, current packages fail it.

## 🧰 Upstream filters

`filters` expressions are PowerShell script blocks that run in the scheduled task
with the task's rights. Anyone who can change the template can run code as
`SYSTEM` on the guest through them, exactly as with the `powershell` provisioner.

## 📊 The report

The report contains guest facts (release, build, edition, language, policy state,
free disk space), the list of updates, and error messages. It contains no
credentials. It is written with permissions `0600` on Unix-like systems.

## ⛓️ Supply chain

- Releases are built by GoReleaser in GitHub Actions. The checksum file is signed
  with keyless cosign, every archive has an SPDX SBOM, and the archives and the
  checksum file have GitHub build provenance attestations.
- Dependencies are kept current by Dependabot. CI runs `govulncheck` and CodeQL.
- GitHub Actions are pinned to commit SHAs.

Verify a release before use:

```sh
cosign verify-blob \
  --certificate packer-plugin-microsoft-update_v0.1.0_SHA256SUMS.pem \
  --signature packer-plugin-microsoft-update_v0.1.0_SHA256SUMS.sig \
  --certificate-identity-regexp 'https://github.com/larskroghdk/packer-plugin-microsoft-update/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  packer-plugin-microsoft-update_v0.1.0_SHA256SUMS
sha256sum --ignore-missing -c packer-plugin-microsoft-update_v0.1.0_SHA256SUMS
gh attestation verify packer-plugin-microsoft-update_v0.1.0_x5.0_windows_amd64.zip \
  --repo larskroghdk/packer-plugin-microsoft-update
```
