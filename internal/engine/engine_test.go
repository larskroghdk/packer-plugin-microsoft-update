package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/larskroghdk/packer-plugin-microsoft-update/internal/guest"
	"github.com/larskroghdk/packer-plugin-microsoft-update/internal/report"
	"github.com/larskroghdk/packer-plugin-microsoft-update/internal/selection"
	"github.com/larskroghdk/packer-plugin-microsoft-update/internal/wuerror"
)

const (
	securityClassificationID = "0fa1201d-4330-4fa8-8ae9-b877473b6441"
	driverClassificationID   = "ebfc1fc5-71a4-4f7b-9aca-3b9a503104a0"
)

type fakeUpdate struct {
	update         selection.Update
	installsAfter  int
	installHResult wuerror.HResult
	needsReboot    bool
	requiresBoot   int
	neverGoesAway  bool
	firstSearch    int
}

type fakeGuest struct {
	system         guest.System
	updates        map[string]*fakeUpdate
	order          []string
	boot           int
	restarts       int
	searches       int
	searchFailures []error
	installCalls   [][]string
	registration   guest.Registration
	pendingAfter   []bool
	installed      map[string]bool
	attempts       map[string]int
	cancelOnSearch context.CancelFunc
	restartErr     error
	offline        guest.OfflineResult
	registrations  []guest.Registration
	registerCalls  int
	failSearchAt   map[int]error
	installErr     error
}

func newFakeGuest(updates ...*fakeUpdate) *fakeGuest {
	target := &fakeGuest{
		system:    guest.System{BootTime: "boot-0"},
		updates:   map[string]*fakeUpdate{},
		installed: map[string]bool{},
		attempts:  map[string]int{},
	}
	for _, update := range updates {
		target.updates[update.update.ID] = update
		target.order = append(target.order, update.update.ID)
	}
	return target
}

func securityUpdate(id, title string) *fakeUpdate {
	return &fakeUpdate{update: selection.Update{
		ID:         id,
		Title:      title,
		KBs:        []string{"KB" + strings.Repeat(id[:1], 7)},
		Categories: []selection.Category{{ID: securityClassificationID, Name: "Security Updates", Type: "UpdateClassification"}},
	}}
}

func (f *fakeGuest) Detect(context.Context) (guest.System, error) { return f.system, nil }

func (f *fakeGuest) RegisterMicrosoftUpdate(context.Context) (guest.Registration, error) {
	f.registerCalls++
	if len(f.registrations) > 0 {
		registration := f.registrations[0]
		f.registrations = f.registrations[1:]
		return registration, nil
	}
	return f.registration, nil
}

func (f *fakeGuest) Search(ctx context.Context, _ guest.SearchRequest) (guest.SearchResult, error) {
	f.searches++
	if f.cancelOnSearch != nil {
		f.cancelOnSearch()
		return guest.SearchResult{}, ctx.Err()
	}
	if failure, found := f.failSearchAt[f.searches]; found {
		return guest.SearchResult{}, failure
	}
	if len(f.searchFailures) > 0 {
		failure := f.searchFailures[0]
		f.searchFailures = f.searchFailures[1:]
		return guest.SearchResult{}, failure
	}
	result := guest.SearchResult{ResultCode: 2, Result: "succeeded"}
	for _, id := range f.order {
		update := f.updates[id]
		if f.installed[id] && !update.neverGoesAway {
			continue
		}
		if update.requiresBoot > f.boot || update.firstSearch > f.searches {
			continue
		}
		result.Updates = append(result.Updates, update.update)
	}
	return result, nil
}

func (f *fakeGuest) Install(_ context.Context, request guest.InstallRequest) (guest.InstallResult, error) {
	f.installCalls = append(f.installCalls, request.UpdateIDs)
	if f.installErr != nil {
		return guest.InstallResult{}, f.installErr
	}
	result := guest.InstallResult{ResultCode: 2, Result: "succeeded"}
	for _, id := range request.UpdateIDs {
		update := f.updates[id]
		f.attempts[id]++
		if update.installsAfter > 0 && f.attempts[id] <= update.installsAfter || update.installHResult != 0 && update.installsAfter == 0 {
			code := update.installHResult
			if code == 0 {
				code = 0x80070643
			}
			result.Updates = append(result.Updates, guest.UpdateResult{ID: id, Phase: "install", ResultCode: 4, Result: "failed", HResult: code})
			continue
		}
		f.installed[id] = true
		result.Updates = append(result.Updates, guest.UpdateResult{ID: id, Phase: "install", ResultCode: 2, Result: "succeeded", RebootRequired: update.needsReboot})
		result.RebootRequired = result.RebootRequired || update.needsReboot
	}
	return result, nil
}

