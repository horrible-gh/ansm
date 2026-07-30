package redirect

import (
	"testing"
	"time"
)

func TestRelayedNeedsTimestampOrOnlineRotation(t *testing.T) {
	stream := Stream{Path: `C:\logs\out.log`}
	tests := []struct {
		name   string
		config Config
		want   bool
	}{
		{"plain", Config{}, false},
		{"timestamp", Config{Timestamp: true}, true},
		{"online without rotate files", Config{RotateOnline: true}, false},
		{"online", Config{RotateFiles: true, RotateOnline: true}, true},
		{"rotate at start only", Config{RotateFiles: true}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.config.Relayed(stream); got != test.want {
				t.Errorf("Relayed() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestRelayedIgnoresDisabledStream(t *testing.T) {
	config := Config{Timestamp: true, RotateFiles: true, RotateOnline: true}
	if config.Relayed(Stream{}) {
		t.Error("a stream without a path must never be relayed")
	}
	if config.Any() {
		t.Error("Any() must be false when no stream has a path")
	}
}

func TestSameTargetIgnoresCaseAndSeparator(t *testing.T) {
	tests := []struct {
		name         string
		out, err     string
		wantSameFile bool
	}{
		{"identical", `C:\logs\svc.log`, `C:\logs\svc.log`, true},
		{"case", `C:\Logs\Svc.log`, `c:\logs\svc.LOG`, true},
		{"separator", `C:/logs/svc.log`, `C:\logs\svc.log`, true},
		{"dot segment", `C:\logs\.\svc.log`, `C:\logs\svc.log`, true},
		{"different", `C:\logs\out.log`, `C:\logs\err.log`, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := Config{Stdout: Stream{Path: test.out}, Stderr: Stream{Path: test.err}}
			if got := config.SameTarget(); got != test.wantSameFile {
				t.Errorf("SameTarget() = %v, want %v", got, test.wantSameFile)
			}
		})
	}
}

func TestSameTargetNeedsBothStreams(t *testing.T) {
	config := Config{Stdout: Stream{Path: `C:\logs\svc.log`}}
	if config.SameTarget() {
		t.Error("SameTarget() must be false when stderr is not redirected")
	}
}

func TestCriteriaConvertsSecondsAndBytes(t *testing.T) {
	config := Config{RotateSeconds: 90, RotateBytes: 4096}
	criteria := config.Criteria()
	if criteria.MaxAge != 90*time.Second || criteria.MinSize != 4096 {
		t.Errorf("Criteria() = %+v", criteria)
	}
}

func TestOnlineNeedsRotateFiles(t *testing.T) {
	if (Config{RotateOnline: true}).Online() {
		t.Error("AppRotateOnline alone must not enable online rotation")
	}
	if !(Config{RotateFiles: true, RotateOnline: true}).Online() {
		t.Error("both settings must enable online rotation")
	}
}
