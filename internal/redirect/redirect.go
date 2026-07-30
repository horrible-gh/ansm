// Package redirect contains platform-independent standard-stream redirection and relay decisions (L0008 2.13-2.15; P0007 5.10).
package redirect

import (
	"path/filepath"
	"strings"
	"time"

	"ansm/internal/rotate"
)

// Stream describes one redirected handle. Empty Path disables it; remaining values pass through to CreateFileW using NSSM-compatible numeric contracts.
type Stream struct {
	Path                string
	ShareMode           uint32
	CreationDisposition uint32
	FlagsAndAttributes  uint32
	// CopyAndTruncate follows the documented behavioral contract. See CopyAndTruncate, AppStdoutCopyAndTruncate, AppStderrCopyAndTruncate.
	CopyAndTruncate bool
}

// Enabled follows the documented behavioral contract. See Enabled.
func (s Stream) Enabled() bool { return s.Path != "" }

// Config follows the documented behavioral contract. See Config.
type Config struct {
	Stdin  Stream
	Stdout Stream
	Stderr Stream

	// Timestamp follows the documented behavioral contract. See Timestamp, AppTimestampLog, UTC.
	Timestamp bool
	// RotateFiles follows the documented behavioral contract. See RotateFiles, AppRotateFiles.
	RotateFiles bool
	// RotateOnline follows the documented behavioral contract. See RotateOnline, AppRotateOnline.
	RotateOnline bool
	// RotateSeconds follows the documented behavioral contract. See RotateSeconds.
	RotateSeconds uint32
	// RotateBytes follows the documented behavioral contract. See RotateBytes, High.
	RotateBytes int64
	// RotateDelay follows the documented behavioral contract. See RotateDelay.
	RotateDelay time.Duration
}

// Any follows the documented behavioral contract. See Any.
func (c Config) Any() bool {
	return c.Stdin.Enabled() || c.Stdout.Enabled() || c.Stderr.Enabled()
}

// Relayed requires a pipe when timestamps or online rotation need parent-side inspection. Otherwise the file handle is inherited directly for NSSM-equivalent throughput. Stdin is never relayed.
func (c Config) Relayed(s Stream) bool {
	if !s.Enabled() {
		return false
	}
	return c.Timestamp || (c.RotateFiles && c.RotateOnline)
}

// Online enables runtime rotation only when both AppRotateOnline and AppRotateFiles are enabled.
func (c Config) Online() bool { return c.RotateFiles && c.RotateOnline }

// Criteria follows the documented behavioral contract. See Criteria, L0008 2.14.
func (c Config) Criteria() rotate.Criteria {
	return rotate.Criteria{
		MaxAge:  time.Duration(c.RotateSeconds) * time.Second,
		MinSize: c.RotateBytes,
	}
}

// SameTarget detects stdout and stderr destinations that must share one handle to prevent overlapping writes and independent file offsets.
func (c Config) SameTarget() bool {
	if !c.Stdout.Enabled() || !c.Stderr.Enabled() {
		return false
	}
	return SamePath(c.Stdout.Path, c.Stderr.Path)
}

// SamePath compares Windows paths case-insensitively and treats both slash forms as separators.
func SamePath(a, b string) bool {
	return strings.EqualFold(normalize(a), normalize(b))
}

func normalize(path string) string {
	return filepath.Clean(strings.ReplaceAll(path, "/", `\`))
}
