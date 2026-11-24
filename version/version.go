package version

import "github.com/hashicorp/packer-plugin-sdk/version"

var (
	Version           = "0.1.0"
	VersionPrerelease = ""
	VersionMetadata   = ""
	Commit            = "unknown"
	PluginVersion     = version.NewPluginVersion(Version, VersionPrerelease, VersionMetadata)
)
