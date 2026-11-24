---
name: reviewer
description: Use when reviewing a change, diff, or pull request in packer-plugin-microsoft-update before it is committed or merged, including reviews of Go code, guest PowerShell scripts, docs, examples, or workflows.
---

# Reviewer

## Overview

Review every change against this repository's rules before it is committed. A
review produces findings, each with a file and line, what is wrong, and a concrete
failure scenario. A change is not ready while a finding is unresolved.

## Checklist

Work through every item that the diff touches.

**Correctness**
- Config change in `provisioner/config.go`: default applied, validation added,
  `explicitKeys` respected (an explicit `0` is a real value), alias precedence
  (canonical name wins), and a test in `provisioner/config_test.go`.
- Engine change: every new guest call goes through `withRetry`; a new error is
  classified (`wuerror` class or a typed guest error); the final status is still
  one of `updated`, `partial`, `failed`, `unsupported`, `cancelled`.
- Guest client change: phase timeouts still apply; the scheduled task is aborted on
  cancellation and timeout; nothing new is left in the guest work directory.
- Wire protocol change: the record is written in `microsoft-update.ps1`, parsed in
  `internal/guest/client.go`, and covered on both sides.
- Report change: `internal/report/schema.json` and `schema_version` updated,
  `docs/reporting.md` updated.

**Repository rules**
- Guest scripts use nothing newer than Windows PowerShell 2.0.
- PowerShell uses Stroustrup braces and passes `PSScriptAnalyzerSettings.psd1`.
- `docs/integration/provisioners/microsoft-update.mdx` matches the configuration.
- No Windows Insider examples, scenarios, or tested claims.
- No compatibility row marked "Tested" without a live run.
- The plugin does not change update policies, services, or licensing on the guest.
- No credentials in code, examples, tests, logs, or the report.

**Verification**
- `gofmt -l .` is empty, `go test ./...` passes, `make test-powershell` passes,
  `make lint` passes.
- `CHANGELOG.md` has an "Unreleased" entry for user-visible changes.

## Output

For each finding: severity (`blocking` or `nit`), `file:line`, the defect, and the
input or state that triggers it. End with a verdict: `ready` or `changes needed`.
Report "no findings" explicitly when the checklist is clean.

## Common Mistakes

- Reviewing only the Go side of a protocol change.
- Accepting "tests pass" without running them.
- Treating a missing `.mdx` update as a nit; it is blocking, because the reference
  is maintained by hand.
