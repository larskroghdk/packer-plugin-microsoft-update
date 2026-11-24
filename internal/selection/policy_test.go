package selection

import "testing"

func category(id, categoryType string) Category {
	return Category{ID: id, Type: categoryType}
}

var (
	securityCategory   = category("0fa1201d-4330-4fa8-8ae9-b877473b6441", "UpdateClassification")
	definitionCategory = category("e0789628-ce08-4437-be74-2495b842f43b", "UpdateClassification")
	driverCategory     = category("ebfc1fc5-71a4-4f7b-9aca-3b9a503104a0", "UpdateClassification")
	upgradeCategory    = category("3689bdc8-b205-4af4-8d4a-a63924c5e9d5", "UpdateClassification")
	updatesCategory    = category("cd5ffd1e-e932-4e3a-bf74-18bf0b1bbd83", "UpdateClassification")
	officeProduct      = Category{ID: "30eb551c-6288-4716-9a78-f300ec36d72b", Name: "Office 2016", Type: "Product"}
)

func boolPointer(value bool) *bool { return &value }

func mustProfile(t *testing.T, profile Profile) Policy {
	t.Helper()
	policy, err := ProfilePolicy(profile)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func TestDecide(t *testing.T) {
	recommended := mustProfile(t, ProfileRecommended)
	security := mustProfile(t, ProfileSecurity)
	all := mustProfile(t, ProfileAll)

	withExclusions := recommended
	withExclusions.ExcludeKBs = []string{"KB5000001"}
	withExclusions.ExcludeTitles = []string{"*Silverlight*"}

	allowList := recommended
	allowList.IncludeKBs = []string{"KB5000002"}

	allowListWithExclusion := allowList
	allowListWithExclusion.ExcludeUpdateIDs = []string{"00000000-0000-0000-0000-000000000002"}

	officeOnly := recommended
	officeOnly.Products = []string{"office 2016"}

	legacy := recommended
	legacy.LegacyFilters = true

	optional := recommended
	optional.IncludeOptional = true

	securityPatch := Update{ID: "00000000-0000-0000-0000-000000000001", Title: "Security Update for Windows", KBs: []string{"KB5000001"}, Categories: []Category{securityCategory}}
	cumulative := Update{ID: "00000000-0000-0000-0000-000000000002", Title: "Cumulative Update", KBs: []string{"5000002"}, Categories: []Category{securityCategory}}
	definitions := Update{ID: "00000000-0000-0000-0000-000000000003", Title: "Defender intelligence", Categories: []Category{definitionCategory}}
	driver := Update{ID: "00000000-0000-0000-0000-000000000004", Title: "Intel driver", Type: "driver", Categories: []Category{driverCategory}}
	featureUpdate := Update{ID: "00000000-0000-0000-0000-000000000005", Title: "Feature update to Windows 11, version 25H2", BrowseOnly: true, Categories: []Category{upgradeCategory}}
	preview := Update{ID: "00000000-0000-0000-0000-000000000006", Title: "2026-10 Cumulative Update Preview for Windows 11", BrowseOnly: true, Categories: []Category{updatesCategory}}
	optionalUpdate := Update{ID: "00000000-0000-0000-0000-000000000007", Title: "Optional update", BrowseOnly: true, Categories: []Category{updatesCategory}}
	silverlight := Update{ID: "00000000-0000-0000-0000-000000000008", Title: "Update for Microsoft Silverlight", Categories: []Category{securityCategory}}
	officeUpdate := Update{ID: "00000000-0000-0000-0000-000000000009", Title: "Security Update for Office", Categories: []Category{securityCategory, officeProduct}}
	regularUpdate := Update{ID: "00000000-0000-0000-0000-00000000000a", Title: "Update", Categories: []Category{updatesCategory}}
	unclassified := Update{ID: "00000000-0000-0000-0000-00000000000b", Title: "Unclassified"}
	beta := Update{ID: "00000000-0000-0000-0000-00000000000c", Title: "Insider", IsBeta: true, Categories: []Category{securityCategory}}
	legacyIncluded := Update{ID: "00000000-0000-0000-0000-00000000000d", Title: "legacy", Categories: []Category{driverCategory}, Type: "driver", LegacyFilterMatch: boolPointer(true)}
	legacySkipped := Update{ID: "00000000-0000-0000-0000-00000000000e", Title: "legacy", Categories: []Category{securityCategory}, LegacyFilterMatch: boolPointer(false)}

	cases := []struct {
		name     string
		policy   Policy
		update   Update
		selected bool
		excluded bool
	}{
		{"recommended installs security", recommended, securityPatch, true, false},
		{"recommended installs definitions", recommended, definitions, true, false},
		{"recommended installs regular updates", recommended, regularUpdate, true, false},
		{"unclassified updates count as regular updates", recommended, unclassified, true, false},
		{"recommended skips drivers", recommended, driver, false, false},
		{"recommended skips feature updates", recommended, featureUpdate, false, false},
		{"recommended skips previews", recommended, preview, false, false},
		{"recommended skips beta updates", recommended, beta, false, false},
		{"recommended skips optional updates", recommended, optionalUpdate, false, false},
		{"optional enabled installs optional updates", optional, optionalUpdate, true, false},
		{"optional enabled still skips previews", optional, preview, false, false},
		{"security skips regular updates", security, regularUpdate, false, false},
		{"all installs drivers", all, driver, true, false},
		{"all installs feature updates", all, featureUpdate, true, false},
		{"all installs previews", all, preview, true, false},
		{"excluded KB", withExclusions, securityPatch, false, true},
		{"excluded title wildcard", withExclusions, silverlight, false, true},
		{"allow list selects by KB without prefix", allowList, cumulative, true, false},
		{"allow list skips other updates", allowList, securityPatch, false, false},
		{"exclusion wins over allow list", allowListWithExclusion, cumulative, false, true},
		{"product filter keeps product", officeOnly, officeUpdate, true, false},
		{"product filter drops other products", officeOnly, securityPatch, false, false},
		{"legacy filter includes regardless of kind", legacy, legacyIncluded, true, false},
		{"legacy filter skips", legacy, legacySkipped, false, false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			decision := testCase.policy.Decide(testCase.update)
			if decision.Selected != testCase.selected || decision.Excluded != testCase.excluded {
				t.Fatalf("got selected=%v excluded=%v (%s), want selected=%v excluded=%v",
					decision.Selected, decision.Excluded, decision.Reason, testCase.selected, testCase.excluded)
			}
			if decision.Reason == "" {
				t.Fatalf("decision without reason")
			}
		})
	}
}

