package main

import (
	"fmt"
	"os"

	"github.com/hashicorp/packer-plugin-sdk/plugin"
	"github.com/larskroghdk/packer-plugin-microsoft-update/provisioner"
	"github.com/larskroghdk/packer-plugin-microsoft-update/version"
)

func main() {
	pluginSet := plugin.NewSet()
	pluginSet.RegisterProvisioner(plugin.DEFAULT_NAME, new(provisioner.Provisioner))
	pluginSet.SetVersion(version.PluginVersion)
	if err := pluginSet.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
