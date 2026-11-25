package provisioner

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/larskroghdk/packer-plugin-microsoft-update/internal/engine"
	"github.com/larskroghdk/packer-plugin-microsoft-update/internal/guest"
	"github.com/larskroghdk/packer-plugin-microsoft-update/internal/report"
	"github.com/larskroghdk/packer-plugin-microsoft-update/internal/selection"
	"github.com/larskroghdk/packer-plugin-microsoft-update/version"
)

type Provisioner struct {
	config Config
}

func (p *Provisioner) ConfigSpec() hcldec.ObjectSpec {
	return p.config.FlatMapstructure().HCL2Spec()
}

func (p *Provisioner) Prepare(raws ...any) error {
	return p.config.Decode(raws...)
}

func (p *Provisioner) Provision(ctx context.Context, ui packer.Ui, communicator packer.Communicator, _ map[string]any) error {
	logger := newUILogger(ui, p.config.LogLevel == "debug")
	client := guest.NewClient(communicator, ui, logger, guest.Options{
		Username: p.config.Username,
		Password: p.config.Password,
		Timeouts: guest.Timeouts{
			Startup:   10 * time.Minute,
			Search:    p.config.SearchTimeout,
			Download:  p.config.DownloadTimeout,
			Install:   p.config.InstallTimeout,
			Servicing: p.config.ServicingTimeout,
			Reboot:    p.config.RebootTimeout,
			Command:   5 * time.Minute,
			Upload:    p.config.InstallTimeout,
		},
	})
	if err := client.Prepare(ctx); err != nil {
		return fmt.Errorf("microsoft-update: prepare guest: %w", err)
	}
	defer client.Cleanup(context.Background())
	updateEngine := engine.New(client, p.engineSettings(), logger, report.Plugin{
		Name:    "packer-plugin-microsoft-update",
		Version: version.PluginVersion.String(),
		Commit:  version.Commit,
	})
	result := updateEngine.Run(ctx)
	printSummary(logger, result)

	if p.config.ReportPath != "" {
		if err := result.Write(p.config.ReportPath); err != nil {
			logger.Warn(err.Error())
		} else {
			logger.Info("Report written to " + p.config.ReportPath + ".")
		}
	}
	switch {
	case result.Status == report.StatusCancelled:
		return ctx.Err()
	case result.Status == report.StatusUpdated:
		return nil
	case p.config.FailOnError.False():
		logger.Warn(fmt.Sprintf("Continuing because fail_on_error is false (status %s).", result.Status))
		return nil
	default:
		return fmt.Errorf("microsoft-update: the guest is %s: %s", result.Status, result.StatusReason)
	}
}

func (p *Provisioner) engineSettings() engine.Settings {
	policy := p.config.Policy()
	var classifications []string
	for _, classification := range selection.AllClassifications {
		if policy.Classifications[classification] {
			classifications = append(classifications, string(classification))
		}
	}
	var offlinePackages []guest.OfflinePackage
	for _, offlinePackage := range p.config.OfflinePackages {
		offlinePackages = append(offlinePackages, guest.OfflinePackage{
			HostPath:  offlinePackage.Source,
			GuestPath: offlinePackage.GuestPath,
			SHA256:    strings.ToLower(offlinePackage.SHA256),
		})
	}
	return engine.Settings{
		Source:             p.config.UpdateSource,
		SearchCriteria:     p.config.SearchCriteria,
		LegacyFilters:      p.config.Filters,
		Policy:             policy,
		OfflinePackages:    offlinePackages,
		MaxUpdatePasses:    p.config.MaxUpdatePasses,
		MaxReboots:         p.config.MaxReboots,
		MaxUpdatesPerPass:  p.config.MaxUpdatesPerPass,
		MaxInstallAttempts: p.config.MaxInstallAttempts,
		MaxRetries:         p.config.MaxRetries,
		RetryDelay:         p.config.RetryDelay,
		MaxRetryDelay:      5 * time.Minute,
		ReportSettings: report.Settings{
			Profile:               p.config.Profile,
			SearchCriteria:        p.config.SearchCriteria,
			Classifications:       classifications,
			IncludeOptional:       policy.IncludeOptional,
			IncludePreview:        policy.IncludePreview,
			IncludeDrivers:        policy.IncludeDrivers,
			IncludeFeatureUpdates: policy.IncludeFeatureUpdates,
			LegacyFilters:         policy.LegacyFilters,
			MaxUpdatePasses:       p.config.MaxUpdatePasses,
			MaxReboots:            p.config.MaxReboots,
			MaxInstallAttempts:    p.config.MaxInstallAttempts,
		},
	}
}

func printSummary(logger *uiLogger, result *report.Report) {
	summary := result.Summary
	logger.Info(fmt.Sprintf("Result: %s after %d passes and %d restarts in %s: %d installed, %d failed, %d remaining, %d skipped, %d excluded.",
		result.Status, result.Passes, result.Reboots, time.Duration(result.DurationSeconds*float64(time.Second)).Round(time.Second),
		summary.Installed, summary.Failed, summary.Remaining, summary.Skipped, summary.Excluded))
	for _, update := range result.Updates {
		switch update.State {
		case report.StateFailed, report.StateRemaining:
			logger.Warn(fmt.Sprintf("%s: %q (%s).", update.State, update.Title, update.Reason))
		}
	}
}

type uiLogger struct {
	ui    packer.Ui
	debug bool
}

func newUILogger(ui packer.Ui, debug bool) *uiLogger {
	return &uiLogger{ui: ui, debug: debug}
}

func (l *uiLogger) Info(message string) {
	log.Printf("[INFO] microsoft-update: %s", message)
	l.ui.Say(message)
}

func (l *uiLogger) Warn(message string) {
	log.Printf("[WARN] microsoft-update: %s", message)
	l.ui.Say("Warning: " + message)
}

func (l *uiLogger) Debug(message string) {
	log.Printf("[DEBUG] microsoft-update: %s", message)
	if l.debug {
		l.ui.Say(message)
	}
}
