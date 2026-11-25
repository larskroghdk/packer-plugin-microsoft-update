# 🪟 Packer Microsoft Update Provisioner

[![CI](https://github.com/larskroghdk/packer-plugin-microsoft-update/actions/workflows/ci.yml/badge.svg)](https://github.com/larskroghdk/packer-plugin-microsoft-update/actions/workflows/ci.yml)

A [Packer](https://developer.hashicorp.com/packer) plugin that installs Windows
updates during an image build. It installs from Windows Update, Microsoft Update,
WSUS, or offline `.msu` and `.cab` packages, restarts the guest as often as the
updates need, and writes a JSON report of every update it saw.

- 🎛️ **Declarative selection**: profiles, classifications, products, KB numbers,
  update IDs, and title patterns, validated when Packer starts.
- 🌐 **Every update source**: Windows Update, Microsoft Update (registered on demand),
  WSUS, and signed offline packages with checksum checks.
- 🔁 **Restarts handled**: pending restart detection, restarts verified by boot time,
  a wait for Windows servicing after each restart, and limits on passes and restarts.
- 🐢 **Reliable on slow guests**: per-phase timeouts, retries with backoff for transient
  errors, and detection of unexpected restarts.
- 📊 **Clear results**: a run status that decides whether the build fails, HRESULTs
  with names and hints, and a JSON report with a published schema.
- 🛡️ **Policy-safe**: never changes update policies, services, or licensing.

This plugin started as a fork of
[rgl/packer-plugin-windows-update](https://github.com/rgl/packer-plugin-windows-update).
Templates for its `windows-update` provisioner can be migrated with small changes;
see [Migrating from windows-update](docs/migration.md).

## 🚀 Quick start

Require the plugin in your template:

```hcl
packer {
  required_plugins {
    microsoft-update = {
      source  = "github.com/larskroghdk/microsoft-update"
      version = ">= 0.1.0"
    }
  }
}
```

Install it:

```sh
packer init .
```

Add the provisioner to a build with a Windows source and a WinRM communicator:

```hcl
build {
  sources = ["source.hyperv-vmcx.windows"]

  provisioner "microsoft-update" {
    update_source = "microsoft-update"
    report_path   = "artifacts/updates.json"
  }
}
```

This installs security, critical, definition, update rollup, service pack, feature
pack, tool, and regular updates from Microsoft Update, restarts as needed, and fails
the build if any selected update fails or remains.

More selection:

```hcl
provisioner "microsoft-update" {
  profile                 = "all"
  include_feature_updates = false
  exclude_kbs             = ["KB5034441"]
  exclude_titles          = ["*Preview*"]
  max_reboots             = 25
}
```

## 📚 Documentation

| Topic                                                                         | Content                                                             |
| ----------------------------------------------------------------------------- | ------------------------------------------------------------------- |
| [Configuration reference](docs/integration/provisioners/microsoft-update.mdx) | Every setting with its default.                                     |
| [Update sources](docs/update-sources.md)                                      | Windows Update, Microsoft Update, WSUS, offline packages, policies. |
| [Update selection](docs/update-selection.md)                                  | How the plugin decides which updates to install.                    |
| [Restart handling](docs/reboot-handling.md)                                   | When and how the guest restarts.                                    |
| [Reporting](docs/reporting.md)                                                | The JSON report and its schema.                                     |
| [Troubleshooting](docs/troubleshooting.md)                                    | Statuses, HRESULTs, and legacy systems.                             |
| [Limitations](docs/limitations.md)                                            | What the plugin does not do.                                        |
| [Security](docs/security.md)                                                  | Rights, credentials, and changes on the guest.                      |
| [Compatibility](docs/compatibility.md)                                        | Windows releases and their test status.                             |
| [Migrating from windows-update](docs/migration.md)                            | Moving from the upstream provisioner.                               |
| [Architecture](docs/architecture.md)                                          | How the plugin works inside.                                        |
| [Development](docs/development.md)                                            | Building, testing, and releasing.                                   |

The [examples](examples/) cover Windows Update, Microsoft Update, WSUS, offline
packages, KB filtering, upstream filters, JSON reporting, legacy releases, and
Windows Server.

## 🧩 Compatibility

The plugin is built for Windows 7 SP1 through Windows 11 and Windows Server 2012
through Windows Server 2025, on any builder with a WinRM communicator. No release
has been tested in a live build yet; see the
[compatibility matrix](docs/compatibility.md). Windows Insider Preview builds are
not supported.

## 🤝 Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Report security problems as described in
[SECURITY.md](SECURITY.md).

## 📄 License

[Mozilla Public License 2.0](LICENSE.txt).
