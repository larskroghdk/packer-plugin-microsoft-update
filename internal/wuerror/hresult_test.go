package wuerror

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	cases := map[string]uint32{
		"0x80240022":  0x80240022,
		"0X8024402c":  0x8024402C,
		"-2145124318": 0x80240022,
		"2147942501":  2147942501,
		"0":           0,
	}
	for input, want := range cases {
		got, err := Parse(input)
		if err != nil || got != want {
			t.Errorf("Parse(%q) = %#x, %v; want %#x", input, got, err, want)
		}
	}
	for _, invalid := range []string{"", "0xZZ", "99999999999", "abc"} {
		if _, err := Parse(invalid); err == nil {
			t.Errorf("Parse(%q) accepted", invalid)
		}
	}
}

func TestLookupClassifies(t *testing.T) {
	cases := map[uint32]Class{
		0x8024402C: Transient,
		0x80244019: Unsupported,
		0x80072F8F: Unsupported,
		0x80070005: Permanent,
		0x80240017: NotApplicable,
		0x80070BC9: RebootRequired,
		0x80249999: Transient,
		0x80079999: Permanent,
	}
	for code, want := range cases {
		if got := Lookup(code).Class; got != want {
			t.Errorf("Lookup(%s).Class = %s, want %s", Format(code), got, want)
		}
	}
}

func TestDescribeIsActionable(t *testing.T) {
	message := Describe(0x80070422)
	for _, expected := range []string{"0x80070422", "ERROR_SERVICE_DISABLED", "wuauserv"} {
		if !strings.Contains(message, expected) {
			t.Errorf("Describe lacks %q: %s", expected, message)
		}
	}
	if unknown := Describe(0x80071234); !strings.HasPrefix(unknown, "0x80071234: ") {
		t.Errorf("unknown code description %q", unknown)
	}
}

func TestHResultJSON(t *testing.T) {
	var decoded struct {
		Code HResult `json:"code"`
	}
	if err := json.Unmarshal([]byte(`{"code":"0x80240022"}`), &decoded); err != nil || decoded.Code != 0x80240022 {
		t.Fatalf("decode: %v %v", decoded.Code, err)
	}
	encoded, err := json.Marshal(decoded)
	if err != nil || string(encoded) != `{"code":"0x80240022"}` {
		t.Fatalf("encode: %s %v", encoded, err)
	}
}

func TestIsSuccess(t *testing.T) {
	if !IsSuccess(0) || !IsSuccess(0x00240006) || IsSuccess(0x80240022) {
		t.Fatalf("IsSuccess misclassifies codes")
	}
}