func (f *fakeGuest) InstallOffline(context.Context, []guest.OfflinePackage) (guest.OfflineResult, error) {
	return f.offline, nil
}

func (f *fakeGuest) PendingReboot(context.Context) (guest.PendingReboot, error) {
	if len(f.pendingAfter) > 0 {
		pending := f.pendingAfter[0]
		f.pendingAfter = f.pendingAfter[1:]
		return guest.PendingReboot{Pending: pending, Reasons: []string{"component-based-servicing"}}, nil
	}
	return guest.PendingReboot{}, nil
}

func (f *fakeGuest) Restart(context.Context, string) error {
	if f.restartErr != nil {
		return f.restartErr
	}
	f.restarts++
	f.boot++
	f.system.BootTime = fmt.Sprintf("boot-%d", f.boot)
	return nil
}

func (f *fakeGuest) BootID(context.Context) (string, error) { return f.system.BootTime, nil }
func (f *fakeGuest) KnownBootID() string                    { return f.system.BootTime }
func (f *fakeGuest) NoteRestart(string)                     {}

type quietLogger struct{ messages []string }

func (l *quietLogger) Info(message string)  { l.messages = append(l.messages, "info: "+message) }
func (l *quietLogger) Warn(message string)  { l.messages = append(l.messages, "warn: "+message) }
func (l *quietLogger) Debug(message string) { l.messages = append(l.messages, "debug: "+message) }

func defaultSettings() Settings {
	policy, _ := selection.ProfilePolicy(selection.ProfileRecommended)
	return Settings{
		Source:             guest.SourceDefault,
		SearchCriteria:     "IsInstalled=0",
		Policy:             policy,
		MaxUpdatePasses:    10,
		MaxReboots:         5,
		MaxUpdatesPerPass:  100,
		MaxInstallAttempts: 2,
		MaxRetries:         3,
		RetryDelay:         time.Millisecond,
	}
}

func runEngine(t *testing.T, target *fakeGuest, settings Settings) (*report.Report, *quietLogger) {
	t.Helper()
	logger := &quietLogger{}
	updateEngine := New(target, settings, logger, report.Plugin{Name: "test", Version: "0.0.0"})
	updateEngine.sleep = func(context.Context, time.Duration) error { return nil }
	return updateEngine.Run(context.Background()), logger
}

func stateOf(result *report.Report, id string) report.UpdateState {
	for _, update := range result.Updates {
		if update.ID == id {
			return update.State
		}
	}
	return ""
}

func TestNoUpdatesIsUpdated(t *testing.T) {
	result, _ := runEngine(t, newFakeGuest(), defaultSettings())
	if result.Status != report.StatusUpdated || result.Passes != 1 || result.Reboots != 0 {
		t.Fatalf("unexpected result: status %s, passes %d, reboots %d", result.Status, result.Passes, result.Reboots)
	}
}

func TestMultiplePassesWithReboots(t *testing.T) {
	first := securityUpdate("11111111-0000-0000-0000-000000000001", "Servicing stack")
	first.needsReboot = true
	second := securityUpdate("22222222-0000-0000-0000-000000000002", "Cumulative update")
	second.requiresBoot = 1
	second.needsReboot = true
	target := newFakeGuest(first, second)
	result, _ := runEngine(t, target, defaultSettings())
	if result.Status != report.StatusUpdated {
		t.Fatalf("status %s: %s", result.Status, result.StatusReason)
	}
	if target.restarts != 2 || result.Reboots != 2 || len(target.installCalls) != 2 {
		t.Fatalf("restarts %d, report reboots %d, install calls %v", target.restarts, result.Reboots, target.installCalls)
	}
	if stateOf(result, first.update.ID) != report.StateInstalled || stateOf(result, second.update.ID) != report.StateInstalled {
		t.Fatalf("updates not reported as installed: %+v", result.Updates)
	}
	if result.Summary.Installed != 2 {
		t.Fatalf("summary %+v", result.Summary)
	}
}

