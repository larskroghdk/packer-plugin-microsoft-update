package wuerror

import (
	"fmt"
	"strconv"
	"strings"
)

type Class string

const (
	Transient      Class = "transient"
	Permanent      Class = "permanent"
	Unsupported    Class = "unsupported"
	RebootRequired Class = "reboot-required"
	NotApplicable  Class = "not-applicable"
)

type Info struct {
	Code        uint32
	Name        string
	Description string
	Class       Class
	Hint        string
}

const legacyPrerequisiteHint = "On Windows 7 SP1, Windows Server 2008 R2, and Windows Server 2012, install the SHA-2 code signing " +
	"and servicing stack prerequisites (for example KB4474419 and KB4490628 on Windows 7) and ensure TLS 1.2 is available to WinHTTP. " +
	"Updates published after end of support also require a valid Extended Security Updates license."

var known = map[uint32]Info{}

func register(code uint32, name string, class Class, description, hint string) {
	known[code] = Info{Code: code, Name: name, Description: description, Class: class, Hint: hint}
}

func init() {
	register(0x00000000, "S_OK", "", "The operation completed successfully.", "")
	register(0x00240005, "WU_S_REBOOT_REQUIRED", RebootRequired, "The update requires a restart to complete.", "")
	register(0x00240006, "WU_S_ALREADY_INSTALLED", NotApplicable, "The update is already installed.", "")
	register(0x00000BC2, "ERROR_SUCCESS_REBOOT_REQUIRED", RebootRequired, "The operation succeeded and a restart is required.", "")

	register(0x80240001, "WU_E_NO_SERVICE", Transient, "The Windows Update Agent was unable to provide the service.", "")
	register(0x8024000B, "WU_E_CALL_CANCELLED", Transient, "The operation was cancelled.", "")
	register(0x8024000E, "WU_E_XML_INVALID", Transient, "The Windows Update Agent found invalid information in the update metadata.", "")
	register(0x80240013, "WU_E_DUPLICATE_ITEM", Transient, "The same update was added to a collection twice.", "The update is retried in the next pass without the duplicate.")
	register(0x80240016, "WU_E_INSTALL_NOT_ALLOWED", Transient, "Another installation is in progress or a restart is pending.", "")
	register(0x80240017, "WU_E_NOT_APPLICABLE", NotApplicable, "The update is not applicable to this system.", "")
	register(0x8024001E, "WU_E_SERVICE_STOP", Transient, "The Windows Update service stopped while the operation was in progress.", "")
	register(0x80240020, "WU_E_NO_INTERACTIVE_USER", Permanent, "The operation requires an interactive user.", "Exclude the update or install it with an interactive session.")
	register(0x80240022, "WU_E_ALL_UPDATES_FAILED", Permanent, "Every update in the operation failed.", "Review the per-update result codes.")
	register(0x8024002E, "WU_E_WU_DISABLED", Permanent, "Access to Windows Update is disabled by policy.", "Review the DisableWindowsUpdateAccess policy or select update_source = \"wsus\".")
	register(0x80240032, "WU_E_INVALID_CRITERIA", Permanent, "The search criteria string is invalid.", "Fix search_criteria; see the IUpdateSearcher::Search documentation.")
	register(0x80240438, "WU_E_PT_ENDPOINT_UNREACHABLE", Transient, "The update service endpoint is unreachable.", "Check proxy and firewall access, and the DoNotConnectToWindowsUpdateInternetLocations policy.")
	register(0x80240FFF, "WU_E_UNEXPECTED", Transient, "The Windows Update Agent reported an unexpected failure.", "")
	register(0x8024200D, "WU_E_UH_NEEDANOTHERDOWNLOAD", Transient, "The update needs to be downloaded again.", "")
	register(0x80242016, "WU_E_UH_POSTREBOOTUNEXPECTEDSTATE", Transient, "The update was in an unexpected state after the restart.", "")
	register(0x80242014, "WU_E_UH_POSTREBOOTSTILLPENDING", RebootRequired, "The post-restart operation of the update is still in progress.", "")
	register(0x80246007, "WU_E_DM_NOTDOWNLOADED", Transient, "The update has not been downloaded.", "")
	register(0x80246008, "WU_E_DM_BITSTRANSFERERROR", Transient, "BITS failed to transfer the update content.", "")
	register(0x8024A10A, "WU_E_AU_SERVICE_SHUTTING_DOWN", Transient, "The Windows Update service is shutting down.", "")

	register(0x8024401C, "WU_E_PT_HTTP_STATUS_REQUEST_TIMEOUT", Transient, "The update server timed out.", "")
	register(0x8024402C, "WU_E_PT_WINHTTP_NAME_NOT_RESOLVED", Transient, "The update server name could not be resolved.", "Check DNS and proxy configuration.")
	register(0x8024402F, "WU_E_PT_ECP_SUCCEEDED_WITH_ERRORS", Transient, "External cab file processing completed with errors.", "")
	register(0x80244010, "WU_E_PT_EXCEEDED_MAX_SERVER_TRIPS", Transient, "The number of round trips to the server exceeded the maximum.", "")
	register(0x80244017, "WU_E_PT_HTTP_STATUS_DENIED", Permanent, "The update server or proxy denied access (HTTP 401).", "Configure proxy authentication for the SYSTEM account or use a transparent proxy.")
	register(0x80244018, "WU_E_PT_HTTP_STATUS_FORBIDDEN", Permanent, "The update server or proxy refused the request (HTTP 403).", "Check proxy rules for the Windows Update endpoints.")
	register(0x80244019, "WU_E_PT_HTTP_STATUS_NOT_FOUND", Unsupported, "The update service endpoint was not found (HTTP 404).", "The update service may be retired for this operating system. "+legacyPrerequisiteHint)
	register(0x80244022, "WU_E_PT_HTTP_STATUS_SERVICE_UNAVAIL", Transient, "The update service is temporarily unavailable (HTTP 503).", "")
	register(0x8024500C, "WU_E_REDIRECTOR_CONNECT_POLICY", Permanent, "Policy prevents connecting to Windows Update.", "Review the DoNotConnectToWindowsUpdateInternetLocations policy or select update_source = \"wsus\".")
	register(0x80072EE2, "ERROR_INTERNET_TIMEOUT", Transient, "The connection to the update service timed out.", "")
	register(0x80072EE7, "ERROR_INTERNET_NAME_NOT_RESOLVED", Transient, "The update server name could not be resolved.", "Check DNS and proxy configuration.")
	register(0x80072EFD, "ERROR_INTERNET_CANNOT_CONNECT", Transient, "A connection to the update service could not be established.", "Check network connectivity, proxy, and firewall rules.")
	register(0x80072EFE, "ERROR_INTERNET_CONNECTION_ABORTED", Transient, "The connection to the update service was aborted.", "")
	register(0x80072F8F, "ERROR_INTERNET_SECURE_FAILURE", Unsupported, "A TLS or certificate validation error occurred while connecting to the update service.", "Check the system clock and root certificates. "+legacyPrerequisiteHint)
	register(0x80072F0C, "ERROR_INTERNET_CLIENT_AUTH_CERT_NEEDED", Permanent, "The update server requires a client certificate.", "")

	register(0x800F0821, "CBS_E_ABORT", Transient, "The servicing operation was aborted.", "")
	register(0x800F081E, "CBS_E_NOT_APPLICABLE", NotApplicable, "The package is not applicable to this system.", "")
	register(0x800F081F, "CBS_E_SOURCE_MISSING", Permanent, "The servicing source files could not be found.", "Repair the component store with DISM /Online /Cleanup-Image /RestoreHealth before building the template.")
	register(0x800F0823, "CBS_E_NEW_SERVICING_STACK_REQUIRED", Transient, "The package requires a newer servicing stack.", "The servicing stack update is installed first; the package is retried in a later pass.")
	register(0x800F0831, "CBS_E_STORE_CORRUPTION", Permanent, "The component store is missing a required manifest.", "Repair the component store with DISM /Online /Cleanup-Image /RestoreHealth.")
	register(0x800F0922, "CBS_E_INSTALLERS_FAILED", Permanent, "Processing advanced installers and generic commands failed.", "Check free space on the System Reserved or EFI partition and review CBS.log.")
	register(0x800F0986, "PSFX_E_APPLY_FORWARD_DELTA_FAILED", Permanent, "Applying a forward delta failed.", "Review CBS.log; the component store may need repair.")
	register(0x80073712, "ERROR_SXS_COMPONENT_STORE_CORRUPT", Permanent, "The component store is corrupt.", "Repair the component store with DISM /Online /Cleanup-Image /RestoreHealth.")

	register(0x80070005, "E_ACCESSDENIED", Permanent, "Access is denied.", "The Windows Update Agent must run elevated; check the username setting.")
	register(0x8007000E, "E_OUTOFMEMORY", Transient, "Not enough memory to complete the operation.", "")
	register(0x80070070, "ERROR_DISK_FULL", Permanent, "There is not enough free disk space.", "Increase the system disk size of the build VM.")
	register(0x80070422, "ERROR_SERVICE_DISABLED", Permanent, "A required service is disabled.", "Enable the Windows Update (wuauserv) and BITS services before running the provisioner.")
	register(0x80070643, "ERROR_INSTALL_FAILURE", Permanent, "The installer reported a fatal error.", "Review the setup logs of the affected product.")
	register(0x80070652, "ERROR_INSTALL_ALREADY_RUNNING", Transient, "Another installation is already in progress.", "")
	register(0x80070BC9, "ERROR_FAIL_REBOOT_REQUIRED", RebootRequired, "The operation requires a restart to complete a previous installation.", "")
	register(0x80070BC2, "ERROR_SUCCESS_REBOOT_REQUIRED", RebootRequired, "The operation succeeded and a restart is required.", "")
	register(0x800706BE, "RPC_S_CALL_FAILED", Transient, "The remote procedure call failed.", "")
	register(0x800706BA, "RPC_S_SERVER_UNAVAILABLE", Transient, "The RPC server is unavailable.", "")
	register(0x80010108, "RPC_E_DISCONNECTED", Transient, "The COM object disconnected from its clients.", "")
}

