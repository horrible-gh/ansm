// Package rotate 는 로그 갈아끼우기 판정과 이름 산출을 담는다.
//
// L0008 2.14, P0007 5.10.
package rotate

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Filename 은 갈아끼운 파일의 이름을 만든다.
//
//	<원래이름>-<YYYYMMDD>T<hhmmss>.<mmm><원래확장자>
//
// 확장자는 마지막 점 이후로 본다. 확장자가 없으면 시각 문자열이 이름 끝에 그냥 붙는다.
// 시각은 UTC 다(L0008 2.14). 원본이 GetSystemTime 을 쓰는 것과 같다.
func Filename(path string, at time.Time) string {
	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(path, ext)
	t := at.UTC()
	stamp := fmt.Sprintf("%04d%02d%02dT%02d%02d%02d.%03d",
		t.Year(), int(t.Month()), t.Day(),
		t.Hour(), t.Minute(), t.Second(), t.Nanosecond()/int(time.Millisecond))
	return stem + "-" + stamp + ext
}

// Criteria 는 갈아끼우기 판정 기준이다. AppRotateSeconds / AppRotateBytes(+High).
type Criteria struct {
	// MaxAge 가 0 이면 나이 기준을 보지 않는다.
	MaxAge time.Duration
	// MinSize 가 0 이면 크기 기준을 보지 않는다.
	MinSize int64
}

// FileInfo 는 판정에 필요한 대상 파일의 상태다.
type FileInfo struct {
	// Exists 가 false 면 갈아끼울 것이 없다.
	Exists bool
	// StatFailed 는 파일은 있으나 상태를 읽지 못했다는 뜻이다.
	// L0008 2.14: 이때는 마지막 기록 시각을 현재 시각으로 보고 두 기준을 모두 무시한다.
	StatFailed bool
	LastWrite  time.Time
	Size       int64
}

// Needed 는 지금 갈아끼워야 하는지 판정한다. L0008 2.14 의 rotate_if_needed() 앞부분.
//
// **나이 기준과 크기 기준은 OR 가 아니라 AND 다.** 둘 다 설정돼 있으면 둘 다
// 넘겨야 갈아끼운다. 하나만 설정하면 그 하나만 본다. 둘 다 0 이면 무조건 갈아끼운다.
//
// 두 번째 반환값은 갈아끼운 파일 이름에 쓸 시각이다. 현재 시각이 아니라
// **원본 파일의 마지막 기록 시각**이다.
func Needed(info FileInfo, c Criteria, now time.Time) (bool, time.Time) {
	if !info.Exists {
		return false, time.Time{}
	}
	if info.StatFailed {
		// 상태를 못 읽었으면 기준을 따질 근거가 없다. 무조건 갈아끼운다.
		return true, now
	}

	if c.MaxAge > 0 && info.LastWrite.After(now.Add(-c.MaxAge)) {
		return false, time.Time{} // 아직 젊다
	}
	if c.MinSize > 0 && info.Size < c.MinSize {
		return false, time.Time{} // 아직 작다
	}
	return true, info.LastWrite
}

// Options 는 갈아끼우기를 실제로 실행하는 방식이다.
type Options struct {
	// CopyAndTruncate 가 참이면 이름을 바꾸는 대신 복사한 뒤 원본을 0 으로 자른다.
	// 누군가 파일을 붙들고 있어 이름을 바꿀 수 없을 때 쓴다(P0007 5.10).
	CopyAndTruncate bool
	// Delay 는 복사와 자르기 사이의 쉼이다. AppRotateDelay.
	// 복사가 끝나기를 기다리는 느린 저장 장치를 위한 여유다.
	Delay time.Duration
	// Sleep 이 nil 이면 실제 시계를 쓴다. 시험에서 갈아끼운다.
	Sleep func(time.Duration)
}

// Stat 는 판정에 필요한 파일 상태를 읽는다.
//
// 파일이 없으면 갈아끼울 것도 없다. 폴더는 로그 파일이 아니므로 같게 다룬다 —
// 이름을 바꿔 버리면 안 되고, 뒤이은 파일 열기가 제대로 실패해 줄 것이다.
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

// Apply 는 필요하면 path 를 갈아끼우고 새로 생긴 이름을 돌려준다.
// 갈아끼우지 않았으면 빈 이름과 nil 을 돌려준다.
//
// 빈 Criteria 를 넘기면 파일이 있는 한 무조건 갈아끼운다. 실행 중 갈아끼우기와
// 관리 명령 rotate 가 이 형태를 쓴다.
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
	// 자르기까지 실패하면 다음 갈아끼우기가 같은 내용을 또 복사하게 되므로
	// 이름은 돌려주되 오류를 함께 알린다.
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

// SizeLimit 은 AppRotateBytes 와 AppRotateBytesHigh 를 하나의 64비트 값으로 합친다.
//
// L0008 2.14 말미: 원본은 시작 시 갈아끼우기 호출에서 인수 순서를 잘못 넘겨
// 크기 기준이 반영되지 않는 결함이 있다. 이식본은 선언된 의미대로 올바르게
// 전달하기로 확정했으므로, 시작 시와 실행 중이 같은 값을 본다.
func SizeLimit(low, high uint32) int64 {
	return int64(uint64(high)<<32 | uint64(low))
}
