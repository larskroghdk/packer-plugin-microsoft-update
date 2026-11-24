package guest

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf16"
)

const (
	recordPrefix = "@@MU@@ "
	runnerPrefix = "@@MU-RUNNER@@ "
)

type record struct {
	Type string
	Raw  json.RawMessage
}

func (r record) decode(target any) error {
	if err := json.Unmarshal(r.Raw, target); err != nil {
		return fmt.Errorf("decode %s record: %w", r.Type, err)
	}
	return nil
}

type recordHeader struct {
	Type string `json:"type"`
}
type logRecord struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}
type phaseRecord struct {
	Name string `json:"name"`
}
type heartbeatRecord struct {
	CPUPercent    int `json:"cpu_percent"`
	MemoryPercent int `json:"memory_percent"`
}
type runnerFooter struct {
	ExitCode int `json:"exit_code"`
}
type errorRecord struct {
	HResult  string `json:"hresult"`
	Message  string `json:"message"`
	Position string `json:"position"`
}

func parseRecord(line string) (parsed record, ok bool, err error) {
	if !strings.HasPrefix(line, recordPrefix) {
		return record{}, false, nil
	}
	payload := strings.TrimPrefix(line, recordPrefix)
	var header recordHeader
	if err := json.Unmarshal([]byte(payload), &header); err != nil {
		return record{}, true, fmt.Errorf("malformed guest record %q: %w", payload, err)
	}
	if header.Type == "" {
		return record{}, true, fmt.Errorf("guest record without type: %q", payload)
	}
	return record{Type: header.Type, Raw: json.RawMessage(payload)}, true, nil
}

func parseRunnerFooter(line string) (runnerFooter, bool) {
	if !strings.HasPrefix(line, runnerPrefix) {
		return runnerFooter{}, false
	}
	var footer runnerFooter
	if err := json.Unmarshal([]byte(strings.TrimPrefix(line, runnerPrefix)), &footer); err != nil {
		return runnerFooter{}, false
	}
	return footer, true
}

type requestParameter struct {
	Name  string
	Value string
}

func encodeRequest(parameters []requestParameter) []byte {
	var builder strings.Builder
	for _, parameter := range parameters {
		builder.WriteString(parameter.Name)
		builder.WriteByte('=')
		builder.WriteString(base64.StdEncoding.EncodeToString([]byte(parameter.Value)))
		builder.WriteString("\r\n")
	}
	return []byte(builder.String())
}

func encodedPowerShellCommand(script string) string {
	codeUnits := utf16.Encode([]rune(script))
	encoded := make([]byte, len(codeUnits)*2)
	for index, codeUnit := range codeUnits {
		encoded[index*2] = byte(codeUnit)
		encoded[index*2+1] = byte(codeUnit >> 8)
	}
	return "powershell.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -EncodedCommand " +
		base64.StdEncoding.EncodeToString(encoded)
}

func powerShellQuote(value string) string {
	replacer := strings.NewReplacer(
		"'", "''",
		"‘", "‘‘",
		"’", "’’",
		"‚", "‚‚",
		"‛", "‛‛",
	)
	return "'" + replacer.Replace(value) + "'"
}
