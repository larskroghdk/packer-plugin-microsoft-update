package guest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const runnerMarker = ".runner.ps1"

func isBootstrap(command string) bool {
	return strings.Contains(decodeEncodedCommand(command), `"type":"bootstrap"`) || strings.Contains(decodeEncodedCommand(command), "bootstrap")
}

func isBootQuery(command string) bool {
	return strings.Contains(decodeEncodedCommand(command), `"type":"boot"`)
}

func TestPrepareAndDetect(t *testing.T) {
	communicator := newFakeCommunicator(func(communicator *fakeCommunicator, command string) fakeResponse {
		switch {
		case isBootstrap(command):
			return bootstrapResponse("20260101000000.000000+000")
		case strings.Contains(command, runnerMarker):
			request := communicator.requestFor(command)
			if request["action"][0] != "detect" {
				t.Errorf("unexpected action %v", request["action"])
			}
			return runnerResponse(
				`{"type":"log","level":"info","message":"hello"}`,
				`{"type":"system","caption":"Microsoft Windows Server 2025 Datacenter","build":26100,"update_build_revision":2033,"powershell_version":"5.1.26100.1","wsus_configured":true,"pending_reboot":{"pending":true,"reasons":["component-based-servicing"],"informational_reasons":[]},"boot_time":"20260101000000.000000+000","missing_prerequisites":[]}`,
				`{"type":"done","action":"detect"}`)
		}
		return fakeResponse{}
	})
	client, logger := newTestClient(communicator)
	if err := client.Prepare(context.Background()); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if want := `C:\Windows\Temp\packer-microsoft-update-` + client.runID; client.workDirectory != want {
		t.Fatalf("work directory %q, want %q", client.workDirectory, want)
	}
	system, err := client.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if system.Build != 26100 || system.UpdateBuildRevision != 2033 || !system.WSUSConfigured {
		t.Fatalf("unexpected system %+v", system.System)
	}
	if !system.PendingReboot.Pending || system.PendingReboot.Reasons[0] != "component-based-servicing" {
		t.Fatalf("unexpected pending reboot %+v", system.PendingReboot)
	}
	if !logger.contains("info: hello") {
		t.Fatalf("guest log was not forwarded: %v", logger.messages)
	}
	scriptPath := uploadPath(client.guestPath("microsoft-update.ps1"))
	if !bytes.Equal(communicator.uploads[scriptPath], updateScript) {
		t.Fatalf("guest script was not uploaded to %s", scriptPath)
	}
}

func TestRunnerDoesNotContainPasswordForSystem(t *testing.T) {
	communicator := newFakeCommunicator(func(communicator *fakeCommunicator, command string) fakeResponse {
		if isBootstrap(command) {
			return bootstrapResponse("1")
		}
		return runnerResponse(`{"type":"registration","state":"registered","changed":false,"hresult":"0x00000000"}`, `{"type":"done"}`)
	})
	client, _ := newTestClient(communicator)
	mustPrepare(t, client)
	if _, err := client.RegisterMicrosoftUpdate(context.Background()); err != nil {
		t.Fatalf("RegisterMicrosoftUpdate: %v", err)
	}
	for path, content := range communicator.uploads {
		if strings.HasSuffix(path, runnerMarker) {
			if !strings.Contains(string(content), "$taskPassword = $null") || !strings.Contains(string(content), "$taskLogonType = 5") {
				t.Fatalf("SYSTEM runner must use a service account logon without a password:\n%s", content)
			}
		}
	}
}

