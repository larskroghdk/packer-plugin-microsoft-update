# Security policy

## Supported versions

Security fixes are released for the latest minor version only.

| Version | Supported |
| ------- | --------- |
| 0.1.x   | Yes       |

## Report a vulnerability

Do not open a public issue for a security problem.

Report it privately through
[GitHub private vulnerability reporting](https://github.com/larskroghdk/packer-plugin-microsoft-update/security/advisories/new).
Include:

- the affected version,
- a description of the problem and its impact,
- steps to reproduce it, or a proof of concept,
- any suggested fix.

You can expect an acknowledgement within five working days and an assessment
within two weeks. Once a fix is released, the advisory is published and you are
credited, unless you prefer otherwise.

## Scope

In scope:

- the plugin binary and the guest scripts it runs,
- the handling of credentials (`password`) and offline packages,
- the release artifacts and their signatures.

Out of scope:

- vulnerabilities in Windows, the Windows Update Agent, or the updates themselves;
  report those to the [Microsoft Security Response Center](https://msrc.microsoft.com/report),
- vulnerabilities in Packer or its other plugins,
- `filters` expressions running code on the guest; they are PowerShell by design.

See [docs/security.md](docs/security.md) for what the plugin does on the guest and
how to verify a release.
