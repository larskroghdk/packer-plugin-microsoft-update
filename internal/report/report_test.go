package report

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func compileSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(Schema))
	if err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	if err := compiler.AddResource("report.schema.json", document); err != nil {
		t.Fatalf("add schema: %v", err)
	}
	schema, err := compiler.Compile("report.schema.json")
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	return schema
}

func validate(t *testing.T, schema *jsonschema.Schema, result *Report) error {
	t.Helper()
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return schema.Validate(instance)
}

func sampleReport() *Report {
	started := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	result := New(Plugin{Name: "packer-plugin-microsoft-update", Version: "0.1.0", Commit: "abc"}, started)
	result.System = &System{Caption: "Microsoft Windows 11 Enterprise", Build: 26200, Architecture: "AMD64"}
	result.Source = Source{Requested: "microsoft-update", Effective: "microsoft-update", ServiceID: "7971f918-a847-4430-9279-4a52d1efe18d", Warnings: []string{}}
	result.Settings = Settings{Profile: "recommended", Classifications: []string{"security"}, MaxUpdatePasses: 20, MaxReboots: 15, MaxInstallAttempts: 2}
	result.Passes, result.Reboots = 2, 1
	result.RebootEvents = []RebootEvent{{At: started.Add(time.Minute), Reason: "complete update pass 1", Expected: true}}
	result.Updates = []Update{
		{ID: "11111111-1111-1111-1111-111111111111", Title: "Cumulative update", KBs: []string{"KB5000001"}, Classification: "security", State: StateInstalled, FirstSeenPass: 1,
			Attempts: []Attempt{{Pass: 1, Phase: "install", Result: "succeeded", ResultCode: 2, HResult: "0x00000000", HResultName: "S_OK", RebootRequired: true}}},
		{ID: "22222222-2222-2222-2222-222222222222", Title: "Driver", Classification: "driver", State: StateSkipped, Reason: "driver update", FirstSeenPass: 1},
		{ID: "33333333-3333-3333-3333-333333333333", Title: "Broken", Classification: "security", State: StateFailed, FirstSeenPass: 1,
			Attempts: []Attempt{{Pass: 1, Phase: "install", Result: "failed", ResultCode: 4, HResult: "0x80070643", HResultName: "ERROR_INSTALL_FAILURE"}}},
	}
	result.OfflinePackages = []OfflinePackage{{Name: "kb.msu", SHA256: strings.Repeat("a", 64), SignatureStatus: "Valid", Result: "installed", ExitCode: 3010, RebootRequired: true}}
	result.Errors = []Error{{At: started, Phase: "search", Class: "transient", HResult: "0x8024402C", Message: "name not resolved"}}
	result.Finish(started.Add(90*time.Minute), StatusPartial, "1 failed")
	return result
}

func TestSampleReportMatchesSchema(t *testing.T) {
	schema := compileSchema(t)
	result := sampleReport()
	if err := validate(t, schema, result); err != nil {
		t.Fatalf("report does not match schema: %v", err)
	}
	if result.Summary.Installed != 1 || result.Summary.Skipped != 1 || result.Summary.Failed != 1 || result.Summary.Discovered != 3 {
		t.Fatalf("summary %+v", result.Summary)
	}
	if result.DurationSeconds != 5400 {
		t.Fatalf("duration %v", result.DurationSeconds)
	}
}

func TestEmptyReportMatchesSchema(t *testing.T) {
	result := New(Plugin{Name: "x", Version: "0.0.0", Commit: "unknown"}, time.Now())
	result.Source.Requested = "default"
	result.Settings.MaxUpdatePasses = 1
	result.Settings.MaxInstallAttempts = 1
	result.Finish(time.Now(), StatusFailed, "detect failed")
	if err := validate(t, compileSchema(t), result); err != nil {
		t.Fatalf("empty report does not match schema: %v", err)
	}
}

func TestSchemaRejectsInvalidStatus(t *testing.T) {
	result := sampleReport()
	result.Status = "great"
	if err := validate(t, compileSchema(t), result); err == nil {
		t.Fatalf("invalid status accepted")
	}
}

func TestWriteCreatesDirectories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "report.json")
	if err := sampleReport().Write(path); err != nil {
		t.Fatalf("Write: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(content, &decoded); err != nil || decoded["schema_version"] != SchemaVersion {
		t.Fatalf("written report invalid: %v %v", err, decoded["schema_version"])
	}
}

func TestReportContainsNoSecrets(t *testing.T) {
	encoded, _ := json.Marshal(sampleReport())
	for _, forbidden := range []string{"password", "username", "wsus_server", "WUServer"} {
		if strings.Contains(strings.ToLower(string(encoded)), strings.ToLower(forbidden)) {
			t.Errorf("report contains %q", forbidden)
		}
	}
}
