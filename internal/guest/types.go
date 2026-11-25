package guest

import (
	"errors"
	"fmt"
	"time"

	"github.com/larskroghdk/packer-plugin-microsoft-update/internal/report"
	"github.com/larskroghdk/packer-plugin-microsoft-update/internal/selection"
	"github.com/larskroghdk/packer-plugin-microsoft-update/internal/wuerror"
)

const (
	SourceDefault         = "default"
	SourceWindowsUpdate   = "windows-update"
	SourceMicrosoftUpdate = "microsoft-update"
	SourceWSUS            = "wsus"
	SourceOffline         = "offline"
)
const MicrosoftUpdateServiceID = "7971f918-a847-4430-9279-4a52d1efe18d"

type System struct {
	report.System
	PendingReboot PendingReboot `json:"pending_reboot"`
	BootTime      string        `json:"boot_time"`
}
type PendingReboot struct {
	Pending              bool     `json:"pending"`
	Reasons              []string `json:"reasons"`
	InformationalReasons []string `json:"informational_reasons"`
	ServicingIdle        *bool    `json:"servicing_idle,omitempty"`
}
type SearchRequest struct {
	Source        string
	Criteria      string
	LegacyFilters []string
}
type SearchResult struct {
	ResultCode  int                `json:"result_code"`
	Result      string             `json:"result"`
	UpdateCount int                `json:"update_count"`
	HResult     wuerror.HResult    `json:"hresult"`
	Message     string             `json:"message"`
	Updates     []selection.Update `json:"-"`
}

func (r SearchResult) Succeeded() bool {
	return r.ResultCode == 2 && wuerror.IsSuccess(uint32(r.HResult))
}

type InstallRequest struct {
	SearchRequest
	UpdateIDs []string
}

type UpdateResult struct {
	ID             string          `json:"id"`
	Phase          string          `json:"phase"`
	Result         string          `json:"result"`
	ResultCode     int             `json:"result_code"`
	HResult        wuerror.HResult `json:"hresult"`
	RebootRequired bool            `json:"reboot_required"`
}

func (r UpdateResult) Succeeded() bool { return r.ResultCode == 2 || r.ResultCode == 3 }

type InstallResult struct {
	Updates        []UpdateResult  `json:"-"`
	ResultCode     int             `json:"result_code"`
	Result         string          `json:"result"`
	HResult        wuerror.HResult `json:"hresult"`
	Message        string          `json:"message"`
	RebootRequired bool            `json:"reboot_required"`
}

type Registration struct {
	State   string          `json:"state"`
	Changed bool            `json:"changed"`
	HResult wuerror.HResult `json:"hresult"`
	Message string          `json:"message"`
}

type OfflinePackage struct {
	HostPath  string
	GuestPath string
	SHA256    string
}

type OfflinePackageResult struct {
	Name            string `json:"name"`
	SHA256          string `json:"sha256"`
	SignatureStatus string `json:"signature_status"`
	Signer          string `json:"signer"`
	Result          string `json:"result"`
	ExitCode        int    `json:"exit_code"`
	RebootRequired  bool   `json:"reboot_required"`
	Message         string `json:"message"`
}

type OfflineResult struct {
	Packages      []OfflinePackageResult
	PendingReboot PendingReboot
}

type TimeoutError struct {
	Phase   string
	Timeout time.Duration
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("the %s phase did not finish within %s", e.Phase, e.Timeout)
}

type CommunicationError struct {
	Operation string
	Err       error
}

func (e *CommunicationError) Error() string {
	return fmt.Sprintf("%s: communication with the guest failed: %v", e.Operation, e.Err)
}

func (e *CommunicationError) Unwrap() error { return e.Err }

type ScriptError struct {
	Action   string
	HResult  wuerror.HResult
	Message  string
	Position string
}

func (e *ScriptError) Error() string {
	message := fmt.Sprintf("guest action %q failed: %s", e.Action, e.Message)
	if e.HResult != 0 {
		message += " [" + wuerror.Describe(uint32(e.HResult)) + "]"
	}
	return message
}

func IsRetryable(err error) bool {
	var communicationError *CommunicationError
	if errors.As(err, &communicationError) {
		return true
	}
	var scriptError *ScriptError
	if errors.As(err, &scriptError) {
		return scriptError.HResult.Info().Class == wuerror.Transient
	}
	return false
}