func TestPermanentlyFailingUpdateDoesNotLoop(t *testing.T) {
	failing := securityUpdate("33333333-0000-0000-0000-000000000003", "Broken update")
	failing.installHResult = 0x80070643
	target := newFakeGuest(failing)
	result, _ := runEngine(t, target, defaultSettings())
	if result.Status != report.StatusPartial {
		t.Fatalf("status %s, want partial", result.Status)
	}
	if got := target.attempts[failing.update.ID]; got != 2 {
		t.Fatalf("expected exactly max_install_attempts (2) attempts, got %d", got)
	}
	if stateOf(result, failing.update.ID) != report.StateFailed || result.Summary.Failed != 1 {
		t.Fatalf("failed update not reported: %+v", result.Updates)
	}
}

func TestUpdateThatSucceedsButIsOfferedAgainIsBounded(t *testing.T) {
	sticky := securityUpdate("44444444-0000-0000-0000-000000000004", "Defender platform")
	sticky.neverGoesAway = true
	target := newFakeGuest(sticky)
	result, _ := runEngine(t, target, defaultSettings())
	if len(target.installCalls) != 2 {
		t.Fatalf("expected two install attempts, got %d", len(target.installCalls))
	}
	if result.Status == report.StatusUpdated {
		t.Fatalf("an update that stays applicable must not be reported as updated")
	}
}

func TestTransientUpdateFailureRecoversInLaterPass(t *testing.T) {
	flaky := securityUpdate("55555555-0000-0000-0000-000000000005", "Flaky download")
	flaky.installsAfter = 1
	flaky.installHResult = 0x80246008
	target := newFakeGuest(flaky)
	result, _ := runEngine(t, target, defaultSettings())
	if result.Status != report.StatusUpdated || stateOf(result, flaky.update.ID) != report.StateInstalled {
		t.Fatalf("status %s, update state %s", result.Status, stateOf(result, flaky.update.ID))
	}
	if updates := result.Updates; len(updates[0].Attempts) != 2 {
		t.Fatalf("expected two recorded attempts, got %+v", updates[0].Attempts)
	}
}

func TestMaxRebootsStopsTheRun(t *testing.T) {
	var updates []*fakeUpdate
	for index := 0; index < 4; index++ {
		update := securityUpdate(fmt.Sprintf("%d6666666-0000-0000-0000-00000000000%d", index, index), fmt.Sprintf("Chained %d", index))
		update.requiresBoot = index
		update.needsReboot = true
		updates = append(updates, update)
	}
	settings := defaultSettings()
	settings.MaxReboots = 2
	target := newFakeGuest(updates...)
	result, _ := runEngine(t, target, settings)
	if result.Status != report.StatusPartial || target.restarts != 2 {
		t.Fatalf("status %s, restarts %d", result.Status, target.restarts)
	}
	if !strings.Contains(result.StatusReason, "max_reboots") {
		t.Fatalf("reason does not mention max_reboots: %s", result.StatusReason)
	}
}

func TestMaxUpdatePassesStopsTheRun(t *testing.T) {
	sticky := securityUpdate("77777777-0000-0000-0000-000000000007", "Sticky")
	sticky.neverGoesAway = true
	settings := defaultSettings()
	settings.MaxUpdatePasses = 1
	settings.MaxInstallAttempts = 5
	result, _ := runEngine(t, newFakeGuest(sticky), settings)
	if result.Status != report.StatusPartial || !strings.Contains(result.StatusReason, "max_update_passes") {
		t.Fatalf("status %s: %s", result.Status, result.StatusReason)
	}
	if stateOf(result, sticky.update.ID) != report.StateRemaining || result.Summary.Remaining != 1 {
		t.Fatalf("remaining update not reported: %+v", result.Updates)
	}
}

func TestTransientSearchErrorIsRetried(t *testing.T) {
	target := newFakeGuest()
	target.searchFailures = []error{
		&SearchError{Result: "failed", HResult: 0x8024402C},
		&guest.CommunicationError{Operation: "search", Err: errors.New("connection reset")},
	}
	result, logger := runEngine(t, target, defaultSettings())
	if result.Status != report.StatusUpdated || target.searches != 3 {
		t.Fatalf("status %s after %d searches", result.Status, target.searches)
	}
	if !containsMessage(logger, "retrying") {
		t.Fatalf("retries were not logged")
	}
}

