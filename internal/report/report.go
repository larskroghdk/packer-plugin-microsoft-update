package report

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const SchemaVersion = "1.0.0"

//go:embed schema.json
var Schema []byte

type Status string

const (
	StatusUpdated     Status = "updated"
	StatusPartial     Status = "partial"
	StatusFailed      Status = "failed"
	StatusUnsupported Status = "unsupported"
	StatusCancelled   Status = "cancelled"
)

type UpdateState string

const (
	StateInstalled     UpdateState = "installed"
	StateFailed        UpdateState = "failed"
	StateExcluded      UpdateState = "excluded"
	StateSkipped       UpdateState = "skipped"
	StateRemaining     UpdateState = "remaining"
	StateNotApplicable UpdateState = "not-applicable"
	StateSuperseded    UpdateState = "superseded"
)

type Report struct {
	SchemaVersion   string           `json:"schema_version"`
	Plugin          Plugin           `json:"plugin"`
	Status          Status           `json:"status"`
	StatusReason    string           `json:"status_reason"`
	StartedAt       time.Time        `json:"started_at"`
	CompletedAt     time.Time        `json:"completed_at"`
	DurationSeconds float64          `json:"duration_seconds"`
	System          *System          `json:"system"`
	Source          Source           `json:"source"`
	Settings        Settings         `json:"settings"`
	Passes          int              `json:"passes"`
	Reboots         int              `json:"reboots"`
	RebootEvents    []RebootEvent    `json:"reboot_events"`
	Summary         Summary          `json:"summary"`
	Updates         []Update         `json:"updates"`
	OfflinePackages []OfflinePackage `json:"offline_packages"`
	Errors          []Error          `json:"errors"`
}

type Plugin struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
}

type System struct {
	Caption                     string   `json:"caption"`
	Version                     string   `json:"version"`
	Build                       int      `json:"build"`
	UpdateBuildRevision         int      `json:"update_build_revision"`
	DisplayVersion              string   `json:"display_version"`
	EditionID                   string   `json:"edition_id"`
	InstallationType            string   `json:"installation_type"`
	ProductType                 int      `json:"product_type"`
	Architecture                string   `json:"architecture"`
	Language                    string   `json:"language"`
	PowerShellVersion           string   `json:"powershell_version"`
	CLRVersion                  string   `json:"clr_version"`
	WindowsUpdateAgent          string   `json:"windows_update_agent_version"`
	InsiderBranch               string   `json:"insider_branch"`
	WSUSConfigured              bool     `json:"wsus_configured"`
	WindowsUpdateAccessDisabled bool     `json:"windows_update_access_disabled"`
	InternetLocationsDisabled   bool     `json:"internet_locations_disabled"`
	MicrosoftUpdateRegistered   bool     `json:"microsoft_update_registered"`
	SupportsAddService2         bool     `json:"supports_add_service2"`
	UpdateServiceStartMode      string   `json:"update_service_start_mode"`
	SystemDriveFreeBytes        int64    `json:"system_drive_free_bytes"`
	LifecycleNotice             string   `json:"lifecycle_notice,omitempty"`
	MissingPrerequisites        []string `json:"missing_prerequisites"`
}

type Source struct {
	Requested     string   `json:"requested"`
	Effective     string   `json:"effective"`
	ServiceID     string   `json:"service_id,omitempty"`
	PolicyManaged bool     `json:"policy_managed"`
	Warnings      []string `json:"warnings"`
}

type Settings struct {
	Profile               string   `json:"profile"`
	SearchCriteria        string   `json:"search_criteria"`
	Classifications       []string `json:"classifications"`
	IncludeOptional       bool     `json:"include_optional_updates"`
	IncludePreview        bool     `json:"include_preview_updates"`
	IncludeDrivers        bool     `json:"include_driver_updates"`
	IncludeFeatureUpdates bool     `json:"include_feature_updates"`
	LegacyFilters         bool     `json:"legacy_filters"`
	MaxUpdatePasses       int      `json:"max_update_passes"`
	MaxReboots            int      `json:"max_reboots"`
	MaxInstallAttempts    int      `json:"max_install_attempts"`
}

