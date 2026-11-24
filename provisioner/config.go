//go:generate packer-sdc mapstructure-to-hcl2 -type Config,OfflinePackage

package provisioner

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
	"github.com/larskroghdk/packer-plugin-microsoft-update/internal/guest"
	"github.com/larskroghdk/packer-plugin-microsoft-update/internal/selection"
	"github.com/mitchellh/mapstructure"
)

type OfflinePackage struct {
	Source    string `mapstructure:"source"`
	GuestPath string `mapstructure:"guest_path"`
	SHA256    string `mapstructure:"sha256"`
}
type Config struct {
	common.PackerConfig    `mapstructure:",squash"`
	UpdateSource           string           `mapstructure:"update_source"`
	Profile                string           `mapstructure:"profile"`
	SearchCriteria         string           `mapstructure:"search_criteria"`
	Classifications        []string         `mapstructure:"classifications"`
	Products               []string         `mapstructure:"products"`
	IncludeOptionalUpdates config.Trilean   `mapstructure:"include_optional_updates"`
	IncludePreviewUpdates  config.Trilean   `mapstructure:"include_preview_updates"`
	IncludeDriverUpdates   config.Trilean   `mapstructure:"include_driver_updates"`
	IncludeFeatureUpdates  config.Trilean   `mapstructure:"include_feature_updates"`
	IncludeKBs             []string         `mapstructure:"include_kbs"`
	ExcludeKBs             []string         `mapstructure:"exclude_kbs"`
	IncludeUpdateIDs       []string         `mapstructure:"include_update_ids"`
	ExcludeUpdateIDs       []string         `mapstructure:"exclude_update_ids"`
	IncludeTitles          []string         `mapstructure:"include_titles"`
	ExcludeTitles          []string         `mapstructure:"exclude_titles"`
	Filters                []string         `mapstructure:"filters"`
	OfflinePackages        []OfflinePackage `mapstructure:"offline_package"`
	MaxUpdatePasses        int              `mapstructure:"max_update_passes"`
	MaxReboots             int              `mapstructure:"max_reboots"`
	MaxUpdatesPerPass      int              `mapstructure:"max_updates_per_pass"`
	UpdateLimit            int              `mapstructure:"update_limit"`
	MaxInstallAttempts     int              `mapstructure:"max_install_attempts"`
	MaxRetries             int              `mapstructure:"max_retries"`
	UpdateMaxRetries       int              `mapstructure:"update_max_retries"`
	RetryDelay             time.Duration    `mapstructure:"retry_delay"`
	SearchTimeout          time.Duration    `mapstructure:"search_timeout"`
	DownloadTimeout        time.Duration    `mapstructure:"download_timeout"`
	InstallTimeout         time.Duration    `mapstructure:"install_timeout"`
	RebootTimeout          time.Duration    `mapstructure:"reboot_timeout"`
	RestartTimeout         time.Duration    `mapstructure:"restart_timeout"`
	ServicingTimeout       time.Duration    `mapstructure:"servicing_timeout"`
	FailOnError            config.Trilean   `mapstructure:"fail_on_error"`
	ReportPath             string           `mapstructure:"report_path"`
	LogLevel               string           `mapstructure:"log_level"`
	Username               string           `mapstructure:"username"`
	Password               string           `mapstructure:"password"`
	ctx                    interpolate.Context
	explicitKeys           map[string]bool
}

const (
	defaultSearchCriteria        = "IsInstalled=0 and IsHidden=0"
	upstreamLegacySearchCriteria = "BrowseOnly=0 and IsInstalled=0"
)

var updateSources = []string{
	guest.SourceDefault, guest.SourceWindowsUpdate, guest.SourceMicrosoftUpdate, guest.SourceWSUS, guest.SourceOffline,
}
var sha256Pattern = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

func (c *Config) Decode(raws ...any) error {
	var metadata mapstructure.Metadata
	err := config.Decode(c, &config.DecodeOpts{
		Metadata:           &metadata,
		PluginType:         "microsoft-update",
		Interpolate:        true,
		InterpolateContext: &c.ctx,
	}, raws...)
	if err != nil {
		return err
	}
	c.explicitKeys = map[string]bool{}
	for _, key := range metadata.Keys {
		c.explicitKeys[key] = true
	}
	c.applyDefaults()
	return c.validate()
}