func TestPermanentSearchErrorFails(t *testing.T) {
	target := newFakeGuest()
	target.searchFailures = []error{&SearchError{Result: "failed", HResult: 0x80240032}}
	result, _ := runEngine(t, target, defaultSettings())
	if result.Status != report.StatusFailed || target.searches != 1 {
		t.Fatalf("status %s after %d searches", result.Status, target.searches)
	}
	if len(result.Errors) != 1 || result.Errors[0].HResult != "0x80240032" {
		t.Fatalf("error not recorded: %+v", result.Errors)
	}
}

func TestRetiredServiceIsUnsupported(t *testing.T) {
	target := newFakeGuest()
	target.searchFailures = []error{&SearchError{Result: "failed", HResult: 0x80244019}}
	result, _ := runEngine(t, target, defaultSettings())
	if result.Status != report.StatusUnsupported {
		t.Fatalf("status %s, want unsupported", result.Status)
	}
	if !strings.Contains(result.StatusReason, "SHA-2") {
		t.Fatalf("reason lacks the legacy prerequisite hint: %s", result.StatusReason)
	}
}

func TestRetriesAreBounded(t *testing.T) {
	target := newFakeGuest()
	for index := 0; index < 10; index++ {
		target.searchFailures = append(target.searchFailures, &SearchError{Result: "failed", HResult: 0x80072EFD})
	}
	settings := defaultSettings()
	settings.MaxRetries = 2
	result, _ := runEngine(t, target, settings)
	if result.Status != report.StatusFailed || target.searches != 3 {
		t.Fatalf("status %s after %d searches, want failed after 3", result.Status, target.searches)
	}
}

func TestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	target := newFakeGuest()
	target.cancelOnSearch = cancel
	updateEngine := New(target, defaultSettings(), &quietLogger{}, report.Plugin{})
	result := updateEngine.Run(ctx)
	if result.Status != report.StatusCancelled {
		t.Fatalf("status %s, want cancelled", result.Status)
	}
}

func TestWSUSWithoutPolicyIsUnsupported(t *testing.T) {
	settings := defaultSettings()
	settings.Source = guest.SourceWSUS
	target := newFakeGuest()
	result, _ := runEngine(t, target, settings)
	if result.Status != report.StatusUnsupported || target.searches != 0 {
		t.Fatalf("status %s, searches %d", result.Status, target.searches)
	}
}

func TestPolicyBlocksExplicitWindowsUpdate(t *testing.T) {
	settings := defaultSettings()
	settings.Source = guest.SourceWindowsUpdate
	target := newFakeGuest()
	target.system.WindowsUpdateAccessDisabled = true
	result, _ := runEngine(t, target, settings)
	if result.Status != report.StatusUnsupported || !strings.Contains(result.StatusReason, "does not override policy") {
		t.Fatalf("status %s: %s", result.Status, result.StatusReason)
	}
}

func TestExplicitSourceWithWSUSPolicyWarns(t *testing.T) {
	settings := defaultSettings()
	settings.Source = guest.SourceWindowsUpdate
	target := newFakeGuest()
	target.system.WSUSConfigured = true
	result, _ := runEngine(t, target, settings)
	if result.Status != report.StatusUpdated || len(result.Source.Warnings) != 1 || !result.Source.PolicyManaged {
		t.Fatalf("status %s, source %+v", result.Status, result.Source)
	}
}

func TestDefaultSourceResolvesToWSUS(t *testing.T) {
	target := newFakeGuest()
	target.system.WSUSConfigured = true
	result, _ := runEngine(t, target, defaultSettings())
	if result.Source.Effective != "wsus" || len(result.Source.Warnings) != 0 {
		t.Fatalf("source %+v", result.Source)
	}
}

func TestDisabledUpdateServiceIsUnsupported(t *testing.T) {
	target := newFakeGuest()
	target.system.UpdateServiceStartMode = "Disabled"
	result, _ := runEngine(t, target, defaultSettings())
	if result.Status != report.StatusUnsupported {
		t.Fatalf("status %s", result.Status)
	}
}

func TestPendingRebootBeforeUpdating(t *testing.T) {
	target := newFakeGuest()
	target.system.PendingReboot = guest.PendingReboot{Pending: true, Reasons: []string{"component-based-servicing"}}
	target.pendingAfter = []bool{true, false}
	result, _ := runEngine(t, target, defaultSettings())
	if target.restarts != 2 || result.Status != report.StatusUpdated {
		t.Fatalf("restarts %d, status %s", target.restarts, result.Status)
	}
}

