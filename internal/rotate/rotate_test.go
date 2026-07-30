package rotate

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFilename(t *testing.T) {
	at := time.Date(2026, 7, 28, 21, 30, 45, 120*int(time.Millisecond), time.UTC)

	// if follows the documented behavioral contract. See P0007 5.10.
	if got := Filename(`C:\app\log\out.log`, at); got != `C:\app\log\out-20260728T213045.120.log` {
		t.Errorf("Filename = %q", got)
	}
	// if follows the documented behavioral contract.
	if got := Filename(`C:\app\log\out`, at); got != `C:\app\log\out-20260728T213045.120` {
		t.Errorf("Filename(no ext) = %q", got)
	}
	// This section follows the documented behavioral contract. See UTC.
	local := at.In(time.FixedZone("KST", 9*3600))
	if got := Filename(`out.log`, local); got != `out-20260728T213045.120.log` {
		t.Errorf("Filename(local) = %q, want UTC conversion", got)
	}
}

func TestNeededMissingFile(t *testing.T) {
	// if follows the documented behavioral contract.
	if ok, _ := Needed(FileInfo{}, Criteria{}, time.Now()); ok {
		t.Error("Needed(missing) = true, want false")
	}
}

func TestNeededNoCriteriaAlwaysRotates(t *testing.T) {
	now := time.Now()
	info := FileInfo{Exists: true, LastWrite: now, Size: 1}
	ok, at := Needed(info, Criteria{}, now)
	if !ok {
		t.Fatal("Needed = false, want true (always rotate when both criteria are zero)")
	}
	if !at.Equal(now) {
		t.Errorf("timestamp = %v, want file's last write %v", at, now)
	}
}

func TestNeededAgeAndSizeAreAND(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	old := now.Add(-2 * time.Hour)
	c := Criteria{MaxAge: time.Hour, MinSize: 1000}

	// if follows the documented behavioral contract. See OR.
	if ok, _ := Needed(FileInfo{Exists: true, LastWrite: old, Size: 10}, c, now); ok {
		t.Error("old but small: Needed = true, want false (AND criteria)")
	}
	// if follows the documented behavioral contract.
	if ok, _ := Needed(FileInfo{Exists: true, LastWrite: now, Size: 9999}, c, now); ok {
		t.Error("big but young: Needed = true, want false (AND criteria)")
	}
	// This section follows the documented behavioral contract.
	ok, at := Needed(FileInfo{Exists: true, LastWrite: old, Size: 9999}, c, now)
	if !ok {
		t.Fatal("old and big: Needed = false, want true")
	}
	if !at.Equal(old) {
		t.Errorf("timestamp = %v, want %v (last-write time, not current time)", at, old)
	}
}

func TestNeededSingleCriterion(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	old := now.Add(-2 * time.Hour)

	// if follows the documented behavioral contract.
	if ok, _ := Needed(FileInfo{Exists: true, LastWrite: old, Size: 1}, Criteria{MaxAge: time.Hour}, now); !ok {
		t.Error("age only: Needed = false, want true")
	}
	// if follows the documented behavioral contract.
	if ok, _ := Needed(FileInfo{Exists: true, LastWrite: now, Size: 9999}, Criteria{MinSize: 1000}, now); !ok {
		t.Error("size only: Needed = false, want true")
	}
}

func TestNeededStatFailed(t *testing.T) {
	now := time.Now()
	ok, at := Needed(FileInfo{Exists: true, StatFailed: true}, Criteria{MaxAge: time.Hour, MinSize: 1 << 40}, now)
	if !ok {
		t.Fatal("stat failed: Needed = false, want true (there is no basis for evaluating the criteria)")
	}
	if !at.Equal(now) {
		t.Errorf("timestamp = %v, want now", at)
	}
}

func TestStatReportsMissingDirectoryAndFile(t *testing.T) {
	dir := t.TempDir()
	if info := Stat(filepath.Join(dir, "absent.log")); info.Exists {
		t.Errorf("Stat(missing) = %+v, want not exists", info)
	}
	if info := Stat(dir); info.Exists {
		t.Errorf("Stat(directory) = %+v, want not exists (directories are not rotated)", info)
	}
	path := filepath.Join(dir, "out.log")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	info := Stat(path)
	if !info.Exists || info.StatFailed || info.Size != 5 {
		t.Errorf("Stat(file) = %+v", info)
	}
}

func TestApplyRenamesAndKeepsContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.log")
	if err := os.WriteFile(path, []byte("old content"), 0o600); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 7, 28, 21, 30, 45, 120*int(time.Millisecond), time.UTC)
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}

	target, err := Apply(path, Criteria{}, Options{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "out-20260728T213045.120.log"); target != want {
		t.Fatalf("Apply = %q, want %q", target, want)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("original still exists after rename: %v", err)
	}
	moved, err := os.ReadFile(target)
	if err != nil || string(moved) != "old content" {
		t.Errorf("rotated content = %q, %v", moved, err)
	}
}

func TestApplySkipsWhenCriteriaAreNotMet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.log")
	if err := os.WriteFile(path, []byte("tiny"), 0o600); err != nil {
		t.Fatal(err)
	}
	target, err := Apply(path, Criteria{MinSize: 1 << 20}, Options{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if target != "" {
		t.Errorf("Apply = %q, want no rotation", target)
	}
	if _, err = os.Stat(path); err != nil {
		t.Errorf("original must survive: %v", err)
	}
}

func TestApplyMissingFileIsNotAnError(t *testing.T) {
	target, err := Apply(filepath.Join(t.TempDir(), "absent.log"), Criteria{}, Options{}, time.Now())
	if err != nil || target != "" {
		t.Errorf("Apply(missing) = %q, %v", target, err)
	}
}

func TestApplyCopyAndTruncateLeavesEmptyOriginal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.log")
	if err := os.WriteFile(path, []byte("copied away"), 0o600); err != nil {
		t.Fatal(err)
	}
	var slept []time.Duration
	options := Options{
		CopyAndTruncate: true,
		Delay:           250 * time.Millisecond,
		Sleep:           func(d time.Duration) { slept = append(slept, d) },
	}
	target, err := Apply(path, Criteria{}, options, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	copied, err := os.ReadFile(target)
	if err != nil || string(copied) != "copied away" {
		t.Fatalf("copy = %q, %v", copied, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("original must survive copy and truncate: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("original size = %d, want 0", info.Size())
	}
	if len(slept) != 1 || slept[0] != 250*time.Millisecond {
		t.Errorf("sleeps = %v, want one AppRotateDelay", slept)
	}
}

func TestSizeLimit(t *testing.T) {
	if got := SizeLimit(10485760, 0); got != 10485760 {
		t.Errorf("SizeLimit = %d", got)
	}
	if got := SizeLimit(0, 1); got != 1<<32 {
		t.Errorf("SizeLimit(high) = %d, want %d", got, int64(1)<<32)
	}
}
