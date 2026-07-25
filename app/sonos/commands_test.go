package sonos

import (
	"encoding/json"
	"testing"
)

func TestBoolOrDefault(t *testing.T) {
	tests := []struct {
		in   string
		def  bool
		want bool
	}{
		{`true`, false, true},
		{`false`, true, false},
		{`"On"`, false, true},
		{`"off"`, true, false},
		{`1`, false, true},
		{`0`, true, false},
		{``, true, true},
		{`null`, true, true},
	}
	for _, tc := range tests {
		if got := boolOrDefault(json.RawMessage(tc.in), tc.def); got != tc.want {
			t.Errorf("boolOrDefault(%q, %v) = %v, want %v", tc.in, tc.def, got, tc.want)
		}
	}
}

func TestAsRepeat(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{`true`, "all", false},
		{`false`, "off", false},
		{`"Off"`, "off", false},
		{`"RepeatAll"`, "all", false},
		{`"RepeatOne"`, "one", false},
		{`"one"`, "one", false},
		{``, "off", false},
		{`"sometimes"`, "", true},
	}
	for _, tc := range tests {
		got, err := asRepeat(json.RawMessage(tc.in))
		if (err != nil) != tc.wantErr {
			t.Errorf("asRepeat(%q) error = %v, wantErr %v", tc.in, err, tc.wantErr)
			continue
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("asRepeat(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestAsDuration(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{`30`, "00:30:00", false},
		{`1`, "00:01:00", false},
		{`"00:15:00"`, "00:15:00", false},
		{``, "", false},
		{`null`, "", false},
		{`false`, "", false},
		{`0`, "", true},
		{`61`, "", true},
	}
	for _, tc := range tests {
		got, err := asDuration(json.RawMessage(tc.in))
		if (err != nil) != tc.wantErr {
			t.Errorf("asDuration(%q) error = %v, wantErr %v", tc.in, err, tc.wantErr)
			continue
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("asDuration(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPlayModeSplitJoin(t *testing.T) {
	modes := []string{"NORMAL", "REPEAT_ALL", "REPEAT_ONE", "SHUFFLE_NOREPEAT", "SHUFFLE", "SHUFFLE_REPEAT_ONE"}
	for _, mode := range modes {
		shuffle, repeat := splitPlayMode(mode)
		if got := joinPlayMode(shuffle, repeat); got != mode {
			t.Errorf("round trip %s -> (%v, %s) -> %s", mode, shuffle, repeat, got)
		}
	}
	// Turning shuffle on while repeating all yields SHUFFLE.
	if got := joinPlayMode(true, "all"); got != "SHUFFLE" {
		t.Errorf("joinPlayMode(true, all) = %s", got)
	}
	// An unknown mode reads as the neutral one.
	if shuffle, repeat := splitPlayMode("SOMETHING_ELSE"); shuffle || repeat != "off" {
		t.Errorf("splitPlayMode(unknown) = (%v, %s)", shuffle, repeat)
	}
}
