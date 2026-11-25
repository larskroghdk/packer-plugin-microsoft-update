package provisioner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/larskroghdk/packer-plugin-microsoft-update/internal/selection"
)

func decode(t *testing.T, raw map[string]any) (*Config, error) {
	t.Helper()
	var configuration Config
	err := configuration.Decode(raw)
	return &configuration, err
}

func mustDecode(t *testing.T, raw map[string]any) *Config {
	t.Helper()
	configuration, err := decode(t, raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	return configuration
}

func TestDefaults(t *testing.T) {
	configuration := mustDecode(t, map[string]any{})
	checks := map[string]bool{
		"update_source":        configuration.UpdateSource == "default",
		"profile":              configuration.Profile == "recommended",
		"search_criteria":      configuration.SearchCriteria == defaultSearchCriteria,
		"max_update_passes":    configuration.MaxUpdatePasses == 20,
		"max_reboots":          configuration.MaxReboots == 15,
		"max_updates_per_pass": configuration.MaxUpdatesPerPass == 1000,
		"max_install_attempts": configuration.MaxInstallAttempts == 2,
		"max_retries":          configuration.MaxRetries == 3,
		"search_timeout":       configuration.SearchTimeout == 30*time.Minute,
		"install_timeout":      configuration.InstallTimeout == 120*time.Minute,
		"reboot_timeout":       configuration.RebootTimeout == 60*time.Minute,
		"username":             configuration.Username == "SYSTEM",
		"log_level":            configuration.LogLevel == "info",
		"fail_on_error":        !configuration.FailOnError.False(),
	}
	for name, ok := range checks {
		if !ok {
			t.Errorf("unexpected default for %s", name)
		}
	}
	policy := configuration.Policy()
	if policy.IncludeDrivers || policy.IncludePreview || policy.IncludeOptional || policy.IncludeFeatureUpdates {
		t.Errorf("default policy must exclude optional, preview, driver, and feature updates: %+v", policy)
	}
}

func TestUpstreamAliases(t *testing.T) {
	configuration := mustDecode(t, map[string]any{
		"filters":            []string{"exclude:$_.Title -like '*Preview*'", "include:$true"},
		"update_limit":       25,
		"update_max_retries": 7,
		"restart_timeout":    "2h",
	})
	if configuration.MaxUpdatesPerPass != 25 || configuration.MaxRetries != 7 || configuration.RebootTimeout != 2*time.Hour {
		t.Fatalf("aliases not applied: %+v", configuration)
	}
	if configuration.SearchCriteria != upstreamLegacySearchCriteria {
		t.Fatalf("legacy filters must keep the upstream default search criteria, got %q", configuration.SearchCriteria)
	}
	if !configuration.Policy().LegacyFilters {
		t.Fatalf("legacy filters not enabled in policy")
	}
}

func TestProfileOverrides(t *testing.T) {
	configuration := mustDecode(t, map[string]any{
		"profile":                 "all",
		"include_preview_updates": false,
		"classifications":         []string{"security", "0e6cf1350-c01b-414d-a61f-263d14d133b4"[1:]},
		"include_kbs":             []string{"kb5034441"},
		"exclude_update_ids":      []string{"{ABCDEF01-2345-6789-ABCD-EF0123456789}"},
	})
	policy := configuration.Policy()
	if policy.IncludePreview || !policy.IncludeDrivers || !policy.IncludeFeatureUpdates {
		t.Fatalf("explicit settings must override the profile: %+v", policy)
	}
	if len(policy.Classifications) != 2 || !policy.Classifications[selection.Security] || !policy.Classifications[selection.Critical] {
		t.Fatalf("classifications %+v", policy.Classifications)
	}
	if policy.IncludeKBs[0] != "KB5034441" || policy.ExcludeUpdateIDs[0] != "abcdef01-2345-6789-abcd-ef0123456789" {
		t.Fatalf("identifiers not normalized: %+v %+v", policy.IncludeKBs, policy.ExcludeUpdateIDs)
	}
}

func TestValidation(t *testing.T) {
	temporaryDirectory := t.TempDir()
	packagePath := filepath.Join(temporaryDirectory, "windows10.0-kb5000001-x64.msu")
	if err := os.WriteFile(packagePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		raw     map[string]any
		message string
	}{
		{"unknown source", map[string]any{"update_source": "bits"}, "update_source must be one of"},
		{"unknown profile", map[string]any{"profile": "everything"}, "profile must be one of"},
		{"unknown classification", map[string]any{"classifications": []string{"hotfix"}}, `unknown classification "hotfix"`},
		{"bad KB", map[string]any{"exclude_kbs": []string{"KBX"}}, "is not a KB number"},
		{"bad update ID", map[string]any{"include_update_ids": []string{"123"}}, "is not an update ID"},
		{"empty title", map[string]any{"exclude_titles": []string{" "}}, "exclude_titles must not contain empty values"},
		{"bad filter", map[string]any{"filters": []string{"$true"}}, "must have the form include:<expression>"},
		{"filters with classifications", map[string]any{"filters": []string{"include:$true"}, "classifications": []string{"security"}}, "filters cannot be combined"},
		{"offline without packages", map[string]any{"update_source": "offline"}, "requires at least one offline_package"},
		{"package with both paths", map[string]any{"offline_package": []map[string]any{{"source": packagePath, "guest_path": `D:\a.msu`}}}, "exactly one of source and guest_path"},
		{"guest package without checksum", map[string]any{"offline_package": []map[string]any{{"guest_path": `D:\a.msu`}}}, "sha256 is required"},
		{"package with wrong extension", map[string]any{"offline_package": []map[string]any{{"guest_path": `D:\a.exe`, "sha256": strings.Repeat("a", 64)}}}, "must be a .msu or .cab"},
		{"missing package", map[string]any{"offline_package": []map[string]any{{"source": filepath.Join(temporaryDirectory, "missing.cab")}}}, "offline_package[0]"},
		{"bad checksum", map[string]any{"offline_package": []map[string]any{{"source": packagePath, "sha256": "abc"}}}, "64 hexadecimal"},
		{"negative reboots", map[string]any{"max_reboots": -1}, "max_reboots must not be negative"},
		{"bad log level", map[string]any{"log_level": "trace"}, "log_level must be info or debug"},
		{"password for SYSTEM", map[string]any{"password": "secret"}, "password must not be set"},
		{"user without password", map[string]any{"username": "packer"}, "password is required"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := decode(t, testCase.raw)
			if err == nil || !strings.Contains(err.Error(), testCase.message) {
				t.Fatalf("expected error containing %q, got %v", testCase.message, err)
			}
		})
	}
}