func TestRunnerQuotesPassword(t *testing.T) {
	communicator := newFakeCommunicator(func(communicator *fakeCommunicator, command string) fakeResponse {
		if isBootstrap(command) {
			return bootstrapResponse("1")
		}
		return runnerResponse(`{"type":"registration","state":"registered","changed":false,"hresult":"0x00000000"}`, `{"type":"done"}`)
	})
	client, _ := newTestClient(communicator)
	client.options.Username = "packer"
	client.options.Password = `it's"$(evil)`
	mustPrepare(t, client)
	if _, err := client.RegisterMicrosoftUpdate(context.Background()); err != nil {
		t.Fatalf("RegisterMicrosoftUpdate: %v", err)
	}
	for path, content := range communicator.uploads {
		if strings.HasSuffix(path, runnerMarker) && !strings.Contains(string(content), `$taskPassword = 'it''s"$(evil)'`) {
			t.Fatalf("password is not safely quoted:\n%s", content)
		}
	}
}

func TestSearchDecodesUpdates(t *testing.T) {
	communicator := newFakeCommunicator(func(communicator *fakeCommunicator, command string) fakeResponse {
		if isBootstrap(command) {
			return bootstrapResponse("1")
		}
		request := communicator.requestFor(command)
		if request["source"][0] != "microsoft-update" || request["legacy_filter"][0] != "include:$true" {
			t.Errorf("unexpected request %v", request)
		}
		return runnerResponse(
			`{"type":"phase","name":"search"}`,
			`{"type":"update","id":"11111111-1111-1111-1111-111111111111","title":"Security Update","kbs":["KB5000001"],"categories":[{"id":"0fa1201d-4330-4fa8-8ae9-b877473b6441","name":"Security Updates","type":"UpdateClassification"}],"legacy_filter_match":true}`,
			`{"type":"search","result_code":2,"result":"succeeded","update_count":1,"hresult":"0x00000000"}`,
			`{"type":"done"}`)
	})
	client, _ := newTestClient(communicator)
	mustPrepare(t, client)
	result, err := client.Search(context.Background(), SearchRequest{Source: SourceMicrosoftUpdate, Criteria: "IsInstalled=0", LegacyFilters: []string{"include:$true"}})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !result.Succeeded() || len(result.Updates) != 1 || result.Updates[0].KBs[0] != "KB5000001" {
		t.Fatalf("unexpected search result %+v", result)
	}
	if match := result.Updates[0].LegacyFilterMatch; match == nil || !*match {
		t.Fatalf("legacy filter match was not decoded")
	}
}

func TestInstallDecodesResults(t *testing.T) {
	communicator := newFakeCommunicator(func(communicator *fakeCommunicator, command string) fakeResponse {
		if isBootstrap(command) {
			return bootstrapResponse("1")
		}
		if got := communicator.requestFor(command)["update_id"]; len(got) != 2 {
			t.Errorf("expected two update IDs, got %v", got)
		}
		return runnerResponse(
			`{"type":"search","result_code":2,"result":"succeeded","update_count":2,"hresult":"0x00000000"}`,
			`{"type":"update-result","id":"a","phase":"download","result_code":2,"result":"succeeded","hresult":"0x00000000"}`,
			`{"type":"update-result","id":"a","phase":"install","result_code":2,"result":"succeeded","hresult":"0x00000000","reboot_required":true}`,
			`{"type":"update-result","id":"b","phase":"install","result_code":4,"result":"failed","hresult":"0x80070643"}`,
			`{"type":"install","result_code":3,"result":"succeeded-with-errors","hresult":"0x00000000","reboot_required":true}`,
			`{"type":"done"}`)
	})
	client, _ := newTestClient(communicator)
	mustPrepare(t, client)
	result, err := client.Install(context.Background(), InstallRequest{SearchRequest: SearchRequest{Source: SourceDefault}, UpdateIDs: []string{"a", "b"}})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(result.Updates) != 3 || !result.RebootRequired || result.Updates[2].HResult != 0x80070643 {
		t.Fatalf("unexpected install result %+v", result)
	}
}