func TestMicrosoftUpdateRegistration(t *testing.T) {
	settings := defaultSettings()
	settings.Source = guest.SourceMicrosoftUpdate
	target := newFakeGuest()
	target.registration = guest.Registration{State: "pending", Changed: true}
	result, _ := runEngine(t, target, settings)
	if target.restarts != 1 || result.Status != report.StatusUpdated || result.Source.ServiceID != guest.MicrosoftUpdateServiceID {
		t.Fatalf("restarts %d, status %s, source %+v", target.restarts, result.Status, result.Source)
	}

	target = newFakeGuest()
	target.registration = guest.Registration{State: "unsupported", Message: "no AddService2"}
	result, _ = runEngine(t, target, settings)
	if result.Status != report.StatusUnsupported {
		t.Fatalf("status %s", result.Status)
	}
}

func TestFiltersAndReporting(t *testing.T) {
	security := securityUpdate("88888888-0000-0000-0000-000000000008", "Security")
	driver := &fakeUpdate{update: selection.Update{
		ID: "99999999-0000-0000-0000-000000000009", Title: "Driver", Type: "driver",
		Categories: []selection.Category{{ID: driverClassificationID, Type: "UpdateClassification"}},
	}}
	excluded := securityUpdate("aaaaaaaa-0000-0000-0000-00000000000a", "Excluded")
	settings := defaultSettings()
	settings.Policy.ExcludeUpdateIDs = []string{excluded.update.ID}
	target := newFakeGuest(security, driver, excluded)
	result, _ := runEngine(t, target, settings)
	if result.Status != report.StatusUpdated {
		t.Fatalf("status %s: %s", result.Status, result.StatusReason)
	}
	if stateOf(result, driver.update.ID) != report.StateSkipped || stateOf(result, excluded.update.ID) != report.StateExcluded {
		t.Fatalf("unexpected states %+v", result.Updates)
	}
	if result.Summary.Skipped != 1 || result.Summary.Excluded != 1 || result.Summary.Installed != 1 {
		t.Fatalf("summary %+v", result.Summary)
	}
	if len(target.installCalls) != 1 || len(target.installCalls[0]) != 1 {
		t.Fatalf("install calls %v", target.installCalls)
	}
}

func TestMaxUpdatesPerPassSplitsBatches(t *testing.T) {
	first := securityUpdate("bbbbbbbb-0000-0000-0000-00000000000b", "First")
	second := securityUpdate("cccccccc-0000-0000-0000-00000000000c", "Second")
	settings := defaultSettings()
	settings.MaxUpdatesPerPass = 1
	target := newFakeGuest(first, second)
	result, _ := runEngine(t, target, settings)
	if result.Status != report.StatusUpdated || len(target.installCalls) != 2 {
		t.Fatalf("status %s, install calls %v", result.Status, target.installCalls)
	}
}

func TestOfflineOnly(t *testing.T) {
	settings := defaultSettings()
	settings.Source = guest.SourceOffline
	settings.OfflinePackages = []guest.OfflinePackage{{GuestPath: `D:\kb.msu`}}
	target := newFakeGuest()
	target.offline = guest.OfflineResult{Packages: []guest.OfflinePackageResult{{Name: "kb.msu", Result: "installed", ExitCode: 3010, RebootRequired: true}}}
	result, _ := runEngine(t, target, settings)
	if result.Status != report.StatusUpdated || target.restarts != 1 || target.searches != 0 {
		t.Fatalf("status %s, restarts %d, searches %d", result.Status, target.restarts, target.searches)
	}

	target = newFakeGuest()
	target.offline = guest.OfflineResult{Packages: []guest.OfflinePackageResult{{Name: "kb.msu", Result: "rejected", Message: "not signed by Microsoft"}}}
	result, _ = runEngine(t, target, settings)
	if result.Status != report.StatusPartial || len(result.Errors) != 1 {
		t.Fatalf("status %s, errors %+v", result.Status, result.Errors)
	}
}

func TestRestartFailureFails(t *testing.T) {
	update := securityUpdate("dddddddd-0000-0000-0000-00000000000d", "Needs restart")
	update.needsReboot = true
	target := newFakeGuest(update)
	target.restartErr = &guest.TimeoutError{Phase: "restart", Timeout: time.Hour}
	result, _ := runEngine(t, target, defaultSettings())
	if result.Status != report.StatusPartial || !strings.Contains(result.StatusReason, "restart") {
		t.Fatalf("status %s: %s", result.Status, result.StatusReason)
	}
}