func Lookup(code uint32) Info {
	if info, ok := known[code]; ok {
		return info
	}
	class := Permanent
	if facility(code) == 0x24 {
		class = Transient
	}
	return Info{Code: code, Name: Format(code), Description: "Unrecognized result code.", Class: class}
}

func IsSuccess(code uint32) bool {
	return code&0x80000000 == 0
}

func Format(code uint32) string {
	return fmt.Sprintf("0x%08X", code)
}

func Parse(value string) (uint32, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("empty HRESULT")
	}
	if strings.HasPrefix(value, "0x") || strings.HasPrefix(value, "0X") {
		parsed, err := strconv.ParseUint(value[2:], 16, 32)
		if err != nil {
			return 0, fmt.Errorf("invalid HRESULT %q: %w", value, err)
		}
		return uint32(parsed), nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid HRESULT %q: %w", value, err)
	}
	if parsed < -1<<31 || parsed > 1<<32-1 {
		return 0, fmt.Errorf("HRESULT %q is out of range", value)
	}
	return uint32(parsed), nil
}

func Describe(code uint32) string {
	info := Lookup(code)
	message := fmt.Sprintf("%s (%s): %s", Format(code), info.Name, info.Description)
	if info.Name == Format(code) {
		message = fmt.Sprintf("%s: %s", Format(code), info.Description)
	}
	if info.Hint != "" {
		message += " " + info.Hint
	}
	return message
}

func facility(code uint32) uint32 {
	return (code >> 16) & 0x1FFF
}

type HResult uint32

func (h HResult) String() string { return Format(uint32(h)) }

func (h HResult) Info() Info { return Lookup(uint32(h)) }

func (h HResult) MarshalText() ([]byte, error) { return []byte(h.String()), nil }

func (h *HResult) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		*h = 0
		return nil
	}
	parsed, err := Parse(string(text))
	if err != nil {
		return err
	}
	*h = HResult(parsed)
	return nil
}
