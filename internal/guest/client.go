package guest

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/larskroghdk/packer-plugin-microsoft-update/internal/selection"
	"github.com/larskroghdk/packer-plugin-microsoft-update/internal/wuerror"
)

//go:embed scripts/microsoft-update.ps1
var updateScript []byte

//go:embed scripts/elevated-runner.ps1
var runnerTemplateSource string

var runnerTemplate = template.Must(template.New("runner").
	Delims("<%", "%>").
	Funcs(template.FuncMap{"psquote": powerShellQuote}).
	Parse(runnerTemplateSource))

const TaskNamePrefix = "packer-microsoft-update-"

type Logger interface {
	Info(message string)
	Warn(message string)
	Debug(message string)
}
type Timeouts struct {
	Startup   time.Duration
	Search    time.Duration
	Download  time.Duration
	Install   time.Duration
	Servicing time.Duration
	Reboot    time.Duration
	Command   time.Duration
	Upload    time.Duration
}
type Options struct {
	Username     string
	Password     string
	Timeouts     Timeouts
	PollInterval time.Duration
}
type Client struct {
	communicator   packer.Communicator
	ui             packer.Ui
	logger         Logger
	options        Options
	runID          string
	workDirectory  string
	invocations    int
	bootID         string
	scriptUploaded bool
}

func NewClient(communicator packer.Communicator, ui packer.Ui, logger Logger, options Options) *Client {
	if options.PollInterval == 0 {
		options.PollInterval = 10 * time.Second
	}
	return &Client{communicator: communicator, ui: ui, logger: logger, options: options, runID: newRunID()}
}

func newRunID() string {
	randomBytes := make([]byte, 4)
	if _, err := rand.Read(randomBytes); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(randomBytes)
}

type bootstrapRecord struct {
	SystemRoot        string `json:"system_root"`
	BootTime          string `json:"boot_time"`
	PowerShellVersion string `json:"powershell_version"`
}

