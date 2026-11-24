package engine

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/larskroghdk/packer-plugin-microsoft-update/internal/guest"
	"github.com/larskroghdk/packer-plugin-microsoft-update/internal/report"
	"github.com/larskroghdk/packer-plugin-microsoft-update/internal/selection"
	"github.com/larskroghdk/packer-plugin-microsoft-update/internal/wuerror"
)

type Guest interface {
	Detect(ctx context.Context) (guest.System, error)
	RegisterMicrosoftUpdate(ctx context.Context) (guest.Registration, error)
	Search(ctx context.Context, request guest.SearchRequest) (guest.SearchResult, error)
	Install(ctx context.Context, request guest.InstallRequest) (guest.InstallResult, error)
	InstallOffline(ctx context.Context, packages []guest.OfflinePackage) (guest.OfflineResult, error)
	PendingReboot(ctx context.Context) (guest.PendingReboot, error)
	Restart(ctx context.Context, reason string) error
	BootID(ctx context.Context) (string, error)
	KnownBootID() string
	NoteRestart(bootID string)
}
type Settings struct {
	Source             string
	SearchCriteria     string
	LegacyFilters      []string
	Policy             selection.Policy
	ReportSettings     report.Settings
	OfflinePackages    []guest.OfflinePackage
	MaxUpdatePasses    int
	MaxReboots         int
	MaxUpdatesPerPass  int
	MaxInstallAttempts int
	MaxRetries         int
	RetryDelay         time.Duration
	MaxRetryDelay      time.Duration
}
type Engine struct {
	guest    Guest
	settings Settings
	logger   guest.Logger
	now      func() time.Time
	sleep    func(ctx context.Context, duration time.Duration) error
	report   *report.Report
	updates  map[string]*report.Update
	order    []string
	attempts map[string]int
	lastSeen map[string]int
	searches int
}

