# Contributing

Thank you for helping improve the Microsoft Update provisioner. This guide covers
how to report problems and how to propose changes.

## Report a bug

Open an issue with the bug report form. Include:

- the plugin version and Packer version,
- the Windows release and build of the guest (`winver`, or `system.caption` and
  `system.build` from the report),
- the provisioner block from your template, without credentials,
- the JSON report (`report_path`), or the Packer output with `log_level = "debug"`.

Report security problems privately, as described in [SECURITY.md](SECURITY.md).

## Propose a change

For anything larger than a small fix, open an issue first so we can agree on the
approach before you write code.

1. Fork the repository and create a branch from `main`.
2. Set up the tools as described in [docs/development.md](docs/development.md).
3. Write a test that fails without your change.
4. Make the change.
5. Run the checks:

   ```sh
   make generate
   make lint
   make test
   ```

6. Update the documentation. Configuration changes must update
   [`docs/integration/provisioners/microsoft-update.mdx`](docs/integration/provisioners/microsoft-update.mdx).
7. Add an entry under "Unreleased" in [CHANGELOG.md](CHANGELOG.md) for changes
   users will notice.
8. Open a pull request and fill in the template.

## Code style

- Format Go code with `gofmt`. golangci-lint must pass.
- PowerShell uses Stroustrup braces and must pass PSScriptAnalyzer with
  [`PSScriptAnalyzerSettings.psd1`](PSScriptAnalyzerSettings.psd1).
- The guest scripts in `internal/guest/scripts/` must run on Windows PowerShell 2.0.

## Testing on real guests

Unit tests and Pester tests run anywhere. Changes to the guest scripts, the
restart logic, or the update loop also need a live run on at least one Windows
release; describe which release and the resulting report status in the pull
request. See [docs/development.md](docs/development.md#integration-tests).

## Scope

The plugin does not change update policies, services, or licensing on the guest,
and does not support Windows Insider Preview builds. Pull requests that change
this are out of scope. See [docs/limitations.md](docs/limitations.md).

## Code of conduct

This project follows the [Code of Conduct](CODE_OF_CONDUCT.md).

## License

By contributing, you agree that your contributions are licensed under the
[Mozilla Public License 2.0](LICENSE.txt).
