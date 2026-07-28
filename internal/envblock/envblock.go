// Package envblock 은 환경 변수 묶음의 구성·병합 규칙을 담는다.
//
// L0008 2.8. 저장 형태는 REG_MULTI_SZ(NUL 로 구분, NUL NUL 로 끝남)이고
// 사람이 보는 형태는 CRLF 로 이어 붙인 것이다. 둘 사이의 변환은 단순 치환이다.
package envblock

import "strings"

// Entry 는 환경 변수 한 줄이다.
type Entry struct {
	Name  string
	Value string
}

// ParseLines 는 목록 형태(줄 단위)를 항목으로 나눈다.
//
// L0008 2.8 규칙 4·5:
//   - "=X:=경로" 형태의 드라이브 변수는 운영체제 내부 변수이므로 건너뛴다.
//   - '=' 가 없는 줄은 무시한다.
func ParseLines(lines []string) []Entry {
	var out []Entry
	for _, line := range lines {
		if line == "" {
			continue
		}
		// 첫 글자가 '=' 이면 드라이브 변수(=C:=C:\work)다.
		if line[0] == '=' {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out = append(out, Entry{Name: name, Value: value})
	}
	return out
}

// Parse 는 사람이 보는 형태(CRLF 또는 LF 로 이어 붙인 문자열)를 항목으로 나눈다.
func Parse(text string) []Entry {
	if text == "" {
		return nil
	}
	return ParseLines(splitLines(text))
}

func splitLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.Split(text, "\n")
}

// Format 은 항목을 사람이 보는 형태(CRLF 로 이어 붙임)로 돌려준다.
// P0007 3.4 의 "REG_MULTI_SZ 계열이므로 CRLF 로 이어 붙임"이 이 표기다.
func Format(entries []Entry) string {
	lines := make([]string, 0, len(entries))
	for _, e := range entries {
		lines = append(lines, e.Name+"="+e.Value)
	}
	return strings.Join(lines, "\r\n")
}

// Upsert 는 같은 이름이 있으면 덮어쓰고 없으면 뒤에 붙인다.
// L0008 2.8 규칙 2: 변수 이름 비교는 대소문자를 구분하지 않는다.
func Upsert(entries []Entry, add Entry) []Entry {
	for i := range entries {
		if strings.EqualFold(entries[i].Name, add.Name) {
			entries[i].Value = add.Value
			return entries
		}
	}
	return append(entries, add)
}

// Remove 는 항목을 지운다. L0008 2.8 규칙 7.
//
//	matchValue == false → 이름만 맞으면 지운다 ("KEY=" 형태)
//	matchValue == true  → 이름과 값이 모두 맞을 때만 지운다 ("KEY=VALUE" 형태)
//
// 값 비교는 대소문자를 구분한다. 이름 비교만 구분하지 않는다.
func Remove(entries []Entry, name, value string, matchValue bool) []Entry {
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if strings.EqualFold(e.Name, name) && (!matchValue || e.Value == value) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// Merge 는 base 위에 extra 를 얹는다(AppEnvironmentExtra 의 의미).
// base 는 고치지 않고 새 목록을 돌려준다.
func Merge(base, extra []Entry) []Entry {
	out := make([]Entry, len(base))
	copy(out, base)
	for _, e := range extra {
		out = Upsert(out, e)
	}
	return out
}

// ExpandPercent 는 Windows의 %NAME% 환경 변수 표기를 현재 항목으로 전개한다.
// 닫는 '%'가 없거나 이름을 찾지 못한 표기는 원문 그대로 둔다.
func ExpandPercent(entries []Entry, value string) string {
	var b strings.Builder
	for len(value) > 0 {
		start := strings.IndexByte(value, '%')
		if start < 0 {
			b.WriteString(value)
			break
		}
		b.WriteString(value[:start])
		value = value[start+1:]
		end := strings.IndexByte(value, '%')
		if end < 0 {
			b.WriteByte('%')
			b.WriteString(value)
			break
		}
		name := value[:end]
		if expanded, ok := Lookup(entries, name); ok {
			b.WriteString(expanded)
		} else {
			b.WriteByte('%')
			b.WriteString(name)
			b.WriteByte('%')
		}
		value = value[end+1:]
	}
	return b.String()
}

// Apply 는 override를 앞에서부터 적용하며, 각 값은 그 시점까지 만들어진
// 환경을 기준으로 %NAME% 표기를 전개한다(L0008 2.8).
func Apply(base, override []Entry) []Entry {
	out := append([]Entry(nil), base...)
	for _, entry := range override {
		entry.Value = ExpandPercent(out, entry.Value)
		out = Upsert(out, entry)
	}
	return out
}

// Strings 는 CreateProcess에 넘길 NAME=VALUE 목록을 만든다.
func Strings(entries []Entry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.Name+"="+entry.Value)
	}
	return out
}

// Lookup 은 이름으로 값을 찾는다. 대소문자를 구분하지 않는다.
func Lookup(entries []Entry, name string) (string, bool) {
	for _, e := range entries {
		if strings.EqualFold(e.Name, name) {
			return e.Value, true
		}
	}
	return "", false
}
