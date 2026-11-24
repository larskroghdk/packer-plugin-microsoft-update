---
name: guest-script-author
description: Use when editing or adding PowerShell that runs on the Windows guest in packer-plugin-microsoft-update, such as internal/guest/scripts/microsoft-update.ps1 or elevated-runner.ps1, or when a Compatibility.Tests.ps1 or PSScriptAnalyzer check fails on them.
---

# Guest Script Author

## Overview

The guest scripts run on every supported Windows release, down to Windows 7 SP1
with Windows PowerShell 2.0, inside a scheduled task with no console. They talk to
the plugin only through `@@MU@@ ` JSON records on standard output.

## Constraints

| Not available in PowerShell 2.0               | Use instead                                           |
| --------------------------------------------- | ----------------------------------------------------- |
| `ConvertTo-Json`, `ConvertFrom-Json`          | `ConvertTo-RecordJson` / `Write-Record` in the script |
| `Get-CimInstance`                             | `Get-WmiObject`                                       |
| `[pscustomobject]`, `[ordered]`               | `@{}` hashtables                                      |
| `[Type]::new()`                               | `New-Object`                                          |
| `-in`, `-notin`                               | `-contains`, `-notcontains`                           |
| `.Where()`, `.ForEach()`                      | `Where-Object`, `foreach`                             |
| `$PSItem`, `$PSScriptRoot`                    | `$_`, `Split-Path $MyInvocation.MyCommand.Path`       |
| `-Raw`, `-Depth`, `-NoNewline`                | `[System.IO.File]` methods, `[Console]::Out.Write`    |
| `Get-ScheduledTask`, `Register-ScheduledTask` | `Schedule.Service` COM object                         |

Also:

- `Set-StrictMode -Version 2.0` is on: read optional COM properties with
  `Get-ComPropertyValue` and test members with `Test-ComMember`.
- Name functions `Verb-Noun` with approved verbs and descriptive nouns.
- Stroustrup braces: `}` then a new line with `elseif`, `else`, `catch`, `finally`.
- Never write to the console directly except through `Write-Record`,
  `Write-LogRecord`, and `Write-Phase`; unprefixed output only shows at debug level.
- Emit `Write-Phase` before any long operation, so the plugin applies the right timeout.
- Errors that the plugin must classify carry an HRESULT: use `Get-ErrorDetail`.
- `elevated-runner.ps1` is a Go `text/template` with `<% %>` delimiters; quote
  values with `psquote`.

## Adding a record or action

1. Write the record with `Write-Record '<type>' @{ ... }` in snake_case keys.
2. Parse it in `internal/guest/client.go` and add the type in `types.go`.
3. Add a Pester test in `test/guest/MicrosoftUpdate.Tests.ps1` and a Go test in
   `internal/guest/client_test.go`.
4. Document the record in `docs/architecture.md`.

## Verify

```powershell
pwsh -NoProfile -File test/Invoke-GuestTests.ps1
pwsh -NoProfile -File test/Invoke-ScriptAnalysis.ps1 -Path internal/guest/scripts
```

Rebuild the plugin afterwards: the scripts are embedded at build time.

## Common Mistakes

- Testing only in PowerShell 7: it accepts syntax that fails on 2.0.
- Relying on `$ErrorActionPreference` alone for COM calls; wrap them in `try` and
  emit a record with the HRESULT.
- Returning a single-item array without the unary comma (`return , $list`), which
  PowerShell unrolls.
