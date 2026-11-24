package selection

import (
	"regexp"
	"strings"
)

type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type Update struct {
	ID                   string     `json:"id"`
	Revision             int        `json:"revision"`
	Title                string     `json:"title"`
	KBs                  []string   `json:"kbs"`
	Categories           []Category `json:"categories"`
	Type                 string     `json:"type"`
	BrowseOnly           bool       `json:"browse_only"`
	AutoSelectOnWebSites bool       `json:"auto_select_on_web_sites"`
	IsMandatory          bool       `json:"is_mandatory"`
	IsBeta               bool       `json:"is_beta"`
	IsDownloaded         bool       `json:"is_downloaded"`
	CanRequestUserInput  bool       `json:"can_request_user_input"`
	RebootBehavior       int        `json:"reboot_behavior"`
	MaxDownloadSize      int64      `json:"max_download_size"`
	MsrcSeverity         string     `json:"msrc_severity"`
	LastDeploymentChange string     `json:"last_deployment_change"`
	LegacyFilterMatch    *bool      `json:"legacy_filter_match,omitempty"`
}

type Classification string

const (
	Security     Classification = "security"
	Critical     Classification = "critical"
	Definition   Classification = "definition"
	UpdateRollup Classification = "update-rollup"
	ServicePack  Classification = "service-pack"
	FeaturePack  Classification = "feature-pack"
	Tool         Classification = "tool"
	Regular      Classification = "update"
	Driver       Classification = "driver"
	Upgrade      Classification = "upgrade"
	Application  Classification = "application"
	Unclassified Classification = "unclassified"
)

var classificationIDs = map[string]Classification{
	"0fa1201d-4330-4fa8-8ae9-b877473b6441": Security,
	"e6cf1350-c01b-414d-a61f-263d14d133b4": Critical,
	"e0789628-ce08-4437-be74-2495b842f43b": Definition,
	"28bc880e-0592-4cbf-8f95-c79b17911d5f": UpdateRollup,
	"68c5b0a3-d1a6-4553-ae49-01d3a7827828": ServicePack,
	"b54e7d24-7add-428f-8b75-90a396fa584f": FeaturePack,
	"b4832bd8-e735-4761-8daf-37f882276dab": Tool,
	"cd5ffd1e-e932-4e3a-bf74-18bf0b1bbd83": Regular,
	"ebfc1fc5-71a4-4f7b-9aca-3b9a503104a0": Driver,
	"77835c8d-62a7-41f5-82ad-f28d1af1e3b1": Driver,
	"3689bdc8-b205-4af4-8d4a-a63924c5e9d5": Upgrade,
	"5c9376ab-8ce6-464a-b136-22113dd69801": Application,
}

var AllClassifications = []Classification{
	Security, Critical, Definition, UpdateRollup, ServicePack, FeaturePack,
	Tool, Regular, Driver, Upgrade, Application,
}

func ParseClassification(value string) (Classification, bool) {
	normalized := strings.ToLower(strings.Trim(strings.TrimSpace(value), "{}"))
	if classification, found := classificationIDs[normalized]; found {
		return classification, true
	}
	for _, classification := range AllClassifications {
		if string(classification) == normalized {
			return classification, true
		}
	}
	return "", false
}

func (u Update) Classification() Classification {
	for _, category := range u.Categories {
		if !strings.EqualFold(category.Type, "UpdateClassification") {
			continue
		}
		if classification, found := classificationIDs[strings.ToLower(category.ID)]; found {
			return classification
		}
	}
	if strings.EqualFold(u.Type, "driver") {
		return Driver
	}
	return Unclassified
}

func (u Update) Products() []Category {
	var products []Category
	for _, category := range u.Categories {
		if strings.EqualFold(category.Type, "Product") || strings.EqualFold(category.Type, "ProductFamily") {
			products = append(products, category)
		}
	}
	return products
}

var previewTitle = regexp.MustCompile(`(?i)\bpreview\b`)

func (u Update) IsPreview() bool {
	return u.IsBeta || (u.BrowseOnly && previewTitle.MatchString(u.Title))
}

func (u Update) IsDriver() bool {
	return strings.EqualFold(u.Type, "driver") || u.Classification() == Driver
}

func (u Update) IsFeatureUpdate() bool {
	return u.Classification() == Upgrade
}