func New(target Guest, settings Settings, logger guest.Logger, plugin report.Plugin) *Engine {
	engine := &Engine{
		guest:    target,
		settings: settings,
		logger:   logger,
		now:      time.Now,
		sleep:    sleepContext,
		updates:  map[string]*report.Update{},
		attempts: map[string]int{},
		lastSeen: map[string]int{},
	}
	engine.report = report.New(plugin, engine.now())
	engine.report.Settings = settings.ReportSettings
	engine.report.Source.Requested = settings.Source
	return engine
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type outcome struct {
	status report.Status
	reason string
}

func (e *Engine) Run(ctx context.Context) *report.Report {
	result := e.run(ctx)
	if ctx.Err() != nil {
		result = outcome{report.StatusCancelled, "the build was cancelled"}
	}
	e.finishUpdateStates()
	e.report.Finish(e.now(), result.status, result.reason)
	return e.report
}

func (e *Engine) run(ctx context.Context) outcome {
	system, err := withRetry(ctx, e, "detect", func() (guest.System, error) { return e.guest.Detect(ctx) })
	if err != nil {
		return e.fatal("detect", err)
	}
	e.report.System = &system.System
	e.describeSystem(system)
	if blocked := e.checkSource(system); blocked != nil {
		return *blocked
	}
	if system.PendingReboot.Pending {
		e.logger.Info(fmt.Sprintf("A restart is already pending (%s).", strings.Join(system.PendingReboot.Reasons, ", ")))
		if result := e.restart(ctx, "pending restart before updating"); result != nil {
			return *result
		}
	}
	if e.settings.Source == guest.SourceMicrosoftUpdate && !system.MicrosoftUpdateRegistered {
		if result := e.registerMicrosoftUpdate(ctx); result != nil {
			return *result
		}
	}
	if len(e.settings.OfflinePackages) > 0 {
		if result := e.installOffline(ctx); result != nil {
			return *result
		}
	}
	if e.settings.Source == guest.SourceOffline {
		return e.offlineOutcome()
	}
	return e.updateLoop(ctx)
}

func (e *Engine) describeSystem(system guest.System) {
	build := fmt.Sprintf("%d", system.Build)
	if system.UpdateBuildRevision > 0 {
		build += fmt.Sprintf(".%d", system.UpdateBuildRevision)
	}
	e.logger.Info(fmt.Sprintf("Guest: %s %s (build %s, %s, %s), Windows PowerShell %s, Windows Update Agent %s.",
		system.Caption, system.DisplayVersion, build, system.Architecture, system.InstallationType,
		system.PowerShellVersion, system.WindowsUpdateAgent))
	if system.InsiderBranch != "" {
		e.logger.Info(fmt.Sprintf("Windows Insider branch: %s.", system.InsiderBranch))
	}
	if system.LifecycleNotice != "" {
		e.logger.Warn(system.LifecycleNotice)
	}
	for _, missing := range system.MissingPrerequisites {
		e.logger.Warn("Prerequisite: " + missing)
	}
}

func (e *Engine) checkSource(system guest.System) *outcome {
	source := &e.report.Source
	source.PolicyManaged = system.WSUSConfigured || system.WindowsUpdateAccessDisabled || system.InternetLocationsDisabled
	source.Effective = e.settings.Source
	if e.settings.Source == guest.SourceDefault {
		source.Effective = "windows-update"
		if system.WSUSConfigured {
			source.Effective = "wsus"
		} else if system.MicrosoftUpdateRegistered {
			source.Effective = "microsoft-update"
		}
	}
	if e.settings.Source == guest.SourceMicrosoftUpdate {
		source.ServiceID = guest.MicrosoftUpdateServiceID
	}
	warn := func(message string) {
		source.Warnings = append(source.Warnings, message)
		e.logger.Warn(message)
	}
	switch e.settings.Source {
	case guest.SourceWSUS:
		if !system.WSUSConfigured {
			return e.blocked(report.StatusUnsupported, "source",
				`update_source = "wsus" requires the WUServer and UseWUServer policies; configure WSUS with Group Policy or an earlier provisioner, or use update_source = "default"`)
		}
	case guest.SourceWindowsUpdate, guest.SourceMicrosoftUpdate:
		if system.WindowsUpdateAccessDisabled {
			return e.blocked(report.StatusUnsupported, "source",
				fmt.Sprintf("update_source = %q is blocked by the DisableWindowsUpdateAccess policy; the plugin does not override policy", e.settings.Source))
		}
		if system.InternetLocationsDisabled {
			return e.blocked(report.StatusUnsupported, "source",
				fmt.Sprintf("update_source = %q is blocked by the DoNotConnectToWindowsUpdateInternetLocations policy; the plugin does not override policy", e.settings.Source))
		}
		if system.WSUSConfigured {
			warn(fmt.Sprintf("WSUS is configured by policy, but update_source = %q was requested explicitly; this run searches %s directly without changing the policy.", e.settings.Source, e.settings.Source))
		}
	}
	if strings.EqualFold(system.UpdateServiceStartMode, "Disabled") && e.settings.Source != guest.SourceOffline {
		return e.blocked(report.StatusUnsupported, "source",
			"the Windows Update service (wuauserv) is disabled; enable it before running the provisioner (the plugin does not change service configuration)")
	}
	e.logger.Info(fmt.Sprintf("Update source: %s (requested %s).", source.Effective, source.Requested))
	return nil
}

func (e *Engine) registerMicrosoftUpdate(ctx context.Context) *outcome {
	e.logger.Info("Registering the Microsoft Update service...")
	registration, err := withRetry(ctx, e, "register", func() (guest.Registration, error) {
		registration, err := e.guest.RegisterMicrosoftUpdate(ctx)
		if err == nil && registration.State == "failed" {
			message := "Microsoft Update registration failed"
			if registration.Message != "" {
				message += ": " + registration.Message
			}
			return registration, &guest.ScriptError{Action: "register-microsoft-update", HResult: registration.HResult, Message: message}
		}
		return registration, err
	})
	if err != nil {
		result := e.fatal("register", err)
		return &result
	}
	switch registration.State {
	case "registered":
		e.logger.Info("Microsoft Update is registered.")
		return nil
	case "pending":
		e.logger.Info("Microsoft Update registration completes after a restart.")
		return e.restart(ctx, "complete Microsoft Update registration")
	case "unsupported":
		return e.blocked(report.StatusUnsupported, "register", registration.Message)
	default:
		message := fmt.Sprintf("Microsoft Update registration failed: %s", wuerror.Describe(uint32(registration.HResult)))
		if registration.Message != "" {
			message += " (" + registration.Message + ")"
		}
		return e.blocked(report.StatusFailed, "register", message)
	}
}

func (e *Engine) installOffline(ctx context.Context) *outcome {
	e.logger.Info(fmt.Sprintf("Installing %d offline packages...", len(e.settings.OfflinePackages)))
	result, err := e.guest.InstallOffline(ctx, e.settings.OfflinePackages)
	if err != nil {
		outcome := e.fatal("offline", err)
		return &outcome
	}
	rebootRequired := result.PendingReboot.Pending
	for _, packageResult := range result.Packages {
		entry := report.OfflinePackage{
			Name:            packageResult.Name,
			SHA256:          packageResult.SHA256,
			SignatureStatus: packageResult.SignatureStatus,
			Signer:          packageResult.Signer,
			Result:          packageResult.Result,
			ExitCode:        packageResult.ExitCode,
			RebootRequired:  packageResult.RebootRequired,
		}
		if packageResult.ExitCode != 0 {
			entry.HResultName = wuerror.Lookup(uint32(int32(packageResult.ExitCode))).Name
		}
		e.report.OfflinePackages = append(e.report.OfflinePackages, entry)
		rebootRequired = rebootRequired || packageResult.RebootRequired
		message := fmt.Sprintf("Offline package %s: %s", packageResult.Name, packageResult.Result)
		switch packageResult.Result {
		case "installed", "already-installed", "not-applicable":
			e.logger.Info(message + ".")
		default:
			if packageResult.Message != "" {
				message += " (" + packageResult.Message + ")"
			} else {
				message += " (" + wuerror.Describe(uint32(int32(packageResult.ExitCode))) + ")"
			}
			e.logger.Warn(message)
			e.recordError("offline", wuerror.Permanent, 0, message)
		}
	}
	if rebootRequired {
		return e.restart(ctx, "complete offline package installation")
	}
	return nil
}

func (e *Engine) offlineOutcome() outcome {
	for _, entry := range e.report.OfflinePackages {
		if entry.Result == "failed" || entry.Result == "rejected" {
			return outcome{report.StatusPartial, "one or more offline packages were not installed"}
		}
	}
	return outcome{report.StatusUpdated, "all offline packages are installed or not applicable"}
}

func (e *Engine) updateLoop(ctx context.Context) outcome {
	request := guest.SearchRequest{Source: e.settings.Source, Criteria: e.settings.SearchCriteria, LegacyFilters: e.settings.LegacyFilters}
	limitReason := ""
	for {
		if e.report.Passes >= e.settings.MaxUpdatePasses {
			limitReason = fmt.Sprintf("reached max_update_passes (%d)", e.settings.MaxUpdatePasses)
			break
		}
		e.report.Passes++
		pass := e.report.Passes
		e.logger.Info(fmt.Sprintf("Update pass %d of at most %d.", pass, e.settings.MaxUpdatePasses))

		searchResult, result := e.search(ctx, request)
		if result != nil {
			return *result
		}
		batch := e.selectBatch(pass, searchResult.Updates)
		if len(batch) == 0 {
			e.logger.Info("No eligible updates remain.")
			return e.finalOutcome("")
		}
		installResult, result := e.install(ctx, pass, request, batch)
		if result != nil {
			return *result
		}

		if installResult.RebootRequired {
			if e.report.Reboots >= e.settings.MaxReboots {
				limitReason = fmt.Sprintf("reached max_reboots (%d) with a restart pending", e.settings.MaxReboots)
				break
			}
			if result := e.restart(ctx, fmt.Sprintf("complete update pass %d", pass)); result != nil {
				return *result
			}
		}
	}
	e.logger.Info("Validating the remaining updates...")
	searchResult, result := e.search(ctx, request)
	if result != nil {
		return *result
	}
	e.markRemaining(searchResult.Updates)
	return e.finalOutcome(limitReason)
}

func (e *Engine) search(ctx context.Context, request guest.SearchRequest) (guest.SearchResult, *outcome) {
	searchResult, err := withRetry(ctx, e, "search", func() (guest.SearchResult, error) {
		result, err := e.guest.Search(ctx, request)
		if err != nil {
			return result, err
		}
		if !result.Succeeded() {
			return result, searchFailure(result)
		}
		return result, nil
	})
	if err != nil {
		result := e.fatal("search", err)
		return guest.SearchResult{}, &result
	}
	e.logger.Info(fmt.Sprintf("Found %d applicable updates.", len(searchResult.Updates)))
	return searchResult, nil
}

type SearchError struct {
	Result  string
	HResult wuerror.HResult
	Message string
}

func (e *SearchError) Error() string {
	message := fmt.Sprintf("the update search %s: %s", e.Result, wuerror.Describe(uint32(e.HResult)))
	if e.Message != "" && !strings.Contains(message, e.Message) {
		message += " (" + e.Message + ")"
	}
	return message
}

func searchFailure(result guest.SearchResult) error {
	code := result.HResult
	if wuerror.IsSuccess(uint32(code)) {
		code = wuerror.HResult(0x80240FFF)
	}
	return &SearchError{Result: result.Result, HResult: code, Message: result.Message}
}

func (e *Engine) selectBatch(pass int, updates []selection.Update) []string {
	e.searches++
	var batch []string
	seenTitles := map[string]bool{}
	for _, update := range updates {
		entry := e.track(pass, update)
		decision := e.settings.Policy.Decide(update)
		switch {
		case decision.Excluded:
			entry.State, entry.Reason = report.StateExcluded, decision.Reason
			e.logger.Debug(fmt.Sprintf("Excluded %s: %s.", describeUpdate(update), decision.Reason))
			continue
		case !decision.Selected:
			entry.State, entry.Reason = report.StateSkipped, decision.Reason
			e.logger.Debug(fmt.Sprintf("Skipped %s: %s.", describeUpdate(update), decision.Reason))
			continue
		}
		if e.attempts[update.ID] >= e.settings.MaxInstallAttempts {
			entry.State = report.StateFailed
			entry.Reason = fmt.Sprintf("still applicable after %d install attempts", e.attempts[update.ID])
			continue
		}
		entry.State, entry.Reason = report.StateRemaining, decision.Reason
		if seenTitles[update.Title] {
			e.logger.Debug(fmt.Sprintf("Deferring %s to the next pass because an update with the same title is queued.", describeUpdate(update)))
			continue
		}
		if len(batch) >= e.settings.MaxUpdatesPerPass {
			continue
		}
		seenTitles[update.Title] = true
		if update.CanRequestUserInput {
			e.logger.Warn(fmt.Sprintf("%s may request user input; exclude it if the installation stops responding.", describeUpdate(update)))
		}
		e.logger.Info("Selected " + describeUpdate(update) + ".")
		batch = append(batch, update.ID)
	}
	return batch
}

func describeUpdate(update selection.Update) string {
	size := ""
	if update.MaxDownloadSize > 0 {
		size = fmt.Sprintf("; %.1f MB", float64(update.MaxDownloadSize)/1024/1024)
	}
	return fmt.Sprintf("%q (%s%s)", update.Title, update.Classification(), size)
}

func (e *Engine) track(pass int, update selection.Update) *report.Update {
	entry, found := e.updates[update.ID]
	if !found {
		var products []string
		for _, product := range update.Products() {
			products = append(products, product.Name)
		}
		var knowledgeBaseIDs []string
		for _, kb := range update.KBs {
			if normalized, ok := selection.NormalizeKB(kb); ok {
				knowledgeBaseIDs = append(knowledgeBaseIDs, normalized)
			}
		}
		entry = &report.Update{
			ID:            update.ID,
			FirstSeenPass: pass,
			KBs:           knowledgeBaseIDs,
			Products:      products,
		}
		e.updates[update.ID] = entry
		e.order = append(e.order, update.ID)
	}
	e.lastSeen[update.ID] = e.searches
	entry.Revision = update.Revision
	entry.Title = update.Title
	entry.Classification = string(update.Classification())
	entry.BrowseOnly = update.BrowseOnly
	entry.Preview = update.IsPreview()
	return entry
}

func (e *Engine) install(ctx context.Context, pass int, request guest.SearchRequest, batch []string) (guest.InstallResult, *outcome) {
	installRequest := guest.InstallRequest{SearchRequest: request, UpdateIDs: batch}
	installResult, err := withRetry(ctx, e, "install", func() (guest.InstallResult, error) {
		result, err := e.guest.Install(ctx, installRequest)
		if err == nil && len(result.Updates) == 0 && !wuerror.IsSuccess(uint32(result.HResult)) && result.HResult.Info().Class == wuerror.Transient {
			return result, &SearchError{Result: result.Result, HResult: result.HResult, Message: result.Message}
		}
		return result, err
	})
	if err != nil {
		result := e.fatal("install", err)
		return guest.InstallResult{}, &result
	}
	finalResults := map[string]guest.UpdateResult{}
	for _, updateResult := range installResult.Updates {
		entry, found := e.updates[updateResult.ID]
		if !found {
			continue
		}
		if updateResult.Phase == "search" {
			entry.State, entry.Reason = report.StateSuperseded, "no longer offered by the update source"
			continue
		}
		entry.Attempts = append(entry.Attempts, report.Attempt{
			Pass:           pass,
			Phase:          updateResult.Phase,
			Result:         updateResult.Result,
			ResultCode:     updateResult.ResultCode,
			HResult:        updateResult.HResult.String(),
			HResultName:    updateResult.HResult.Info().Name,
			RebootRequired: updateResult.RebootRequired,
		})
		if existing, seen := finalResults[updateResult.ID]; !seen || existing.Phase == "download" {
			finalResults[updateResult.ID] = updateResult
		}
	}
	installedCount, failedCount := 0, 0
	for _, updateID := range batch {
		entry := e.updates[updateID]
		updateResult, found := finalResults[updateID]
		if !found {
			continue
		}
		e.attempts[updateID]++
		if updateResult.Phase == "install" && updateResult.Succeeded() {
			installedCount++
			entry.State, entry.Reason = report.StateInstalled, "installed in pass "+fmt.Sprint(pass)
			if updateResult.ResultCode == 3 {
				entry.Reason += " with errors"
			}
			continue
		}
		info := updateResult.HResult.Info()
		if info.Class == wuerror.NotApplicable {
			entry.State, entry.Reason = report.StateNotApplicable, info.Description
			continue
		}
		failedCount++
		entry.State = report.StateFailed
		entry.Reason = fmt.Sprintf("%s failed: %s", updateResult.Phase, wuerror.Describe(uint32(updateResult.HResult)))
		e.logger.Warn(fmt.Sprintf("%q %s", entry.Title, entry.Reason))
	}
	e.logger.Info(fmt.Sprintf("Pass %d: %d installed, %d failed.", pass, installedCount, failedCount))
	if !wuerror.IsSuccess(uint32(installResult.HResult)) && installResult.HResult.Info().Class == wuerror.RebootRequired {
		installResult.RebootRequired = true
	}
	return installResult, nil
}

func (e *Engine) markRemaining(updates []selection.Update) {
	e.searches++
	for _, update := range updates {
		entry := e.track(e.report.Passes, update)
		decision := e.settings.Policy.Decide(update)
		if !decision.Selected {
			entry.State, entry.Reason = report.StateSkipped, decision.Reason
			if decision.Excluded {
				entry.State = report.StateExcluded
			}
			continue
		}
		if entry.State != report.StateFailed {
			entry.State, entry.Reason = report.StateRemaining, "still applicable at the end of the run"
		}
	}
}

func (e *Engine) finalOutcome(limitReason string) outcome {
	remaining, failed := 0, 0
	for updateID, entry := range e.updates {
		notOfferedAnymore := e.lastSeen[updateID] < e.searches
		if notOfferedAnymore && (entry.State == report.StateFailed || entry.State == report.StateRemaining) {
			entry.State, entry.Reason = report.StateSuperseded, "no longer offered by the update source after an earlier attempt"
		}
		switch entry.State {
		case report.StateRemaining:
			remaining++
		case report.StateFailed:
			failed++
		}
	}
	for _, entry := range e.report.OfflinePackages {
		if entry.Result == "failed" || entry.Result == "rejected" {
			failed++
		}
	}
	switch {
	case remaining == 0 && failed == 0 && limitReason == "":
		return outcome{report.StatusUpdated, "no eligible updates remain"}
	case limitReason != "":
		return outcome{report.StatusPartial, fmt.Sprintf("%s; %d eligible updates remain and %d failed", limitReason, remaining, failed)}
	default:
		return outcome{report.StatusPartial, fmt.Sprintf("%d eligible updates remain and %d failed", remaining, failed)}
	}
}

func (e *Engine) finishUpdateStates() {
	e.report.Updates = e.report.Updates[:0]
	for _, updateID := range e.order {
		e.report.Updates = append(e.report.Updates, *e.updates[updateID])
	}
	sort.SliceStable(e.report.Updates, func(left, right int) bool {
		return e.report.Updates[left].FirstSeenPass < e.report.Updates[right].FirstSeenPass
	})
}

func (e *Engine) restart(ctx context.Context, reason string) *outcome {
	if e.report.Reboots >= e.settings.MaxReboots {
		return e.blocked(report.StatusPartial, "restart", fmt.Sprintf("a restart is required to %s, but max_reboots (%d) is reached", reason, e.settings.MaxReboots))
	}
	err := e.guest.Restart(ctx, reason)
	e.report.Reboots++
	e.report.RebootEvents = append(e.report.RebootEvents, report.RebootEvent{At: e.now().UTC(), Reason: reason, Expected: true})
	if err != nil {
		result := e.fatal("restart", err)
		return &result
	}
	pending, err := withRetry(ctx, e, "servicing", func() (guest.PendingReboot, error) { return e.guest.PendingReboot(ctx) })
	if err != nil {
		result := e.fatal("servicing", err)
		return &result
	}
	if pending.ServicingIdle != nil && !*pending.ServicingIdle {
		e.logger.Warn("Windows servicing was still busy when the servicing timeout expired; continuing.")
	}
	if pending.Pending {
		e.logger.Info(fmt.Sprintf("Another restart is pending (%s).", strings.Join(pending.Reasons, ", ")))
		return e.restart(ctx, "finish servicing operations")
	}
	return nil
}

func (e *Engine) fatal(phase string, err error) outcome {
	if errors.Is(err, context.Canceled) {
		return outcome{report.StatusCancelled, "the build was cancelled"}
	}
	class, code := classify(err)
	e.recordError(phase, class, code, err.Error())
	e.logger.Warn(fmt.Sprintf("%s failed: %v", phase, err))
	status := report.StatusFailed
	if class == wuerror.Unsupported {
		status = report.StatusUnsupported
	}
	if status == report.StatusFailed && e.anyInstalled() {
		status = report.StatusPartial
	}
	return outcome{status, fmt.Sprintf("%s failed: %v", phase, err)}
}

func (e *Engine) blocked(status report.Status, phase, message string) *outcome {
	class := wuerror.Permanent
	if status == report.StatusUnsupported {
		class = wuerror.Unsupported
	}
	e.recordError(phase, class, 0, message)
	e.logger.Warn(message)
	return &outcome{status, message}
}

func (e *Engine) recordError(phase string, class wuerror.Class, code wuerror.HResult, message string) {
	entry := report.Error{At: e.now().UTC(), Phase: phase, Class: string(class), Message: message}
	if code != 0 {
		entry.HResult = code.String()
	}
	e.report.Errors = append(e.report.Errors, entry)
}

func (e *Engine) anyInstalled() bool {
	for _, entry := range e.updates {
		if entry.State == report.StateInstalled {
			return true
		}
	}
	for _, entry := range e.report.OfflinePackages {
		if entry.Result == "installed" {
			return true
		}
	}
	return false
}

func classify(err error) (wuerror.Class, wuerror.HResult) {
	var searchError *SearchError
	if errors.As(err, &searchError) {
		return searchError.HResult.Info().Class, searchError.HResult
	}
	var scriptError *guest.ScriptError
	if errors.As(err, &scriptError) {
		return scriptError.HResult.Info().Class, scriptError.HResult
	}
	var communicationError *guest.CommunicationError
	if errors.As(err, &communicationError) {
		return wuerror.Transient, 0
	}
	var timeoutError *guest.TimeoutError
	if errors.As(err, &timeoutError) {
		return wuerror.Transient, 0
	}
	return wuerror.Permanent, 0
}

func retryable(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	class, _ := classify(err)
	return class == wuerror.Transient
}

func withRetry[T any](ctx context.Context, e *Engine, phase string, operation func() (T, error)) (T, error) {
	delay := e.settings.RetryDelay
	var lastErr error
	for attempt := 0; ; attempt++ {
		result, err := operation()
		if err == nil {
			return result, nil
		}
		lastErr = err
		if ctx.Err() != nil || !retryable(err) || attempt >= e.settings.MaxRetries {
			return result, lastErr
		}
		e.logger.Warn(fmt.Sprintf("%s failed (attempt %d of %d), retrying in %s: %v", phase, attempt+1, e.settings.MaxRetries+1, delay, err))
		if sleepErr := e.sleep(ctx, delay); sleepErr != nil {
			return result, sleepErr
		}
		e.detectUnexpectedRestart(ctx, err)
		delay *= 2
		if e.settings.MaxRetryDelay > 0 && delay > e.settings.MaxRetryDelay {
			delay = e.settings.MaxRetryDelay
		}
	}
}

func (e *Engine) detectUnexpectedRestart(ctx context.Context, cause error) {
	var communicationError *guest.CommunicationError
	var timeoutError *guest.TimeoutError
	if !errors.As(cause, &communicationError) && !errors.As(cause, &timeoutError) {
		return
	}
	known := e.guest.KnownBootID()
	current, err := e.guest.BootID(ctx)
	if err != nil || known == "" || current == known {
		return
	}
	e.guest.NoteRestart(current)
	e.report.Reboots++
	e.report.RebootEvents = append(e.report.RebootEvents, report.RebootEvent{At: e.now().UTC(), Reason: "unexpected restart detected after a communication failure", Expected: false})
	e.logger.Warn("The guest restarted unexpectedly; continuing.")
}
