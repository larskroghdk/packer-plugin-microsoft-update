---
name: docs-maintainer
description: Use when a change in packer-plugin-microsoft-update adds, removes, or changes a configuration setting, default, validation rule, update source, report field, restart behavior, supported Windows release, or example, or when writing or editing anything under docs/ or README.md.
---

# Docs Maintainer

## Overview

The Packer configuration reference cannot be generated from the code and is
maintained by hand, so every behavior change updates the docs in the same change.

## Where each fact lives

| Change                                   | Update                                               |
| ---------------------------------------- | ---------------------------------------------------- |
| Setting, default, alias, validation      | `docs/integration/provisioners/microsoft-update.mdx` |
| Update source or policy check            | `docs/update-sources.md`, the `.mdx`                 |
| Selection rule or classification         | `docs/update-selection.md`                           |
| Restart, pending restart, servicing wait | `docs/reboot-handling.md`                            |
| Report field, state, status              | `docs/reporting.md`, `internal/report/schema.json`   |
| HRESULT or error hint                    | `docs/troubleshooting.md` when it is common          |
| Something the plugin does not do         | `docs/limitations.md`                                |
| Rights, credentials, guest changes       | `docs/security.md`                                   |
| Upstream setting behavior                | `docs/migration.md`                                  |
| Package, protocol record, phase          | `docs/architecture.md`                               |
| Windows release or scenario              | `docs/compatibility.md`                              |
| User-visible change                      | `CHANGELOG.md` under "Unreleased"                    |

## Writing rules

- Describe behavior from the code, not from intent. Read the default in
  `applyDefaults` and the check in `validate` before writing about a setting.
- For each setting: type, what it does, default, allowed values, alias.
- Compatibility rows stay "Not tested" until a live integration run passes.
  Windows Insider builds are always "Not supported".
- Links between docs are relative; links from the `.mdx` to other docs are absolute
  GitHub URLs, because the `.mdx` is published to the Packer registry.
- Align Markdown tables; the editor lint (MD060) flags misaligned pipes.

## After editing

Render the registry docs and check that nothing else changed:

```sh
make generate
git status --short .web-docs
```

Commit `.web-docs/` together with `docs/integration/`.

## Common Mistakes

- Updating the README example but not the `.mdx` reference.
- Documenting a default that `applyDefaults` does not set.
- Claiming a Windows release works because unit tests pass.
