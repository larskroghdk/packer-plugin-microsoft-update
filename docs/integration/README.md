The Microsoft Update plugin installs Windows updates on Windows guests during a
Packer build. It installs from Windows Update, Microsoft Update, WSUS, or offline
`.msu` and `.cab` packages, restarts the guest as often as the updates need, and
writes a JSON report of every update it saw.

### Installation

To install this plugin, copy and paste this code into your Packer configuration,
then run [`packer init`](https://developer.hashicorp.com/packer/docs/commands/init).

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

Alternatively, you can use `packer plugins install` to manage installation of this plugin.

```sh
packer plugins install github.com/larskroghdk/microsoft-update
```

### Components

#### Provisioners

- [microsoft-update](/packer/integrations/larskroghdk/microsoft-update/latest/components/provisioner/microsoft-update) -
  Searches for, downloads, and installs Windows updates in passes, restarts the
  guest when needed, and reports the result.
