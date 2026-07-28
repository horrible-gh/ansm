// Package exitaction 은 자식의 종료 코드에 붙는 조치를 다룬다.
//
// L0008 2.5 (종료 조치 되짚기), P0007 3.2 (표기).
package exitaction

import (
	"strings"

	"ansm/internal/params"
)

// Action 은 자식이 끝났을 때 감독자가 취할 조치다. 값은 L0008 1.4 를 따른다.
type Action int

const (
	// Restart 는 자식을 다시 띄운다. 되짚기에 실패했을 때의 기본값이기도 하다.
	Restart Action = 0
	// Ignore 는 서비스를 실행 중인 채로 두고 아무 것도 하지 않는다.
	Ignore Action = 1
	// Exit 는 서비스를 곱게 접는다.
	Exit Action = 2
	// Suicide 는 상태를 보고하지 않고 프로세스 자체를 끝낸다.
	Suicide Action = 3
)

// names 의 순서가 곧 Action 의 값이다. P0007 3.10 의 안내 문구 순서이기도 하다.
var names = [...]string{"Restart", "Ignore", "Exit", "Suicide"}

// String 은 레지스트리에 저장되는 표기를 돌려준다.
func (a Action) String() string {
	if a < 0 || int(a) >= len(names) {
		return names[Restart]
	}
	return names[a]
}

// Names 는 유효한 조치 이름을 저장 순서대로 돌려준다.
func Names() []string {
	out := make([]string, len(names))
	copy(out, names[:])
	return out
}

// Parse 는 저장된 문자열을 조치로 바꾼다.
//
// L0008 2.5: 앞 16자(params.ActionMax)만 대소문자 무시로 비교하고,
// 어느 이름과도 맞지 않으면 Restart 로 본다. 손으로 고친 레지스트리 값이
// 서비스 기동을 막지 않게 하려는 원본 동작이며 그대로 승계한다.
func Parse(text string) Action {
	prefix := text
	if len(prefix) > params.ActionMax {
		prefix = prefix[:params.ActionMax]
	}
	for i, name := range names {
		if strings.EqualFold(prefix, name) {
			return Action(i)
		}
	}
	return Restart
}

// ParseStrict 는 설정 단계(set 명령)에서 쓰는 엄격한 해석이다.
// P0007 3.10 이 정한 대로 알 수 없는 값은 거부해야 하므로 ok 로 구분해 돌려준다.
func ParseStrict(text string) (Action, bool) {
	for i, name := range names {
		if strings.EqualFold(text, name) {
			return Action(i), true
		}
	}
	return Restart, false
}
