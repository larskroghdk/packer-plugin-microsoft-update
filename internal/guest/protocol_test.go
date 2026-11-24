package guest

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestEncodeRequestIsLineSafe(t *testing.T) {
	encoded := string(encodeRequest([]requestParameter{
		{"search_criteria", "IsInstalled=0\r\nand evil=1"},
		{"legacy_filter", "include:$_.Title -like '*KB*'"},
	}))
	lines := strings.Split(strings.TrimSuffix(encoded, "\r\n"), "\r\n")
	if len(lines) != 2 {
		t.Fatalf("expected two lines, got %q", encoded)
	}
	name, value, _ := strings.Cut(lines[0], "=")
	decoded, err := base64.StdEncoding.DecodeString(value)
	if name != "search_criteria" || err != nil || string(decoded) != "IsInstalled=0\r\nand evil=1" {
		t.Fatalf("value did not round-trip: %q %q %v", name, decoded, err)
	}
}

func TestParseRecord(t *testing.T) {
	parsed, isRecord, err := parseRecord(recordPrefix + `{"type":"log","level":"info","message":"x"}`)
	if !isRecord || err != nil || parsed.Type != "log" {
		t.Fatalf("unexpected result %+v %v %v", parsed, isRecord, err)
	}
	if _, isRecord, _ := parseRecord("plain output"); isRecord {
		t.Fatalf("plain output parsed as a record")
	}
	if _, isRecord, err := parseRecord(recordPrefix + `{broken`); !isRecord || err == nil {
		t.Fatalf("malformed record not reported")
	}
}

func TestPowerShellQuote(t *testing.T) {
	cases := map[string]string{
		"plain":    "'plain'",
		"it's":     "'it''s'",
		"‘smart’":  "'‘‘smart’’'",
		`$(x) "y"`: `'$(x) "y"'`,
	}
	for input, want := range cases {
		if got := powerShellQuote(input); got != want {
			t.Errorf("powerShellQuote(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestEncodedPowerShellCommandRoundTrip(t *testing.T) {
	script := "Write-Output 'héllo'"
	if got := decodeEncodedCommand(encodedPowerShellCommand(script)); got != script {
		t.Fatalf("round trip produced %q", got)
	}
}

func TestRunnerTemplateHasNoUnrenderedPlaceholders(t *testing.T) {
	var builder strings.Builder
	err := runnerTemplate.Execute(&builder, runnerOptions{
		TaskName: "task", ScriptPath: `C:\x.ps1`, RequestPath: `C:\x.request`, LogPath: `C:\x.log`,
		Username: "SYSTEM", ExecutionTimeLimit: "PT5H",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(builder.String(), "<%") || strings.Contains(builder.String(), "%>") {
		t.Fatalf("unrendered placeholders:\n%s", builder.String())
	}
	if !strings.Contains(builder.String(), `{{"type":"heartbeat"`) {
		t.Fatalf("PowerShell format braces must survive rendering")
	}
}
