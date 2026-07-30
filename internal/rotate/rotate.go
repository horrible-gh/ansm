// Package rotate implements log-rotation decisions, filenames, and file replacement (L0008 2.14; P0007 5.10).
package rotate

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Filename inserts a UTC YYYYMMDDThhmmss.mmm timestamp before the final extension, matching NSSM GetSystemTime behavior.
func Filename(path string, at time.Time) string {
	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(path, ext)
	t := at.UTC()
	stamp := fmt.Sprintf("%04d%02d%02dT%02d%02d%02d.%03d",
		t.Year(), int(t.Month()), t.Day(),
		t.Hour(), t.Minute(), t.Second(), t.Nanosecond()/int(time.Millisecond))
	return stem + "-" + stamp + ext
}

// Criteria follows the documented behavioral contract. See Criteria, AppRotateSeconds, AppRotateBytes, High.
type Criteria struct {
	// MaxAge follows the documented behavioral contract. See MaxAge.
	MaxAge time.Duration
	// MinSize follows the documented behavioral contract. See MinSize.
	MinSize int64
}

// FileInfo follows the documented behavioral contract. See FileInfo.
type FileInfo struct {
	// Exists follows the documented behavioral contract. See Exists.
	Exists bool
	// StatFailed follows the documented behavioral contract. See StatFailed, L0008 2.14.
	StatFailed bool
	LastWrite  time.Time
	Size       int64
}

// Needed rotates only when every configured criterion is met: age and size are AND conditions, not OR conditions. It returns the original last-write time for the rotated filename.
func Needed(info FileInfo, c Criteria, now time.Time) (bool, time.Time) {
	if !info.Exists {
		return false, time.Time{}
	}
	if info.StatFailed {
		// return follows the documented behavioral contract.
		return true, now
	}

	if c.MaxAge > 0 && info.LastWrite.After(now.Add(-c.MaxAge)) {
		return false, time.Time{} // return follows the documented contract.
	}
	if c.MinSize > 0 && info.Size < c.MinSize {
		return false, time.Time{} // return follows the documented contract.
	}
	return true, info.LastWrite
}

// Options follows the documented behavioral contract. See Options.
type Options struct {
	// CopyAndTruncate follows the documented behavioral contract. See CopyAndTruncate, P0007 5.10.
	CopyAndTruncate bool
	// Delay follows the documented behavioral contract. See Delay, AppRotateDelay.
	Delay time.Duration
	// Sleep follows the documented behavioral contract. See Sleep.
	Sleep func(time.Duration)
}

// Stat follows the documented behavioral contract. See Stat.
func Stat(path string) FileInfo {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return FileInfo{}
		}
		return FileInfo{Exists: true, StatFailed: true}
	}
	if info.IsDir() {
		return FileInfo{}
	}
	return FileInfo{Exists: true, LastWrite: info.ModTime(), Size: info.Size()}
}

// Apply rotates path when Needed permits it. Empty Criteria rotate any existing file; copy-and-truncate is used when another process prevents renaming.
func Apply(path string, c Criteria, o Options, now time.Time) (string, error) {
	needed, at := Needed(Stat(path), c, now)
	if !needed {
		return "", nil
	}
	target := Filename(path, at)
	if !o.CopyAndTruncate {
		if err := os.Rename(path, target); err != nil {
			return "", err
		}
		return target, nil
	}
	if err := copyFile(path, target); err != nil {
		return "", err
	}
	if o.Delay > 0 {
		sleep := o.Sleep
		if sleep == nil {
			sleep = time.Sleep
		}
		sleep(o.Delay)
	}
	// if follows the documented behavioral contract.
	if err := os.Truncate(path, 0); err != nil {
		return target, err
	}
	return target, nil
}

func copyFile(from, to string) error {
	source, err := os.Open(from)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666)
	if err != nil {
		return err
	}
	if _, err = io.Copy(target, source); err != nil {
		target.Close()
		return err
	}
	return target.Close()
}

// SizeLimit combines AppRotateBytes and AppRotateBytesHigh into one 64-bit threshold. Startup and online rotation intentionally receive the same correctly ordered value.
func SizeLimit(low, high uint32) int64 {
	return int64(uint64(high)<<32 | uint64(low))
}
