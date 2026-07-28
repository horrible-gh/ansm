package exitaction

import (
	"strings"
	"testing"

	"ansm/internal/params"
)

func TestParse(t *testing.T) {
	tests := map[string]Action{
		"Restart": Restart,
		"restart": Restart,
		"IGNORE":  Ignore,
		"Exit":    Exit,
		"Suicide": Suicide,
		// 알 수 없는 문자열은 Restart 로 본다. 손으로 고친 레지스트리 값이
		// 서비스 기동을 막지 않게 하려는 원본 동작이다.
		"Reboot": Restart,
		"":       Restart,
	}
	for in, want := range tests {
		if got := Parse(in); got != want {
			t.Errorf("Parse(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseComparesOnlyFirst16Chars(t *testing.T) {
	// 앞 16자만 비교한다. 17자 이상은 잘린 앞부분으로 판정된다.
	long := "Ignore" + strings.Repeat("x", params.ActionMax)
	if got := Parse(long); got != Restart {
		t.Errorf("Parse(long) = %v, want Restart", got)
	}
	// 16자 이내면 전체가 비교 대상이므로, 뒤에 군더더기가 붙으면 이름과 어긋난다.
	padded := "Suicide" + strings.Repeat("y", params.ActionMax-len("Suicide"))
	if got := Parse(padded); got != Restart {
		t.Errorf("Parse(%q) = %v, want Restart", padded, got)
	}
}

func TestParseStrictRejectsUnknown(t *testing.T) {
	// set 명령은 알 수 없는 값을 거부해야 한다(P0007 3.10, 종료 코드 6).
	if _, ok := ParseStrict("Reboot"); ok {
		t.Error("ParseStrict(Reboot) = ok, want rejected")
	}
	if a, ok := ParseStrict("suicide"); !ok || a != Suicide {
		t.Errorf("ParseStrict(suicide) = %v, %v", a, ok)
	}
}

func TestNamesOrderIsContract(t *testing.T) {
	// P0007 3.10 의 안내 순서이자 Action 값의 순서다.
	want := []string{"Restart", "Ignore", "Exit", "Suicide"}
	got := Names()
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Names() = %v, want %v", got, want)
		}
		if Action(i).String() != want[i] {
			t.Errorf("Action(%d).String() = %q, want %q", i, Action(i).String(), want[i])
		}
	}
}