func TestParseClassification(t *testing.T) {
	for _, value := range []string{"security", "SECURITY", "0fa1201d-4330-4fa8-8ae9-b877473b6441", "{0FA1201D-4330-4FA8-8AE9-B877473B6441}"} {
		if classification, ok := ParseClassification(value); !ok || classification != Security {
			t.Errorf("ParseClassification(%q) = %q, %v", value, classification, ok)
		}
	}
	if _, ok := ParseClassification("bogus"); ok {
		t.Errorf("bogus classification accepted")
	}
}

func TestNormalizers(t *testing.T) {
	if kb, ok := NormalizeKB("kb5034441"); !ok || kb != "KB5034441" {
		t.Errorf("NormalizeKB = %q %v", kb, ok)
	}
	if kb, ok := NormalizeKB("5034441"); !ok || kb != "KB5034441" {
		t.Errorf("NormalizeKB without prefix = %q %v", kb, ok)
	}
	for _, invalid := range []string{"", "KB", "KBabc", "KB12"} {
		if _, ok := NormalizeKB(invalid); ok {
			t.Errorf("NormalizeKB(%q) accepted", invalid)
		}
	}
	if id, ok := NormalizeUpdateID("{ABCDEF01-2345-6789-ABCD-EF0123456789}"); !ok || id != "abcdef01-2345-6789-abcd-ef0123456789" {
		t.Errorf("NormalizeUpdateID = %q %v", id, ok)
	}
	if _, ok := NormalizeUpdateID("not-a-guid"); ok {
		t.Errorf("invalid update ID accepted")
	}
}

func TestTitlePatterns(t *testing.T) {
	cases := []struct {
		pattern string
		title   string
		match   bool
	}{
		{"silverlight", "Update for Microsoft Silverlight (KB1)", true},
		{"*Cumulative Update*", "2026-11 Cumulative Update for Windows 11 Version 25H2 for x64-based Systems", true},
		{"*.NET Framework 3.5/4.8*", "Cumulative Update for .NET Framework 3.5/4.8.1", true},
		{"2026-1? *", "2026-11 Update", true},
		{"Preview", "Security Update", false},
	}
	for _, testCase := range cases {
		if got := titlePatternMatches(testCase.pattern, testCase.title); got != testCase.match {
			t.Errorf("titlePatternMatches(%q, %q) = %v", testCase.pattern, testCase.title, got)
		}
	}
}

func TestPreviewDetectionRequiresBrowseOnly(t *testing.T) {
	update := Update{Title: "Security Update for Windows Insider Preview", Categories: []Category{securityCategory}}
	if update.IsPreview() {
		t.Fatalf("a required security update must not be classified as a preview because of its title")
	}
}
