// Package redirect 는 표준 입출력 돌리기의 설정 묶음과 그 판정을 담는다.
//
// L0008 2.13(손잡이 얻기)·2.14(갈아끼우기)·2.15(중계), P0007 5.10.
// 파일 손잡이를 실제로 여는 일은 internal/platform 이 맡고, 이 패키지는
// 어느 운영체제에도 매이지 않는 순수 판정만 담는다.
package redirect

import (
	"path/filepath"
	"strings"
	"time"

	"ansm/internal/rotate"
)

// Stream 은 돌릴 통로 하나의 설정이다.
//
// Path 가 비어 있으면 그 통로는 돌리지 않는다. 나머지 셋은 CreateFileW 에
// 그대로 넘어가는 값이라 원본과 같은 숫자를 쓴다(AppStd*ShareMode,
// AppStd*CreationDisposition, AppStd*FlagsAndAttributes).
type Stream struct {
	Path                string
	ShareMode           uint32
	CreationDisposition uint32
	FlagsAndAttributes  uint32
	// CopyAndTruncate 는 이름을 바꾸는 대신 복사한 뒤 원본을 0 으로 자른다.
	// 다른 프로세스가 파일을 붙들고 있어 이름을 바꿀 수 없을 때 쓴다.
	// AppStdoutCopyAndTruncate / AppStderrCopyAndTruncate.
	CopyAndTruncate bool
}

// Enabled 는 이 통로를 돌리기로 설정했는지 알려준다.
func (s Stream) Enabled() bool { return s.Path != "" }

// Config 는 서비스 한 번의 실행에 쓰는 돌리기 설정 전체다.
type Config struct {
	Stdin  Stream
	Stdout Stream
	Stderr Stream

	// Timestamp 는 AppTimestampLog 다. 켜면 줄머리에 UTC 시각을 붙인다.
	Timestamp bool
	// RotateFiles 는 AppRotateFiles 다. 끄면 갈아끼우기를 아예 하지 않는다.
	RotateFiles bool
	// RotateOnline 은 AppRotateOnline 이다. 자식을 멈추지 않고 갈아끼운다.
	RotateOnline bool
	// RotateSeconds 는 시작 시 갈아끼우기의 나이 기준이다.
	RotateSeconds uint32
	// RotateBytes 는 시작 시 갈아끼우기의 크기 기준이다(High 를 합친 64비트).
	RotateBytes int64
	// RotateDelay 는 복사 후 자르기에서 자르기 전 쉼이다.
	RotateDelay time.Duration
}

// Any 는 돌릴 통로가 하나라도 있는지 알려준다.
// 하나도 없으면 자식은 아무 표준 손잡이도 물려받지 않는다.
func (c Config) Any() bool {
	return c.Stdin.Enabled() || c.Stdout.Enabled() || c.Stderr.Enabled()
}

// Relayed 는 그 통로를 파이프로 받아 중계해야 하는지 판정한다.
//
// L0008 2.15: 줄머리 시각을 붙이거나 실행 중 갈아끼우려면 부모가 내용을
// 들여다봐야 하므로 파이프가 필요하다. 둘 다 아니면 파일 손잡이를 자식에게
// 곧바로 넘겨 부모가 중간에 끼지 않는다 — 자식이 아무리 많이 찍어도
// 부모를 거치지 않으므로 원본과 같은 성능이 나온다.
//
// **표준 입력은 읽기 통로라 중계 대상이 아니다.** 언제나 곧바로 넘긴다.
func (c Config) Relayed(s Stream) bool {
	if !s.Enabled() {
		return false
	}
	return c.Timestamp || (c.RotateFiles && c.RotateOnline)
}

// Online 은 실행 중 갈아끼우기가 실제로 동작하는 상태인지 알려준다.
// AppRotateOnline 만 켜고 AppRotateFiles 를 끄면 아무 일도 하지 않는다.
func (c Config) Online() bool { return c.RotateFiles && c.RotateOnline }

// Criteria 는 시작 시 갈아끼우기 판정 기준이다. L0008 2.14.
//
// 실행 중 갈아끼우기는 사람이 시킨 것이므로 기준을 보지 않는다(빈 Criteria).
func (c Config) Criteria() rotate.Criteria {
	return rotate.Criteria{
		MaxAge:  time.Duration(c.RotateSeconds) * time.Second,
		MinSize: c.RotateBytes,
	}
}

// SameTarget 은 표준 출력과 표준 오류가 같은 파일을 가리키는지 본다.
//
// 이때는 손잡이 하나를 나눠 써야 두 통로의 줄이 서로 끼어들거나 각자의 쓰기
// 위치가 어긋나 앞 내용을 덮어쓰는 일이 없다. 원본도 같은 이유로 손잡이를
// 복제해 넘긴다.
func (c Config) SameTarget() bool {
	if !c.Stdout.Enabled() || !c.Stderr.Enabled() {
		return false
	}
	return SamePath(c.Stdout.Path, c.Stderr.Path)
}

// SamePath 는 두 경로가 같은 파일을 가리키는지 이름만으로 견준다.
// Windows 파일 이름은 대소문자를 구분하지 않고 구분자도 두 가지다.
func SamePath(a, b string) bool {
	return strings.EqualFold(normalize(a), normalize(b))
}

func normalize(path string) string {
	return filepath.Clean(strings.ReplaceAll(path, "/", `\`))
}