func (c *Client) Prepare(ctx context.Context) error {
	script := fmt.Sprintf(`$ErrorActionPreference = 'Stop'
$workDirectory = Join-Path $env:SystemRoot ('Temp\%s' + %s)
[void](New-Item -ItemType Directory -Force -Path $workDirectory)
$bootTime = [string](Get-WmiObject -Class Win32_OperatingSystem).LastBootUpTime
[Console]::Out.WriteLine('%s{"type":"bootstrap","system_root":"' + $env:SystemRoot.Replace('\','\\') + '","boot_time":"' + $bootTime + '","powershell_version":"' + $PSVersionTable.PSVersion + '"}')
`, TaskNamePrefix, powerShellQuote(c.runID), recordPrefix)
	records, err := c.runDirect(ctx, "prepare", script)
	if err != nil {
		return err
	}
	for _, parsed := range records {
		if parsed.Type != "bootstrap" {
			continue
		}
		var bootstrap bootstrapRecord
		if err := parsed.decode(&bootstrap); err != nil {
			return err
		}
		if bootstrap.SystemRoot == "" {
			return errors.New("the guest did not report its Windows directory")
		}
		c.workDirectory = bootstrap.SystemRoot + `\Temp\` + TaskNamePrefix + c.runID
		c.bootID = bootstrap.BootTime
		c.logger.Debug(fmt.Sprintf("Guest work directory %s (Windows PowerShell %s).", c.workDirectory, bootstrap.PowerShellVersion))
		return nil
	}
	return errors.New("the guest did not answer the bootstrap command; Windows PowerShell 2.0 or later is required")
}

func (c *Client) Cleanup(ctx context.Context) {
	if c.workDirectory == "" {
		return
	}
	script := fmt.Sprintf("Remove-Item -LiteralPath %s -Recurse -Force -ErrorAction SilentlyContinue", powerShellQuote(c.workDirectory))
	if _, err := c.runDirect(ctx, "cleanup", script); err != nil {
		c.logger.Warn(fmt.Sprintf("Could not remove the guest work directory %s: %v", c.workDirectory, err))
	}
}

func (c *Client) Detect(ctx context.Context) (System, error) {
	records, err := c.invoke(ctx, "detect", nil)
	if err != nil {
		return System{}, err
	}
	var system System
	if err := decodeSingle(records, "system", &system); err != nil {
		return System{}, err
	}
	if system.BootTime != "" {
		c.bootID = system.BootTime
	}
	return system, nil
}

func (c *Client) RegisterMicrosoftUpdate(ctx context.Context) (Registration, error) {
	records, err := c.invoke(ctx, "register-microsoft-update", nil)
	if err != nil {
		return Registration{}, err
	}
	var registration Registration
	err = decodeSingle(records, "registration", &registration)
	return registration, err
}

func (c *Client) Search(ctx context.Context, request SearchRequest) (SearchResult, error) {
	records, err := c.invoke(ctx, "search", searchParameters(request))
	if err != nil {
		return SearchResult{}, err
	}
	var result SearchResult
	if err := decodeSingle(records, "search", &result); err != nil {
		return SearchResult{}, err
	}
	for _, parsed := range records {
		if parsed.Type != "update" {
			continue
		}
		var update selection.Update
		if err := parsed.decode(&update); err != nil {
			return SearchResult{}, err
		}
		result.Updates = append(result.Updates, update)
	}
	return result, nil
}

func (c *Client) Install(ctx context.Context, request InstallRequest) (InstallResult, error) {
	parameters := searchParameters(request.SearchRequest)
	for _, updateID := range request.UpdateIDs {
		parameters = append(parameters, requestParameter{"update_id", updateID})
	}
	records, err := c.invoke(ctx, "install", parameters)
	if err != nil {
		return InstallResult{}, err
	}
	var result InstallResult
	var searchResult SearchResult
	if err := decodeSingle(records, "search", &searchResult); err != nil {
		return InstallResult{}, err
	}
	if !searchResult.Succeeded() {
		return InstallResult{ResultCode: searchResult.ResultCode, Result: searchResult.Result, HResult: searchResult.HResult, Message: searchResult.Message}, nil
	}
	if err := decodeSingle(records, "install", &result); err != nil {
		return InstallResult{}, err
	}
	for _, parsed := range records {
		if parsed.Type != "update-result" {
			continue
		}
		var updateResult UpdateResult
		if err := parsed.decode(&updateResult); err != nil {
			return InstallResult{}, err
		}
		result.Updates = append(result.Updates, updateResult)
	}
	return result, nil
}

func (c *Client) InstallOffline(ctx context.Context, packages []OfflinePackage) (OfflineResult, error) {
	var parameters []requestParameter
	for _, offlinePackage := range packages {
		guestPath := offlinePackage.GuestPath
		expectedSHA256 := strings.ToLower(offlinePackage.SHA256)
		if offlinePackage.HostPath != "" {
			uploadedPath, hostSHA256, err := c.uploadPackage(ctx, offlinePackage.HostPath)
			if err != nil {
				return OfflineResult{}, err
			}
			if expectedSHA256 != "" && expectedSHA256 != hostSHA256 {
				return OfflineResult{}, fmt.Errorf("the SHA-256 checksum of %s is %s, expected %s", offlinePackage.HostPath, hostSHA256, expectedSHA256)
			}
			guestPath, expectedSHA256 = uploadedPath, hostSHA256
		}
		parameters = append(parameters, requestParameter{"offline_package", guestPath + "|" + expectedSHA256})
	}
	records, err := c.invoke(ctx, "offline-install", parameters)
	if err != nil {
		return OfflineResult{}, err
	}
	var result OfflineResult
	for _, parsed := range records {
		if parsed.Type != "offline-result" {
			continue
		}
		var packageResult OfflinePackageResult
		if err := parsed.decode(&packageResult); err != nil {
			return OfflineResult{}, err
		}
		result.Packages = append(result.Packages, packageResult)
	}
	err = decodeSingle(records, "pending-reboot", &result.PendingReboot)
	return result, err
}

func (c *Client) PendingReboot(ctx context.Context) (PendingReboot, error) {
	seconds := int(c.options.Timeouts.Servicing.Seconds())
	records, err := c.invoke(ctx, "pending-reboot", []requestParameter{{"servicing_timeout_seconds", strconv.Itoa(seconds)}})
	if err != nil {
		return PendingReboot{}, err
	}
	var pending PendingReboot
	err = decodeSingle(records, "pending-reboot", &pending)
	return pending, err
}

func (c *Client) BootID(ctx context.Context) (string, error) {
	script := fmt.Sprintf(`[Console]::Out.WriteLine('%s{"type":"boot","boot_time":"' + [string](Get-WmiObject -Class Win32_OperatingSystem).LastBootUpTime + '"}')`, recordPrefix)
	records, err := c.runDirect(ctx, "boot-id", script)
	if err != nil {
		return "", err
	}
	var boot struct {
		BootTime string `json:"boot_time"`
	}
	if err := decodeSingle(records, "boot", &boot); err != nil {
		return "", err
	}
	if boot.BootTime == "" {
		return "", errors.New("the guest reported an empty boot time")
	}
	return boot.BootTime, nil
}

func (c *Client) KnownBootID() string { return c.bootID }

func (c *Client) NoteRestart(bootID string) {
	c.bootID = bootID
	c.scriptUploaded = false
}

func (c *Client) Restart(ctx context.Context, reason string) error {
	previousBootID := c.bootID
	if previousBootID == "" {
		bootID, err := c.BootID(ctx)
		if err != nil {
			return err
		}
		previousBootID = bootID
	}
	c.logger.Info(fmt.Sprintf("Restarting the guest (%s)...", reason))
	restartCommand := `shutdown.exe /r /f /t 5 /d p:2:17 /c "packer-plugin-microsoft-update restart"`
	exitStatus, err := c.runCommand(ctx, "restart", restartCommand)
	if err != nil {
		c.logger.Debug(fmt.Sprintf("The restart command was interrupted: %v", err))
	} else if exitStatus != 0 && exitStatus != 1190 {
		return fmt.Errorf("shutdown.exe exited with status %d", exitStatus)
	}
	waitContext, cancel := context.WithTimeout(ctx, c.options.Timeouts.Reboot)
	defer cancel()
	c.logger.Info("Waiting for the guest to restart...")
	for {
		select {
		case <-waitContext.Done():
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return &TimeoutError{Phase: "restart", Timeout: c.options.Timeouts.Reboot}
		case <-time.After(c.options.PollInterval):
		}
		bootID, err := c.BootID(waitContext)
		if err != nil {
			c.logger.Debug(fmt.Sprintf("The guest is not available yet: %v", err))
			continue
		}
		if bootID != previousBootID {
			c.NoteRestart(bootID)
			c.logger.Info("The guest restarted and is accepting commands.")
			return nil
		}
	}
}

func searchParameters(request SearchRequest) []requestParameter {
	parameters := []requestParameter{
		{"source", request.Source},
		{"search_criteria", request.Criteria},
	}
	for _, filter := range request.LegacyFilters {
		parameters = append(parameters, requestParameter{"legacy_filter", filter})
	}
	return parameters
}

func decodeSingle(records []record, recordType string, target any) error {
	for _, parsed := range records {
		if parsed.Type == recordType {
			return parsed.decode(target)
		}
	}
	return fmt.Errorf("the guest did not report a %s result", recordType)
}

func (c *Client) guestPath(name string) string {
	return c.workDirectory + `\` + name
}

func uploadPath(guestPath string) string {
	return strings.ReplaceAll(guestPath, `\`, "/")
}

func (c *Client) upload(ctx context.Context, guestPath string, content []byte) error {
	return c.withUploadTimeout(ctx, "upload "+filepath.Base(uploadPath(guestPath)), func() error {
		return c.communicator.Upload(uploadPath(guestPath), bytes.NewReader(content), nil)
	})
}

func (c *Client) withUploadTimeout(ctx context.Context, operation string, upload func() error) error {
	uploadContext, cancel := context.WithTimeout(ctx, c.options.Timeouts.Upload)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- upload() }()
	select {
	case err := <-result:
		if err != nil {
			return &CommunicationError{Operation: operation, Err: err}
		}
		return nil
	case <-uploadContext.Done():
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &TimeoutError{Phase: operation, Timeout: c.options.Timeouts.Upload}
	}
}

func (c *Client) uploadPackage(ctx context.Context, hostPath string) (guestPath, checksum string, err error) {
	file, err := os.Open(hostPath)
	if err != nil {
		return "", "", fmt.Errorf("open offline package: %w", err)
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", "", fmt.Errorf("hash offline package %s: %w", hostPath, err)
	}
	checksum = hex.EncodeToString(hash.Sum(nil))
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", "", fmt.Errorf("rewind offline package %s: %w", hostPath, err)
	}
	info, err := file.Stat()
	if err != nil {
		return "", "", fmt.Errorf("stat offline package %s: %w", hostPath, err)
	}
	guestPath = c.guestPath(checksum[:16] + "-" + filepath.Base(hostPath))
	c.logger.Info(fmt.Sprintf("Uploading offline package %s (%.1f MB)...", filepath.Base(hostPath), float64(info.Size())/1024/1024))
	err = c.withUploadTimeout(ctx, "upload "+filepath.Base(hostPath), func() error {
		return c.communicator.Upload(uploadPath(guestPath), file, &info)
	})
	return guestPath, checksum, err
}

type runnerOptions struct {
	TaskName           string
	ScriptPath         string
	RequestPath        string
	LogPath            string
	Username           string
	Password           string
	ExecutionTimeLimit string
}

func (c *Client) executionTimeLimit() string {
	timeouts := c.options.Timeouts
	total := timeouts.Startup + timeouts.Search + timeouts.Download + timeouts.Install + timeouts.Servicing
	hours := int(total.Hours()) + 1
	return fmt.Sprintf("PT%dH", hours)
}

func (c *Client) invoke(ctx context.Context, action string, parameters []requestParameter) ([]record, error) {
	if c.workDirectory == "" {
		return nil, errors.New("guest client used before Prepare")
	}
	c.invocations++
	taskName := fmt.Sprintf("%s%s-%03d", TaskNamePrefix, c.runID, c.invocations)
	scriptPath := c.guestPath("microsoft-update.ps1")
	requestPath := c.guestPath(taskName + ".request")
	runnerPath := c.guestPath(taskName + ".runner.ps1")
	if !c.scriptUploaded {
		if err := c.upload(ctx, scriptPath, updateScript); err != nil {
			return nil, err
		}
		c.scriptUploaded = true
	}
	parameters = append([]requestParameter{{"action", action}, {"task_name", taskName}}, parameters...)
	if err := c.upload(ctx, requestPath, encodeRequest(parameters)); err != nil {
		return nil, err
	}
	var runner bytes.Buffer
	if err := runnerTemplate.Execute(&runner, runnerOptions{
		TaskName:           taskName,
		ScriptPath:         scriptPath,
		RequestPath:        requestPath,
		LogPath:            c.guestPath(taskName + ".log"),
		Username:           c.options.Username,
		Password:           c.options.Password,
		ExecutionTimeLimit: c.executionTimeLimit(),
	}); err != nil {
		return nil, fmt.Errorf("render elevated runner: %w", err)
	}
	if err := c.upload(ctx, runnerPath, runner.Bytes()); err != nil {
		return nil, err
	}
	c.logger.Debug(fmt.Sprintf("Running guest action %s as scheduled task %s.", action, taskName))
	watcher := newPhaseWatcher(ctx, c.options.Timeouts)
	defer watcher.stop()
	collector := newOutputCollector(c.ui, c.logger, watcher)
	command := &packer.RemoteCmd{
		Command: fmt.Sprintf(`powershell.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "%s"`, runnerPath),
	}
	runErr := command.RunWithUi(watcher.context, c.communicator, collector)
	if ctx.Err() != nil || watcher.expired() != nil || runErr != nil {
		c.abortTask(taskName)
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if timeoutErr := watcher.expired(); timeoutErr != nil {
		return nil, timeoutErr
	}
	if runErr != nil {
		return nil, &CommunicationError{Operation: action, Err: runErr}
	}
	if collector.parseErr != nil {
		return nil, collector.parseErr
	}
	if collector.footer == nil {
		return nil, &CommunicationError{Operation: action, Err: fmt.Errorf("the elevated runner ended without a result (exit status %d)", command.ExitStatus())}
	}
	for _, parsed := range collector.records {
		if parsed.Type != "error" {
			continue
		}
		var failure errorRecord
		if err := parsed.decode(&failure); err != nil {
			return nil, err
		}
		code, _ := wuerror.Parse(failure.HResult)
		return nil, &ScriptError{Action: action, HResult: wuerror.HResult(code), Message: failure.Message, Position: failure.Position}
	}
	if !collector.done {
		return nil, &ScriptError{Action: action, Message: fmt.Sprintf("the guest script exited with code %d without completing", collector.footer.ExitCode)}
	}
	return collector.records, nil
}

func (c *Client) abortTask(taskName string) {
	abortContext, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	script := fmt.Sprintf(`$scheduler = New-Object -ComObject 'Schedule.Service'
$scheduler.Connect()
$rootFolder = $scheduler.GetFolder('\')
try { $rootFolder.GetTask(%[1]s).Stop(0) } catch { }
try { $rootFolder.DeleteTask(%[1]s, 0) } catch { }`, powerShellQuote(taskName))
	if _, err := c.runDirect(abortContext, "abort", script); err != nil {
		c.logger.Warn(fmt.Sprintf("Could not stop scheduled task %s; it is removed on the next run: %v", taskName, err))
	}
}

func (c *Client) runDirect(ctx context.Context, operation, script string) ([]record, error) {
	commandContext, cancel := context.WithTimeout(ctx, c.options.Timeouts.Command)
	defer cancel()
	collector := newOutputCollector(c.ui, c.logger, nil)
	command := &packer.RemoteCmd{Command: encodedPowerShellCommand(script)}
	if err := command.RunWithUi(commandContext, c.communicator, collector); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &CommunicationError{Operation: operation, Err: err}
	}
	if collector.parseErr != nil {
		return nil, collector.parseErr
	}
	if status := command.ExitStatus(); status != 0 {
		return nil, &CommunicationError{Operation: operation, Err: fmt.Errorf("exit status %d", status)}
	}
	return collector.records, nil
}

func (c *Client) runCommand(ctx context.Context, operation, commandLine string) (int, error) {
	commandContext, cancel := context.WithTimeout(ctx, c.options.Timeouts.Command)
	defer cancel()
	collector := newOutputCollector(c.ui, c.logger, nil)
	command := &packer.RemoteCmd{Command: commandLine}
	if err := command.RunWithUi(commandContext, c.communicator, collector); err != nil {
		return 0, &CommunicationError{Operation: operation, Err: err}
	}
	return command.ExitStatus(), nil
}

type phaseWatcher struct {
	context  context.Context
	cancel   context.CancelFunc
	timeouts Timeouts

	mutex      sync.Mutex
	timer      *time.Timer
	phase      string
	phaseLimit time.Duration
	timeoutErr *TimeoutError
	stopOnce   sync.Once
}

func newPhaseWatcher(parent context.Context, timeouts Timeouts) *phaseWatcher {
	invocationContext, cancel := context.WithCancel(parent)
	watcher := &phaseWatcher{context: invocationContext, cancel: cancel, timeouts: timeouts}
	watcher.enter("startup")
	return watcher
}

func (w *phaseWatcher) limitFor(phase string) time.Duration {
	switch phase {
	case "search":
		return w.timeouts.Search
	case "download":
		return w.timeouts.Download
	case "install", "offline":
		return w.timeouts.Install
	case "servicing-wait":
		return w.timeouts.Servicing + time.Minute
	default:
		return w.timeouts.Startup
	}
}

func (w *phaseWatcher) enter(phase string) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	if w.timeoutErr != nil {
		return
	}
	if w.timer != nil {
		w.timer.Stop()
	}
	w.phase = phase
	w.phaseLimit = w.limitFor(phase)
	limit := w.phaseLimit
	w.timer = time.AfterFunc(limit, func() {
		w.mutex.Lock()
		if w.phase == phase && w.timeoutErr == nil {
			w.timeoutErr = &TimeoutError{Phase: phase, Timeout: limit}
		}
		w.mutex.Unlock()
		w.cancel()
	})
}

func (w *phaseWatcher) expired() *TimeoutError {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	return w.timeoutErr
}

func (w *phaseWatcher) stop() {
	w.stopOnce.Do(func() {
		w.mutex.Lock()
		if w.timer != nil {
			w.timer.Stop()
		}
		w.mutex.Unlock()
		w.cancel()
	})
}

type outputCollector struct {
	packer.Ui
	logger   Logger
	watcher  *phaseWatcher
	mutex    sync.Mutex
	records  []record
	footer   *runnerFooter
	done     bool
	parseErr error
}

func newOutputCollector(ui packer.Ui, logger Logger, watcher *phaseWatcher) *outputCollector {
	return &outputCollector{Ui: ui, logger: logger, watcher: watcher}
}

func (o *outputCollector) Say(line string)     { o.handle(line, false) }
func (o *outputCollector) Message(line string) { o.handle(line, false) }
func (o *outputCollector) Error(line string)   { o.handle(line, true) }

func (o *outputCollector) Sayf(format string, arguments ...any) {
	o.handle(fmt.Sprintf(format, arguments...), false)
}

func (o *outputCollector) Errorf(format string, arguments ...any) {
	o.handle(fmt.Sprintf(format, arguments...), true)
}

func (o *outputCollector) handle(line string, fromStderr bool) {
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return
	}
	if footer, ok := parseRunnerFooter(line); ok {
		o.mutex.Lock()
		o.footer = &footer
		o.mutex.Unlock()
		return
	}
	parsed, isRecord, err := parseRecord(line)
	if !isRecord {
		if fromStderr {
			o.logger.Warn("guest: " + line)
		} else {
			o.logger.Debug("guest: " + line)
		}
		return
	}
	if err != nil {
		o.mutex.Lock()
		if o.parseErr == nil {
			o.parseErr = err
		}
		o.mutex.Unlock()
		return
	}
	switch parsed.Type {
	case "log":
		var entry logRecord
		if parsed.decode(&entry) == nil {
			switch entry.Level {
			case "warning", "error":
				o.logger.Warn(entry.Message)
			case "debug":
				o.logger.Debug(entry.Message)
			default:
				o.logger.Info(entry.Message)
			}
		}
	case "phase":
		var phase phaseRecord
		if parsed.decode(&phase) == nil && o.watcher != nil {
			o.watcher.enter(phase.Name)
		}
	case "heartbeat":
		var heartbeat heartbeatRecord
		if parsed.decode(&heartbeat) == nil {
			o.logger.Debug(fmt.Sprintf("Still working (cpu %d%%, memory %d%%)...", heartbeat.CPUPercent, heartbeat.MemoryPercent))
		}
	default:
		o.mutex.Lock()
		if parsed.Type == "done" {
			o.done = true
		}
		o.records = append(o.records, parsed)
		o.mutex.Unlock()
	}
}