func containsMessage(logger *quietLogger, substring string) bool {
	for _, message := range logger.messages {
		if strings.Contains(message, substring) {
			return true
		}
	}
	return false
}

func TestUpdatesFirstSeenInFinalValidationGetAState(t *testing.T) {
	sticky := securityUpdate("77777777-0000-0000-0000-000000000007", "Sticky")
	sticky.neverGoesAway = true
	lateDriver := &fakeUpdate{firstSearch: 2, update: selection.Update{
		ID:    "88888888-0000-0000-0000-000000000008",
		Title: "Late driver",
		Type:  "driver",
	}}
	lateExcluded := securityUpdate("99999999-0000-0000-0000-000000000009", "Late excluded")
	lateExcluded.firstSearch = 2
	settings := defaultSettings()
	settings.MaxUpdatePasses = 1
	settings.MaxInstallAttempts = 5
	settings.Policy.ExcludeKBs = []string{lateExcluded.update.KBs[0]}
	result, _ := runEngine(t, newFakeGuest(sticky, lateDriver, lateExcluded), settings)
	if got := stateOf(result, lateDriver.update.ID); got != report.StateSkipped {
		t.Errorf("late driver state %q, want %q", got, report.StateSkipped)
	}
	if got := stateOf(result, lateExcluded.update.ID); got != report.StateExcluded {
		t.Errorf("late excluded update state %q, want %q", got, report.StateExcluded)
	}
	if result.Summary.Skipped != 1 || result.Summary.Excluded != 1 || result.Summary.Discovered != 3 {
		t.Errorf("summary does not count late updates: %+v", result.Summary)
	}
}

func TestTransientRegistrationFailureIsRetried(t *testing.T) {
	settings := defaultSettings()
	settings.Source = guest.SourceMicrosoftUpdate
	target := newFakeGuest()
	target.registrations = []guest.Registration{
		{State: "failed", HResult: wuerror.HResult(0x80072EE2), Message: "timeout"},
		{State: "registered", Changed: true},
	}
	result, _ := runEngine(t, target, settings)
	if target.registerCalls != 2 || result.Status != report.StatusUpdated {
		t.Fatalf("register calls %d, status %s: %s", target.registerCalls, result.Status, result.StatusReason)
	}
}

func TestPermanentRegistrationFailureIsNotRetried(t *testing.T) {
	settings := defaultSettings()
	settings.Source = guest.SourceMicrosoftUpdate
	target := newFakeGuest()
	target.registration = guest.Registration{State: "failed", HResult: wuerror.HResult(0x80070005)}
	result, _ := runEngine(t, target, settings)
	if target.registerCalls != 1 || result.Status != report.StatusFailed {
		t.Fatalf("register calls %d, status %s: %s", target.registerCalls, result.Status, result.StatusReason)
	}
}

func TestFinalValidationSearchFailureIsReported(t *testing.T) {
	sticky := securityUpdate("77777777-0000-0000-0000-000000000007", "Sticky")
	sticky.neverGoesAway = true
	settings := defaultSettings()
	settings.MaxUpdatePasses = 1
	settings.MaxInstallAttempts = 5
	target := newFakeGuest(sticky)
	target.failSearchAt = map[int]error{2: &SearchError{Result: "failed", HResult: wuerror.HResult(0x80244019)}}
	result, _ := runEngine(t, target, settings)
	if result.Status != report.StatusUnsupported {
		t.Fatalf("status %s: %s", result.Status, result.StatusReason)
	}
}

func TestDeferredUpdatesAreReportedAsRemaining(t *testing.T) {
	first := securityUpdate("11111111-0000-0000-0000-000000000001", "Same title")
	second := securityUpdate("22222222-0000-0000-0000-000000000002", "Same title")
	third := securityUpdate("33333333-0000-0000-0000-000000000003", "Over the batch limit")
	settings := defaultSettings()
	settings.MaxUpdatesPerPass = 1
	target := newFakeGuest(first, second, third)
	target.installErr = &guest.ScriptError{Action: "install", HResult: wuerror.HResult(0x80070005), Message: "denied"}
	result, _ := runEngine(t, target, settings)
	for _, update := range []*fakeUpdate{second, third} {
		if got := stateOf(result, update.update.ID); got != report.StateRemaining {
			t.Errorf("deferred update %s state %q, want %q", update.update.Title, got, report.StateRemaining)
		}
	}
}