type RebootEvent struct {
	At       time.Time `json:"at"`
	Reason   string    `json:"reason"`
	Expected bool      `json:"expected"`
}

type Summary struct {
	Discovered    int `json:"discovered"`
	Installed     int `json:"installed"`
	Failed        int `json:"failed"`
	Skipped       int `json:"skipped"`
	Excluded      int `json:"excluded"`
	NotApplicable int `json:"not_applicable"`
	Remaining     int `json:"remaining"`
}

type Update struct {
	ID             string      `json:"id"`
	Revision       int         `json:"revision"`
	Title          string      `json:"title"`
	KBs            []string    `json:"kbs"`
	Classification string      `json:"classification"`
	Products       []string    `json:"products"`
	BrowseOnly     bool        `json:"browse_only"`
	Preview        bool        `json:"preview"`
	State          UpdateState `json:"state"`
	Reason         string      `json:"reason"`
	FirstSeenPass  int         `json:"first_seen_pass"`
	Attempts       []Attempt   `json:"attempts"`
}

type Attempt struct {
	Pass           int    `json:"pass"`
	Phase          string `json:"phase"`
	Result         string `json:"result"`
	ResultCode     int    `json:"result_code"`
	HResult        string `json:"hresult"`
	HResultName    string `json:"hresult_name"`
	RebootRequired bool   `json:"reboot_required"`
}

type OfflinePackage struct {
	Name            string `json:"name"`
	SHA256          string `json:"sha256"`
	SignatureStatus string `json:"signature_status"`
	Signer          string `json:"signer"`
	Result          string `json:"result"`
	ExitCode        int    `json:"exit_code"`
	HResultName     string `json:"hresult_name"`
	RebootRequired  bool   `json:"reboot_required"`
}

type Error struct {
	At      time.Time `json:"at"`
	Phase   string    `json:"phase"`
	Class   string    `json:"class"`
	HResult string    `json:"hresult,omitempty"`
	Message string    `json:"message"`
}

func New(plugin Plugin, startedAt time.Time) *Report {
	return &Report{
		SchemaVersion:   SchemaVersion,
		Plugin:          plugin,
		StartedAt:       startedAt.UTC(),
		RebootEvents:    []RebootEvent{},
		Updates:         []Update{},
		OfflinePackages: []OfflinePackage{},
		Errors:          []Error{},
		Source:          Source{Warnings: []string{}},
	}
}

func (r *Report) Finish(completedAt time.Time, status Status, reason string) {
	r.CompletedAt = completedAt.UTC()
	r.DurationSeconds = r.CompletedAt.Sub(r.StartedAt).Seconds()
	r.Status = status
	r.StatusReason = reason
	r.Summary = Summary{}
	for _, update := range r.Updates {
		r.Summary.Discovered++
		switch update.State {
		case StateInstalled:
			r.Summary.Installed++
		case StateFailed:
			r.Summary.Failed++
		case StateSkipped:
			r.Summary.Skipped++
		case StateExcluded:
			r.Summary.Excluded++
		case StateNotApplicable, StateSuperseded:
			r.Summary.NotApplicable++
		case StateRemaining:
			r.Summary.Remaining++
		}
	}
	if r.Settings.Classifications == nil {
		r.Settings.Classifications = []string{}
	}
	if r.Source.Warnings == nil {
		r.Source.Warnings = []string{}
	}
	if r.System != nil && r.System.MissingPrerequisites == nil {
		r.System.MissingPrerequisites = []string{}
	}
	for index := range r.Updates {
		if r.Updates[index].KBs == nil {
			r.Updates[index].KBs = []string{}
		}
		if r.Updates[index].Products == nil {
			r.Updates[index].Products = []string{}
		}
		if r.Updates[index].Attempts == nil {
			r.Updates[index].Attempts = []Attempt{}
		}
	}
}

func (r *Report) Write(path string) error {
	content, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	if directory := filepath.Dir(path); directory != "" {
		if err := os.MkdirAll(directory, 0o750); err != nil {
			return fmt.Errorf("create report directory: %w", err)
		}
	}
	if err := os.WriteFile(path, append(content, '\n'), 0o600); err != nil {
		return fmt.Errorf("write report %s: %w", path, err)
	}
	return nil
}