func TestScriptErrorIsReported(t *testing.T) {
	communicator := newFakeCommunicator(func(communicator *fakeCommunicator, command string) fakeResponse {
		if isBootstrap(command) {
			return bootstrapResponse("1")
		}
		return fakeResponse{lines: []string{
			recordPrefix + `{"type":"error","hresult":"0x80070005","message":"Access is denied."}`,
			runnerPrefix + `{"exit_code":1}`,
		}}
	})
	client, _ := newTestClient(communicator)
	mustPrepare(t, client)
	_, err := client.Detect(context.Background())
	var scriptError *ScriptError
	if !errors.As(err, &scriptError) || scriptError.HResult != 0x80070005 {
		t.Fatalf("expected ScriptError with E_ACCESSDENIED, got %v", err)
	}
	if IsRetryable(err) {
		t.Fatalf("access denied must not be retried")
	}
	if !strings.Contains(err.Error(), "must run elevated") {
		t.Fatalf("error lacks the actionable hint: %v", err)
	}
}

func TestMissingRunnerFooterIsCommunicationError(t *testing.T) {
	communicator := newFakeCommunicator(func(communicator *fakeCommunicator, command string) fakeResponse {
		if isBootstrap(command) {
			return bootstrapResponse("1")
		}
		return fakeResponse{lines: []string{recordPrefix + `{"type":"done"}`}, exitStatus: 1}
	})
	client, _ := newTestClient(communicator)
	mustPrepare(t, client)
	_, err := client.Detect(context.Background())
	var communicationError *CommunicationError
	if !errors.As(err, &communicationError) || !IsRetryable(err) {
		t.Fatalf("expected retryable CommunicationError, got %v", err)
	}
}

func TestPhaseTimeoutAbortsTask(t *testing.T) {
	communicator := newFakeCommunicator(func(communicator *fakeCommunicator, command string) fakeResponse {
		switch {
		case isBootstrap(command):
			return bootstrapResponse("1")
		case strings.Contains(command, runnerMarker):
			return fakeResponse{lines: []string{recordPrefix + `{"type":"phase","name":"search"}`}, hang: true}
		}
		return fakeResponse{}
	})
	client, _ := newTestClient(communicator)
	client.options.Timeouts.Search = 50 * time.Millisecond
	mustPrepare(t, client)
	started := time.Now()
	_, err := client.Search(context.Background(), SearchRequest{Source: SourceDefault})
	var timeoutError *TimeoutError
	if !errors.As(err, &timeoutError) || timeoutError.Phase != "search" {
		t.Fatalf("expected search TimeoutError, got %v", err)
	}
	if time.Since(started) > 2*time.Second {
		t.Fatalf("timeout took too long: %s", time.Since(started))
	}
	if communicator.commandCount("DeleteTask") == 0 {
		t.Fatalf("the scheduled task was not aborted")
	}
}

func TestCancellationAbortsTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	communicator := newFakeCommunicator(func(communicator *fakeCommunicator, command string) fakeResponse {
		switch {
		case isBootstrap(command):
			return bootstrapResponse("1")
		case strings.Contains(command, runnerMarker):
			go func() {
				time.Sleep(20 * time.Millisecond)
				cancel()
			}()
			return fakeResponse{hang: true}
		}
		return fakeResponse{}
	})
	client, _ := newTestClient(communicator)
	client.options.Timeouts.Startup = time.Minute
	mustPrepare(t, client)
	_, err := client.Detect(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if communicator.commandCount("DeleteTask") == 0 {
		t.Fatalf("the scheduled task was not aborted after cancellation")
	}
}

