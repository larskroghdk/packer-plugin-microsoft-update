# Update selection

Selection happens in two steps:

1. The guest searches the update source with `search_criteria`
   (default `IsInstalled=0 and IsHidden=0`). Only updates that match are considered.
2. The provisioner decides for each update found whether to install it, skip it,
   or exclude it. The decision and its reason are recorded in the report.

## Order of the rules

For each update, the first rule that applies decides:

1. **Exclusions.** `exclude_update_ids`, then `exclude_kbs`, then `exclude_titles`.
   A match makes the update `excluded`. Exclusions apply in every mode, including
   with `filters`.
2. **Include lists.** If any of `include_update_ids`, `include_kbs`, or
   `include_titles` is set, an update is selected only if one of them matches,
   and `skipped` otherwise. The remaining rules are not evaluated, so an include
   list can select a driver, preview, or feature update.
3. **Upstream filters.** If `filters` is set, the update is selected when the first
   matching filter is an `include` filter, and `skipped` otherwise.
4. **Products.** If `products` is set, an update for a product that is not listed
   is `skipped`.
5. **Update kind.**
   - Driver updates need `include_driver_updates`.
   - Feature updates (classification `upgrade`) need `include_feature_updates`.
   - Preview updates need `include_preview_updates`. An update counts as a preview
     when Windows Update marks it as beta, or when it is optional and its title
     contains the word "Preview".
   - Other optional (`BrowseOnly`) updates need `include_optional_updates`.
6. **Classification.** The update is selected when its classification is enabled by
   `profile` or `classifications`. Updates without a known classification are
   treated as `update`.

The `include_*_updates` settings default to the profile: all `true` for
`profile = "all"`, all `false` otherwise.

## Within a pass

After the decision, the provisioner also:

- installs at most `max_updates_per_pass` updates in one pass; the rest wait for
  the next pass,
- defers an update when another update with the same title is already queued in
  the pass, which avoids `WU_E_DUPLICATE_ITEM` with updates that are offered for
  several products,
- marks an update `failed` when it is still offered after `max_install_attempts`
  installation attempts,
- warns when an update can request user input, because such an update can stop
  responding in a non-interactive session.

## Classifications

| Name            | Windows Update classification | GUID                                                                           |
| --------------- | ----------------------------- | ------------------------------------------------------------------------------ |
| `security`      | Security Updates              | `0fa1201d-4330-4fa8-8ae9-b877473b6441`                                         |
| `critical`      | Critical Updates              | `e6cf1350-c01b-414d-a61f-263d14d133b4`                                         |
| `definition`    | Definition Updates            | `e0789628-ce08-4437-be74-2495b842f43b`                                         |
| `update-rollup` | Update Rollups                | `28bc880e-0592-4cbf-8f95-c79b17911d5f`                                         |
| `service-pack`  | Service Packs                 | `68c5b0a3-d1a6-4553-ae49-01d3a7827828`                                         |
| `feature-pack`  | Feature Packs                 | `b54e7d24-7add-428f-8b75-90a396fa584f`                                         |
| `tool`          | Tools                         | `b4832bd8-e735-4761-8daf-37f882276dab`                                         |
| `update`        | Updates                       | `cd5ffd1e-e932-4e3a-bf74-18bf0b1bbd83`                                         |
| `driver`        | Drivers, Driver Sets          | `ebfc1fc5-71a4-4f7b-9aca-3b9a503104a0`, `77835c8d-62a7-41f5-82ad-f28d1af1e3b1` |
| `upgrade`       | Upgrades                      | `3689bdc8-b205-4af4-8d4a-a63924c5e9d5`                                         |
| `application`   | Applications                  | `5c9376ab-8ce6-464a-b136-22113dd69801`                                         |

## Title patterns

`include_titles` and `exclude_titles` ignore case.

- A pattern without `*` or `?` matches when the title contains it:
  `Microsoft Edge` matches "Update for Microsoft Edge-Stable Channel".
- A pattern with `*` or `?` must match the whole title: `*Preview*` matches any
  title containing "Preview", and `Security Update for *` matches only titles
  that start with "Security Update for ".

## Examples

Security updates only:

```hcl
profile = "security"
```

Everything except feature updates and Edge:

```hcl
profile                 = "all"
include_feature_updates = false
exclude_titles          = ["Microsoft Edge"]
```

Exactly two updates, nothing else:

```hcl
include_kbs = ["KB890830", "KB2267602"]
```

Software only, with a custom search:

```hcl
search_criteria = "IsInstalled=0 and IsHidden=0 and Type='Software'"
classifications = ["security", "critical", "update-rollup"]
```

## Upstream filters

`filters` keeps templates written for the upstream `windows-update` provisioner
working. The expressions run as PowerShell on the guest with the `IUpdate` COM
object as `$_`. Packer checks only the `include:` or `exclude:` prefix; mistakes
in an expression show up when it runs on the guest, and an expression that is
valid but wrong silently skips updates. Move to the declarative settings when you can; see
[Migrating from windows-update](migration.md).
