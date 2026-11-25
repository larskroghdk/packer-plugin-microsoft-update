# 🔀 Migrating from windows-update

This plugin started as a fork of
[rgl/packer-plugin-windows-update](https://github.com/rgl/packer-plugin-windows-update)
and was rewritten. Templates for the upstream `windows-update` provisioner work
after two changes, and can then move to the new settings step by step.

## 1️⃣ 1. Change the plugin and the provisioner type

```hcl
packer {
  required_plugins {
    microsoft-update = {
      source  = "github.com/larskroghdk/microsoft-update"
      version = ">= 0.1.0"
    }
  }
}

build {
  provisioner "microsoft-update" {
    # settings as before
  }
}
```

Run `packer init` again.

## 2️⃣ 2. Check the settings that behave differently

| Upstream setting       | In this plugin                                | Difference                                                                                                                               |
| ---------------------- | --------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| `search_criteria`      | `search_criteria`                             | Same meaning. Default is `IsInstalled=0 and IsHidden=0`, or the upstream default `BrowseOnly=0 and IsInstalled=0` when `filters` is set. |
| `filters`              | `filters`                                     | Same syntax and evaluation. Cannot be combined with the declarative selection settings, except the `exclude_*` lists.                    |
| `update_limit`         | `max_updates_per_pass` (alias `update_limit`) | Same meaning.                                                                                                                            |
| `update_max_retries`   | `max_retries` (alias `update_max_retries`)    | Applies to every guest operation, with exponential backoff.                                                                              |
| `restart_timeout`      | `reboot_timeout` (alias `restart_timeout`)    | Default lowered from `4h` to `60m`.                                                                                                      |
| `username`, `password` | `username`, `password`                        | Same. `password` is now rejected with `SYSTEM` and required otherwise.                                                                   |

New behavior that can change a build:

- **The run fails when selected updates remain or fail** at the end of the run.
  Set `fail_on_error = false` to let the build continue while you investigate.
- **The number of passes and restarts is limited** by `max_update_passes` (`20`)
  and `max_reboots` (`15`). Raise them for old media.
- **Policies are respected.** A guest with `DisableWindowsUpdateAccess` or a
  disabled `wuauserv` service ends with status `unsupported`.
- **Updates that keep coming back** are marked failed after `max_install_attempts`
  (`2`) instead of being retried in every pass.

## 3️⃣ 3. Replace filters with declarative settings

The declarative settings are validated when Packer starts, are evaluated on the
build machine, and give each update a reason in the report.

| Upstream filters                                                                | Declarative settings                                                                                    |
| ------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| `search_criteria = "AutoSelectOnWebSites=1 and IsInstalled=0"`, `include:$true` | `profile = "security"` is the closest match.                                                            |
| `search_criteria = "BrowseOnly=0 and IsInstalled=0"`, `include:$true`           | `profile = "recommended"` (the default)                                                                 |
| `search_criteria = "IsInstalled=0"`, `include:$true`                            | `profile = "all"`                                                                                       |
| `exclude:$_.Title -like '*Preview*'`                                            | `exclude_titles = ["*Preview*"]`, or leave `include_preview_updates` unset                              |
| `exclude:$_.KBArticleIDs -contains '5034441'`                                   | `exclude_kbs = ["KB5034441"]`                                                                           |
| `include:$_.KBArticleIDs -contains '890830'`                                    | `include_kbs = ["KB890830"]`                                                                            |
| `exclude:$_.InstallationBehavior.CanRequestUserInput`                           | No equivalent; the plugin warns about these updates. Keep this as a filter or exclude the update by KB. |
| `include:$_.Categories \| Where-Object Name -eq 'Security Updates'`             | `classifications = ["security"]`                                                                        |

The profiles select by classification, while the upstream search criteria select
by Windows Update flags, so the sets of updates are close but not identical.
Compare the reports of both configurations on the same base image before you
switch.

## 4️⃣ 4. Use the new features

- `update_source = "microsoft-update"` to update other Microsoft products.
- `report_path` for a JSON record of the run.
- `offline_package` blocks for air-gapped builds and legacy prerequisites.
- Remove `windows-restart` provisioners that only followed `windows-update`; the
  plugin restarts the guest until Windows reports no pending restart.