func TestRestartWaitsForNewBootTime(t *testing.T) {
	var bootQueries atomic.Int32
	communicator := newFakeCommunicator(func(communicator *fakeCommunicator, command string) fakeResponse {
		switch {
		case isBootstrap(command):
			return bootstrapResponse("boot-1")
		case strings.HasPrefix(command, "shutdown.exe"):
			return fakeResponse{}
		case isBootQuery(command):
			switch bootQueries.Add(1) {
			case 1:
				return fakeResponse{lines: []string{recordPrefix + `{"type":"boot","boot_time":"boot-1"}`}}
			case 2:
				return fakeResponse{err: errors.New("connection refused")}
			default:
				return fakeResponse{lines: []string{recordPrefix + `{"type":"boot","boot_time":"boot-2"}`}}
			}
		}
		return fakeResponse{}
	})
	client, _ := newTestClient(communicator)
	mustPrepare(t, client)
	client.scriptUploaded = true
	if err := client.Restart(context.Background(), "test"); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if client.KnownBootID() != "boot-2" || client.scriptUploaded {
		t.Fatalf("restart state not updated: boot %q, script uploaded %v", client.KnownBootID(), client.scriptUploaded)
	}
	if bootQueries.Load() < 3 {
		t.Fatalf("expected at least three boot queries, got %d", bootQueries.Load())
	}
}

func TestRestartTimesOut(t *testing.T) {
	communicator := newFakeCommunicator(func(communicator *fakeCommunicator, command string) fakeResponse {
		switch {
		case isBootstrap(command):
			return bootstrapResponse("boot-1")
		case isBootQuery(command):
			return fakeResponse{lines: []string{recordPrefix + `{"type":"boot","boot_time":"boot-1"}`}}
		}
		return fakeResponse{}
	})
	client, _ := newTestClient(communicator)
	client.options.Timeouts.Reboot = 100 * time.Millisecond
	mustPrepare(t, client)
	err := client.Restart(context.Background(), "test")
	var timeoutError *TimeoutError
	if !errors.As(err, &timeoutError) || timeoutError.Phase != "restart" {
		t.Fatalf("expected restart TimeoutError, got %v", err)
	}
}

func mustPrepare(t *testing.T, client *Client) {
	t.Helper()
	if err := client.Prepare(context.Background()); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
}

func TestOfflinePackagesWithTheSameNameDoNotOverwriteEachOther(t *testing.T) {
	hostDirectory := t.TempDir()
	contents := map[string][]byte{
		filepath.Join(hostDirectory, "first", "ssu.msu"):  []byte("first package"),
		filepath.Join(hostDirectory, "second", "ssu.msu"): []byte("second package"),
	}
	var packages []OfflinePackage
	for hostPath, content := range contents {
		if err := os.MkdirAll(filepath.Dir(hostPath), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(hostPath, content, 0o600); err != nil {
			t.Fatal(err)
		}
		packages = append(packages, OfflinePackage{HostPath: hostPath})
	}
	var requested []string
	communicator := newFakeCommunicator(func(communicator *fakeCommunicator, command string) fakeResponse {
		switch {
		case isBootstrap(command):
			return bootstrapResponse("boot-1")
		case strings.Contains(command, runnerMarker):
			requested = communicator.requestFor(command)["offline_package"]
			return runnerResponse(
				`{"type":"pending-reboot","pending":false,"reasons":[],"informational_reasons":[]}`,
				`{"type":"done","action":"offline-install"}`)
		}
		return fakeResponse{}
	})
	client, _ := newTestClient(communicator)
	if err := client.Prepare(context.Background()); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if _, err := client.InstallOffline(context.Background(), packages); err != nil {
		t.Fatalf("InstallOffline: %v", err)
	}
	if len(requested) != 2 {
		t.Fatalf("expected two offline package requests, got %v", requested)
	}
	seen := map[string]bool{}
	for _, entry := range requested {
		guestPath, expectedSHA256, _ := strings.Cut(entry, "|")
		if seen[guestPath] {
			t.Fatalf("two packages share the guest path %s", guestPath)
		}
		seen[guestPath] = true
		uploaded := communicator.uploads[uploadPath(guestPath)]
		checksum := sha256.Sum256(uploaded)
		if hex.EncodeToString(checksum[:]) != expectedSHA256 {
			t.Errorf("the file uploaded to %s does not match its requested checksum", guestPath)
		}
		if !strings.HasSuffix(guestPath, "-ssu.msu") {
			t.Errorf("guest path %s lost the package file name", guestPath)
		}
	}
}