func (c *Config) applyDefaults() {
	if c.UpdateSource == "" {
		c.UpdateSource = guest.SourceDefault
	}
	c.UpdateSource = strings.ToLower(c.UpdateSource)
	if c.Profile == "" {
		c.Profile = string(selection.ProfileRecommended)
	}
	c.Profile = strings.ToLower(c.Profile)
	if c.SearchCriteria == "" {
		c.SearchCriteria = defaultSearchCriteria
		if len(c.Filters) > 0 {
			c.SearchCriteria = upstreamLegacySearchCriteria
		}
	}
	applyAlias(c, "max_updates_per_pass", &c.MaxUpdatesPerPass, "update_limit", c.UpdateLimit)
	applyAlias(c, "max_retries", &c.MaxRetries, "update_max_retries", c.UpdateMaxRetries)
	applyAlias(c, "reboot_timeout", &c.RebootTimeout, "restart_timeout", c.RestartTimeout)
	defaultUnlessSet(c, "max_update_passes", &c.MaxUpdatePasses, 20)
	defaultUnlessSet(c, "max_reboots", &c.MaxReboots, 15)
	defaultUnlessSet(c, "max_updates_per_pass", &c.MaxUpdatesPerPass, 1000)
	defaultUnlessSet(c, "max_install_attempts", &c.MaxInstallAttempts, 2)
	defaultUnlessSet(c, "max_retries", &c.MaxRetries, 3)
	defaultUnlessSet(c, "retry_delay", &c.RetryDelay, 15*time.Second)
	defaultUnlessSet(c, "search_timeout", &c.SearchTimeout, 30*time.Minute)
	defaultUnlessSet(c, "download_timeout", &c.DownloadTimeout, 120*time.Minute)
	defaultUnlessSet(c, "install_timeout", &c.InstallTimeout, 120*time.Minute)
	defaultUnlessSet(c, "reboot_timeout", &c.RebootTimeout, 60*time.Minute)
	defaultUnlessSet(c, "servicing_timeout", &c.ServicingTimeout, 30*time.Minute)
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
	if c.Username == "" {
		c.Username = "SYSTEM"
	}
}

func applyAlias[T any](c *Config, key string, target *T, aliasKey string, aliasValue T) {
	if !c.explicitKeys[key] && c.explicitKeys[aliasKey] {
		*target = aliasValue
		c.explicitKeys[key] = true
	}
}

func defaultUnlessSet[T any](c *Config, key string, target *T, value T) {
	if !c.explicitKeys[key] {
		*target = value
	}
}