func TestValidOfflinePackage(t *testing.T) {
	packagePath := filepath.Join(t.TempDir(), "ssu.cab")
	if err := os.WriteFile(packagePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	configuration := mustDecode(t, map[string]any{
		"update_source":   "offline",
		"offline_package": []map[string]any{{"source": packagePath}, {"guest_path": `E:\updates\kb.msu`, "sha256": strings.Repeat("A", 64)}},
	})
	settings := (&Provisioner{config: *configuration}).engineSettings()
	if len(settings.OfflinePackages) != 2 || settings.OfflinePackages[1].SHA256 != strings.Repeat("a", 64) {
		t.Fatalf("offline packages %+v", settings.OfflinePackages)
	}
}

func TestConfigSpecExposesOptions(t *testing.T) {
	spec := (&Provisioner{}).ConfigSpec()
	for _, option := range []string{
		"update_source", "profile", "search_criteria", "classifications", "products",
		"include_optional_updates", "include_preview_updates", "include_driver_updates", "include_feature_updates",
		"include_kbs", "exclude_kbs", "include_update_ids", "exclude_update_ids", "include_titles", "exclude_titles",
		"filters", "offline_package", "max_update_passes", "max_reboots", "max_updates_per_pass", "update_limit",
		"max_install_attempts", "max_retries", "update_max_retries", "retry_delay", "search_timeout", "download_timeout",
		"install_timeout", "reboot_timeout", "restart_timeout", "servicing_timeout", "fail_on_error", "report_path",
		"log_level", "username", "password",
	} {
		if _, ok := spec[option]; !ok {
			t.Errorf("HCL2 spec lacks %s", option)
		}
	}
}

func TestExplicitZeroIsKept(t *testing.T) {
	configuration := mustDecode(t, map[string]any{
		"max_reboots":       0,
		"max_retries":       0,
		"retry_delay":       "0s",
		"servicing_timeout": "0s",
	})
	if configuration.MaxReboots != 0 || configuration.MaxRetries != 0 || configuration.RetryDelay != 0 || configuration.ServicingTimeout != 0 {
		t.Fatalf("explicit zero values were replaced by defaults: %+v", configuration)
	}
}

func TestExplicitZeroAliasesAreKept(t *testing.T) {
	configuration := mustDecode(t, map[string]any{"update_max_retries": 0})
	if configuration.MaxRetries != 0 {
		t.Fatalf("update_max_retries = 0 was replaced by the default, got %d", configuration.MaxRetries)
	}
}

func TestCanonicalNameWinsOverAlias(t *testing.T) {
	configuration := mustDecode(t, map[string]any{"max_retries": 4, "update_max_retries": 9, "reboot_timeout": "10m", "restart_timeout": "2h"})
	if configuration.MaxRetries != 4 || configuration.RebootTimeout != 10*time.Minute {
		t.Fatalf("canonical settings must win over aliases: %+v", configuration)
	}
}

func TestExplicitZeroRejectedWhereMeaningless(t *testing.T) {
	for _, name := range []string{"max_update_passes", "max_updates_per_pass", "update_limit", "max_install_attempts"} {
		if _, err := decode(t, map[string]any{name: 0}); err == nil || !strings.Contains(err.Error(), "at least 1") {
			t.Errorf("%s = 0 must be rejected, got %v", name, err)
		}
	}
	for _, name := range []string{"search_timeout", "download_timeout", "install_timeout", "reboot_timeout", "restart_timeout"} {
		if _, err := decode(t, map[string]any{name: "0s"}); err == nil || !strings.Contains(err.Error(), "greater than zero") {
			t.Errorf("%s = 0s must be rejected, got %v", name, err)
		}
	}
}

func decodeHCL(t *testing.T, source string) (*Config, error) {
	t.Helper()
	file, diagnostics := hclsyntax.ParseConfig([]byte(source), "test.pkr.hcl", hcl.InitialPos)
	if diagnostics.HasErrors() {
		t.Fatalf("parse: %v", diagnostics)
	}
	var configuration Config
	value, diagnostics := hcldec.Decode(file.Body, (&Provisioner{}).ConfigSpec(), nil)
	if diagnostics.HasErrors() {
		t.Fatalf("decode: %v", diagnostics)
	}
	err := configuration.Decode(value)
	return &configuration, err
}

func TestHCLUnsetAttributesUseDefaults(t *testing.T) {
	configuration, err := decodeHCL(t, `max_reboots = 0
servicing_timeout = "0s"
`)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if configuration.MaxReboots != 0 || configuration.ServicingTimeout != 0 {
		t.Fatalf("explicit zero values were replaced by defaults: %+v", configuration)
	}
	if configuration.MaxRetries != 3 || configuration.MaxUpdatePasses != 20 || configuration.RebootTimeout != 60*time.Minute {
		t.Fatalf("unset attributes must use their defaults: %+v", configuration)
	}
}
