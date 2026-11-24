package selection

import (
	"fmt"
	"regexp"
	"strings"
)

type Profile string

const (
	ProfileSecurity    Profile = "security"
	ProfileRecommended Profile = "recommended"
	ProfileAll         Profile = "all"
)

var Profiles = []Profile{ProfileSecurity, ProfileRecommended, ProfileAll}

type Policy struct {
	Classifications       map[Classification]bool
	IncludeOptional       bool
	IncludePreview        bool
	IncludeDrivers        bool
	IncludeFeatureUpdates bool
	Products              []string
	IncludeKBs            []string
	ExcludeKBs            []string
	IncludeUpdateIDs      []string
	ExcludeUpdateIDs      []string
	IncludeTitles         []string
	ExcludeTitles         []string
	LegacyFilters         bool
}

func ProfilePolicy(profile Profile) (Policy, error) {
	switch profile {
	case ProfileSecurity:
		return Policy{Classifications: setOf(Security, Critical, Definition)}, nil
	case ProfileRecommended, "":
		return Policy{Classifications: setOf(Security, Critical, Definition, UpdateRollup, ServicePack, FeaturePack, Tool, Regular)}, nil
	case ProfileAll:
		return Policy{
			Classifications:       setOf(AllClassifications...),
			IncludeOptional:       true,
			IncludePreview:        true,
			IncludeDrivers:        true,
			IncludeFeatureUpdates: true,
		}, nil
	default:
		return Policy{}, fmt.Errorf("unknown profile %q", profile)
	}
}

func setOf(values ...Classification) map[Classification]bool {
	set := make(map[Classification]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

type Decision struct {
	Selected bool
	Excluded bool
	Reason   string
}

func selected(reason string) Decision { return Decision{Selected: true, Reason: reason} }
func skipped(reason string) Decision  { return Decision{Reason: reason} }
func excluded(reason string) Decision { return Decision{Excluded: true, Reason: reason} }

func (p Policy) Decide(update Update) Decision {
	if containsFold(p.ExcludeUpdateIDs, update.ID) {
		return excluded("listed in exclude_update_ids")
	}
	if kb, ok := firstSharedKB(p.ExcludeKBs, update.KBs); ok {
		return excluded(fmt.Sprintf("%s is listed in exclude_kbs", kb))
	}
	if pattern, ok := firstTitleMatch(p.ExcludeTitles, update.Title); ok {
		return excluded(fmt.Sprintf("title matches exclude_titles pattern %q", pattern))
	}

	if p.hasIncludeLists() {
		if containsFold(p.IncludeUpdateIDs, update.ID) {
			return selected("listed in include_update_ids")
		}
		if kb, ok := firstSharedKB(p.IncludeKBs, update.KBs); ok {
			return selected(fmt.Sprintf("%s is listed in include_kbs", kb))
		}
		if pattern, ok := firstTitleMatch(p.IncludeTitles, update.Title); ok {
			return selected(fmt.Sprintf("title matches include_titles pattern %q", pattern))
		}
		return skipped("not matched by include_kbs, include_update_ids, or include_titles")
	}

	if p.LegacyFilters {
		if update.LegacyFilterMatch != nil && *update.LegacyFilterMatch {
			return selected("included by filters")
		}
		return skipped("not included by filters")
	}

	if len(p.Products) > 0 && !p.matchesProduct(update) {
		return skipped("product is not listed in products")
	}

	switch {
	case update.IsDriver():
		if p.IncludeDrivers {
			return selected("driver updates are enabled")
		}
		return skipped("driver update (enable include_driver_updates)")
	case update.IsFeatureUpdate():
		if p.IncludeFeatureUpdates {
			return selected("feature updates are enabled")
		}
		return skipped("feature update (enable include_feature_updates)")
	case update.IsPreview():
		if !p.IncludePreview {
			return skipped("preview update (enable include_preview_updates)")
		}
	case update.BrowseOnly:
		if !p.IncludeOptional {
			return skipped("optional update (enable include_optional_updates)")
		}
	}

	classification := update.Classification()
	if classification == Unclassified {
		classification = Regular
	}
	if !p.Classifications[classification] {
		return skipped(fmt.Sprintf("classification %q is not enabled", classification))
	}
	return selected(fmt.Sprintf("classification %q is enabled", classification))
}

func (p Policy) hasIncludeLists() bool {
	return len(p.IncludeKBs) > 0 || len(p.IncludeUpdateIDs) > 0 || len(p.IncludeTitles) > 0
}

func (p Policy) matchesProduct(update Update) bool {
	for _, product := range update.Products() {
		for _, wanted := range p.Products {
			if strings.EqualFold(wanted, product.ID) || strings.EqualFold(wanted, product.Name) {
				return true
			}
		}
	}
	return false
}

var kbPattern = regexp.MustCompile(`^(?i:kb)?(\d{4,8})$`)

func NormalizeKB(value string) (string, bool) {
	match := kbPattern.FindStringSubmatch(strings.TrimSpace(value))
	if match == nil {
		return "", false
	}
	return "KB" + match[1], true
}

var guidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func NormalizeUpdateID(value string) (string, bool) {
	trimmed := strings.Trim(strings.TrimSpace(value), "{}")
	if !guidPattern.MatchString(trimmed) {
		return "", false
	}
	return strings.ToLower(trimmed), true
}

func firstSharedKB(wanted, kbs []string) (string, bool) {
	for _, kb := range kbs {
		normalized, ok := NormalizeKB(kb)
		if !ok {
			continue
		}
		for _, candidate := range wanted {
			if strings.EqualFold(candidate, normalized) {
				return normalized, true
			}
		}
	}
	return "", false
}

func firstTitleMatch(patterns []string, title string) (string, bool) {
	for _, pattern := range patterns {
		if titlePatternMatches(pattern, title) {
			return pattern, true
		}
	}
	return "", false
}

func titlePatternMatches(pattern, title string) bool {
	if !strings.ContainsAny(pattern, "*?") {
		return strings.Contains(strings.ToLower(title), strings.ToLower(pattern))
	}
	var expression strings.Builder
	expression.WriteString("(?is)^")
	for _, character := range pattern {
		switch character {
		case '*':
			expression.WriteString(".*")
		case '?':
			expression.WriteString(".")
		default:
			expression.WriteString(regexp.QuoteMeta(string(character)))
		}
	}
	expression.WriteString("$")
	return regexp.MustCompile(expression.String()).MatchString(title)
}

func containsFold(values []string, value string) bool {
	for _, candidate := range values {
		if strings.EqualFold(candidate, value) {
			return true
		}
	}
	return false
}