func (c *Config) validate() error {
	var errs *packer.MultiError
	fail := func(format string, arguments ...any) {
		errs = packer.MultiErrorAppend(errs, fmt.Errorf(format, arguments...))
	}
	if !slices.Contains(updateSources, c.UpdateSource) {
		fail("update_source must be one of %s, got %q", strings.Join(updateSources, ", "), c.UpdateSource)
	}
	if _, err := selection.ProfilePolicy(selection.Profile(c.Profile)); err != nil {
		fail("profile must be one of security, recommended, all, got %q", c.Profile)
	}
	for _, classification := range c.Classifications {
		if _, ok := selection.ParseClassification(classification); !ok {
			fail("classifications: unknown classification %q", classification)
		}
	}
	for _, field := range []struct {
		name   string
		values []string
	}{{"include_kbs", c.IncludeKBs}, {"exclude_kbs", c.ExcludeKBs}} {
		for _, value := range field.values {
			if _, ok := selection.NormalizeKB(value); !ok {
				fail("%s: %q is not a KB number such as KB5034441", field.name, value)
			}
		}
	}
	for _, field := range []struct {
		name   string
		values []string
	}{{"include_update_ids", c.IncludeUpdateIDs}, {"exclude_update_ids", c.ExcludeUpdateIDs}} {
		for _, value := range field.values {
			if _, ok := selection.NormalizeUpdateID(value); !ok {
				fail("%s: %q is not an update ID (GUID)", field.name, value)
			}
		}
	}
	for _, field := range []struct {
		name   string
		values []string
	}{{"include_titles", c.IncludeTitles}, {"exclude_titles", c.ExcludeTitles}, {"products", c.Products}} {
		for _, value := range field.values {
			if strings.TrimSpace(value) == "" {
				fail("%s must not contain empty values", field.name)
			}
		}
	}
	for _, filter := range c.Filters {
		action, expression, found := strings.Cut(filter, ":")
		if !found || (action != "include" && action != "exclude") || strings.TrimSpace(expression) == "" {
			fail("filters: %q must have the form include:<expression> or exclude:<expression>", filter)
		}
	}
	if len(c.Filters) > 0 {
		conflicting := c.Profile != string(selection.ProfileRecommended) || len(c.Classifications) > 0 || len(c.Products) > 0 ||
			len(c.IncludeKBs) > 0 || len(c.IncludeUpdateIDs) > 0 || len(c.IncludeTitles) > 0 ||
			c.IncludeOptionalUpdates != config.TriUnset || c.IncludePreviewUpdates != config.TriUnset ||
			c.IncludeDriverUpdates != config.TriUnset || c.IncludeFeatureUpdates != config.TriUnset
		if conflicting {
			fail("filters cannot be combined with profile, classifications, products, include_* lists, or include_*_updates; use exclude_* lists to refine filters")
		}
	}
	if c.UpdateSource == guest.SourceOffline && len(c.OfflinePackages) == 0 {
		fail(`update_source = "offline" requires at least one offline_package block`)
	}
	for index, offlinePackage := range c.OfflinePackages {
		label := fmt.Sprintf("offline_package[%d]", index)
		hasSource, hasGuestPath := offlinePackage.Source != "", offlinePackage.GuestPath != ""
		if hasSource == hasGuestPath {
			fail("%s: set exactly one of source and guest_path", label)
			continue
		}
		packagePath := offlinePackage.Source + offlinePackage.GuestPath
		extension := strings.ToLower(filepath.Ext(strings.ReplaceAll(packagePath, `\`, "/")))
		if extension != ".msu" && extension != ".cab" {
			fail("%s: %q must be a .msu or .cab package", label, packagePath)
		}
		if offlinePackage.SHA256 != "" && !sha256Pattern.MatchString(offlinePackage.SHA256) {
			fail("%s: sha256 must be 64 hexadecimal characters", label)
		}
		if hasGuestPath && offlinePackage.SHA256 == "" {
			fail("%s: sha256 is required for guest_path packages", label)
		}
		if hasSource {
			if info, err := os.Stat(offlinePackage.Source); err != nil {
				fail("%s: %v", label, err)
			} else if info.IsDir() {
				fail("%s: %q is a directory", label, offlinePackage.Source)
			}
		}
	}
	positive := []struct {
		name  string
		value int
	}{
		{"max_update_passes", c.MaxUpdatePasses}, {"max_updates_per_pass", c.MaxUpdatesPerPass},
		{"max_install_attempts", c.MaxInstallAttempts},
	}
	for _, field := range positive {
		if field.value < 1 {
			fail("%s must be at least 1", field.name)
		}
	}
	if c.MaxReboots < 0 {
		fail("max_reboots must not be negative")
	}
	if c.MaxRetries < 0 {
		fail("max_retries must not be negative")
	}
	type durationSetting struct {
		name  string
		value time.Duration
	}
	for _, field := range []durationSetting{
		{"search_timeout", c.SearchTimeout}, {"download_timeout", c.DownloadTimeout},
		{"install_timeout", c.InstallTimeout}, {"reboot_timeout", c.RebootTimeout},
	} {
		if field.value <= 0 {
			fail("%s must be greater than zero", field.name)
		}
	}
	for _, field := range []durationSetting{{"retry_delay", c.RetryDelay}, {"servicing_timeout", c.ServicingTimeout}} {
		if field.value < 0 {
			fail("%s must not be negative", field.name)
		}
	}
	if c.LogLevel != "info" && c.LogLevel != "debug" {
		fail("log_level must be info or debug, got %q", c.LogLevel)
	}
	if strings.EqualFold(c.Username, "SYSTEM") && c.Password != "" {
		fail("password must not be set when username is SYSTEM")
	}
	if !strings.EqualFold(c.Username, "SYSTEM") && c.Password == "" {
		fail("password is required when username is not SYSTEM")
	}
	if errs == nil || len(errs.Errors) == 0 {
		return nil
	}
	return errs
}

func (c *Config) Policy() selection.Policy {
	policy, err := selection.ProfilePolicy(selection.Profile(c.Profile))
	if err != nil {
		panic(errors.New("Policy called on an invalid configuration"))
	}
	if len(c.Classifications) > 0 {
		policy.Classifications = map[selection.Classification]bool{}
		for _, value := range c.Classifications {
			classification, _ := selection.ParseClassification(value)
			policy.Classifications[classification] = true
		}
	}
	policy.IncludePreview = resolve(c.IncludePreviewUpdates, policy.IncludePreview)
	policy.IncludeDrivers = resolve(c.IncludeDriverUpdates, policy.IncludeDrivers)
	policy.IncludeFeatureUpdates = resolve(c.IncludeFeatureUpdates, policy.IncludeFeatureUpdates)
	policy.IncludeOptional = resolve(c.IncludeOptionalUpdates, policy.IncludeOptional)
	policy.Products = c.Products
	policy.IncludeKBs = normalizeAll(c.IncludeKBs, selection.NormalizeKB)
	policy.ExcludeKBs = normalizeAll(c.ExcludeKBs, selection.NormalizeKB)
	policy.IncludeUpdateIDs = normalizeAll(c.IncludeUpdateIDs, selection.NormalizeUpdateID)
	policy.ExcludeUpdateIDs = normalizeAll(c.ExcludeUpdateIDs, selection.NormalizeUpdateID)
	policy.IncludeTitles = c.IncludeTitles
	policy.ExcludeTitles = c.ExcludeTitles
	policy.LegacyFilters = len(c.Filters) > 0
	return policy
}

func resolve(value config.Trilean, profileDefault bool) bool {
	if value == config.TriUnset {
		return profileDefault
	}
	return value.True()
}

func normalizeAll(values []string, normalize func(string) (string, bool)) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		if canonical, ok := normalize(value); ok {
			normalized = append(normalized, canonical)
		}
	}
	return normalized
}
