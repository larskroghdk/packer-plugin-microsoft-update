package guest

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/hashicorp/packer-plugin-sdk/packer"
)

type fakeResponse struct {
	lines      []string
	exitStatus int
	err        error
	hang       bool
}

type fakeCommunicator struct {
	mutex    sync.Mutex
	uploads  map[string][]byte
	commands []string
	handler  func(communicator *fakeCommunicator, command string) fakeResponse
}

func newFakeCommunicator(handler func(communicator *fakeCommunicator, command string) fakeResponse) *fakeCommunicator {
	return &fakeCommunicator{uploads: map[string][]byte{}, handler: handler}
}

func (f *fakeCommunicator) Start(ctx context.Context, command *packer.RemoteCmd) error {
	f.mutex.Lock()
	f.commands = append(f.commands, command.Command)
	f.mutex.Unlock()
	response := f.handler(f, command.Command)
	if response.err != nil {
		return response.err
	}
	go func() {
		for _, line := range response.lines {
			_, _ = fmt.Fprintln(command.Stdout, line)
		}
		if response.hang {
			<-ctx.Done()
			command.SetExited(-1)
			return
		}
		command.SetExited(response.exitStatus)
	}()
	return nil
}

func (f *fakeCommunicator) Upload(path string, content io.Reader, _ *os.FileInfo) error {
	data, err := io.ReadAll(content)
	if err != nil {
		return err
	}
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.uploads[path] = data
	return nil
}

func (f *fakeCommunicator) UploadDir(string, string, []string) error { return nil }
func (f *fakeCommunicator) Download(string, io.Writer) error         { return nil }
func (f *fakeCommunicator) DownloadDir(string, string, []string) error {
	return nil
}

func (f *fakeCommunicator) requestFor(command string) map[string][]string {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	start := strings.Index(command, `-File "`)
	runnerPath := strings.TrimSuffix(command[start+len(`-File "`):], `"`)
	requestPath := uploadPath(strings.TrimSuffix(runnerPath, ".runner.ps1") + ".request")
	request := map[string][]string{}
	for _, line := range strings.Split(string(f.uploads[requestPath]), "\r\n") {
		name, encoded, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		value, _ := base64.StdEncoding.DecodeString(encoded)
		request[name] = append(request[name], string(value))
	}
	return request
}

func (f *fakeCommunicator) commandCount(substring string) int {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	count := 0
	for _, command := range f.commands {
		if strings.Contains(command, substring) || strings.Contains(decodeEncodedCommand(command), substring) {
			count++
		}
	}
	return count
}

func decodeEncodedCommand(command string) string {
	_, encoded, found := strings.Cut(command, "-EncodedCommand ")
	if !found {
		return ""
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return ""
	}
	codeUnits := make([]uint16, len(raw)/2)
	for index := range codeUnits {
		codeUnits[index] = uint16(raw[index*2]) | uint16(raw[index*2+1])<<8
	}
	return string(utf16.Decode(codeUnits))
}

func bootstrapResponse(bootTime string) fakeResponse {
	return fakeResponse{lines: []string{recordPrefix + `{"type":"bootstrap","system_root":"C:\\Windows","boot_time":"` + bootTime + `","powershell_version":"5.1"}`}}
}

func runnerResponse(records ...string) fakeResponse {
	lines := make([]string, 0, len(records)+1)
	for _, line := range records {
		lines = append(lines, recordPrefix+line)
	}
	lines = append(lines, runnerPrefix+`{"exit_code":0}`)
	return fakeResponse{lines: lines}
}

type recordingLogger struct {
	mutex    sync.Mutex
	messages []string
}

func (l *recordingLogger) add(level, message string) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.messages = append(l.messages, level+": "+message)
}

func (l *recordingLogger) Info(message string)  { l.add("info", message) }
func (l *recordingLogger) Warn(message string)  { l.add("warn", message) }
func (l *recordingLogger) Debug(message string) { l.add("debug", message) }

func (l *recordingLogger) contains(substring string) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	for _, message := range l.messages {
		if strings.Contains(message, substring) {
			return true
		}
	}
	return false
}

func testTimeouts() Timeouts {
	return Timeouts{
		Startup: time.Second, Search: time.Second, Download: time.Second, Install: time.Second,
		Servicing: time.Second, Reboot: 2 * time.Second, Command: time.Second, Upload: time.Second,
	}
}

func newTestClient(communicator *fakeCommunicator) (*Client, *recordingLogger) {
	logger := &recordingLogger{}
	ui := &packer.BasicUi{Reader: new(bytes.Buffer), Writer: io.Discard, ErrorWriter: io.Discard}
	client := NewClient(communicator, ui, logger, Options{Username: "SYSTEM", Timeouts: testTimeouts(), PollInterval: 10 * time.Millisecond})
	return client, logger
}
