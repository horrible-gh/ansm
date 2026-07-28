// Package msgcat 은 메시지 목록 파일(`resources/messages.mc`)을 읽는다.
//
// 원본 나씀은 이 파일을 Windows SDK 의 `mc.exe` 로 컴파일해 실행 파일에
// MESSAGETABLE 리소스로 심는다. T1 스파이크(docs/T1-spike.md 1장)가 정한 대로
// ANSM 은 외부 툴체인을 쓰지 않고 같은 파일을 직접 읽어 리소스를 만든다.
// 여기는 그 읽는 쪽이고, 만드는 쪽은 internal/rsrc 다.
//
// mc 문법 전체가 아니라 `messages.mc` 가 실제로 쓰는 갈래만 받는다. 모르는
// 지시자를 만나면 조용히 넘기지 않고 오류를 낸다. 조용히 넘기면 문구가 빠진
// 리소스가 만들어지고, 그 사실은 이벤트 뷰어에서야 드러나기 때문이다.
package msgcat

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Severity 는 mc 의 심각도다. 메시지 번호 상위 2비트에 실린다.
//
// 원본 나씀이 실제로 남긴 이벤트 기록으로 확인한 값이다. 예를 들어
// NSSM_EVENT_STARTED_SERVICE(1008) 는 이벤트 로그에 0x40000000|1008 =
// 1073742832 로 적혀 있다. 곧 mc 기본값은 Facility 0, Customer 비트 0 이며
// 심각도만 상위 2비트에 얹힌다.
type Severity uint32

const (
	SeveritySuccess       Severity = 0
	SeverityInformational Severity = 1
	SeverityWarning       Severity = 2
	SeverityError         Severity = 3
)

var severityNames = map[string]Severity{
	"Success":       SeveritySuccess,
	"Informational": SeverityInformational,
	"Warning":       SeverityWarning,
	"Error":         SeverityError,
}

// String 은 mc 표기 이름이다.
func (s Severity) String() string {
	for name, value := range severityNames {
		if value == s {
			return name
		}
	}
	return "Severity(" + strconv.FormatUint(uint64(s), 10) + ")"
}

// Language 는 LanguageNames 머리글의 한 줄이다.
type Language struct {
	Name string // "English"
	ID   uint16 // 0x0409
	File string // "MSG00409" — mc 가 쓰던 중간 파일 이름. 우리는 쓰지 않는다.
}

// Message 는 번호 하나에 붙은 여러 언어의 문구다.
type Message struct {
	Code     uint32 // 501, 1001 처럼 설계 문서가 못 박은 번호
	Symbol   string // NSSM_EVENT_STARTED_SERVICE
	Severity Severity
	Texts    map[uint16]string // 언어 번호 -> 문구. 줄 구분은 CRLF.
}

// ID 는 ReportEvent·FormatMessage 에 넘기는 32비트 값이다.
//
// 이벤트 뷰어가 보여주는 "이벤트 ID" 는 하위 16비트라 Code 와 같아 보이지만,
// 기록에 실리는 값과 문구를 찾는 열쇠는 이쪽이다.
func (m Message) ID() uint32 { return uint32(m.Severity)<<30 | m.Code }

// Catalog 는 파일 하나를 통째로 읽은 결과다. 순서는 파일 순서를 지킨다.
type Catalog struct {
	Languages []Language
	Messages  []Message
}

// Lookup 은 번호로 메시지를 찾는다.
func (c *Catalog) Lookup(code uint32) (Message, bool) {
	for _, m := range c.Messages {
		if m.Code == code {
			return m, true
		}
	}
	return Message{}, false
}

// ErrSyntax 는 목록 파일이 우리가 받는 갈래를 벗어났다는 뜻이다.
var ErrSyntax = errors.New("malformed message catalogue")

type syntaxError struct {
	line int
	msg  string
}

func (e *syntaxError) Error() string {
	return fmt.Sprintf("%s: line %d: %s", ErrSyntax.Error(), e.line, e.msg)
}
func (e *syntaxError) Unwrap() error { return ErrSyntax }

// Parse 는 목록 파일을 읽는다.
//
// UTF-16LE(원본 그대로), UTF-8, 그리고 둘 다의 BOM 을 받는다. 저장소에 넣은
// 사본은 diff 가 되도록 UTF-8·LF 로 두었지만, 원본 파일을 그대로 넘겨도 같은
// 결과가 나와야 이식본이 원본과 어긋나지 않았음을 그때그때 확인할 수 있다.
func Parse(r io.Reader) (*Catalog, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	text, err := decode(raw)
	if err != nil {
		return nil, err
	}

	p := &parser{lines: splitLines(text)}
	return p.parse()
}

func decode(raw []byte) (string, error) {
	switch {
	case bytes.HasPrefix(raw, []byte{0xff, 0xfe}):
		body := raw[2:]
		if len(body)%2 != 0 {
			return "", fmt.Errorf("%w: truncated UTF-16LE content", ErrSyntax)
		}
		units := make([]uint16, len(body)/2)
		for i := range units {
			units[i] = uint16(body[2*i]) | uint16(body[2*i+1])<<8
		}
		return string(utf16.Decode(units)), nil
	case bytes.HasPrefix(raw, []byte{0xfe, 0xff}):
		return "", fmt.Errorf("%w: UTF-16BE is not supported", ErrSyntax)
	case bytes.HasPrefix(raw, []byte{0xef, 0xbb, 0xbf}):
		return string(raw[3:]), nil
	default:
		return string(raw), nil
	}
}

func splitLines(text string) []string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	// 마지막 줄바꿈 뒤의 빈 줄은 내용이 아니다.
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines
}

type parser struct {
	lines []string
	at    int

	catalog  Catalog
	byName   map[string]uint16 // 언어 이름 -> 번호
	lastCode uint32
	severity Severity
	current  *Message
}

func (p *parser) parse() (*Catalog, error) {
	p.byName = make(map[string]uint16)
	for p.at < len(p.lines) {
		line := p.lines[p.at]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, ";") {
			p.at++
			continue
		}

		key, value, ok := cutDirective(trimmed)
		if !ok {
			return nil, p.errorf("expected a directive, got %q", clip(trimmed))
		}
		p.at++

		var err error
		switch key {
		case "LanguageNames":
			err = p.languageNames(value)
		case "MessageId":
			err = p.messageID(value)
		case "SymbolicName":
			err = p.symbolicName(value)
		case "Severity":
			err = p.severityDirective(value)
		case "Language":
			err = p.language(value)
		default:
			err = p.errorf("unsupported directive %q", key)
		}
		if err != nil {
			return nil, err
		}
	}

	if len(p.catalog.Languages) == 0 {
		return nil, fmt.Errorf("%w: no LanguageNames block", ErrSyntax)
	}
	if len(p.catalog.Messages) == 0 {
		return nil, fmt.Errorf("%w: no messages", ErrSyntax)
	}
	for _, m := range p.catalog.Messages {
		if len(m.Texts) != len(p.catalog.Languages) {
			return nil, fmt.Errorf("%w: message %d (%s) has %d of %d languages",
				ErrSyntax, m.Code, m.Symbol, len(m.Texts), len(p.catalog.Languages))
		}
	}
	return &p.catalog, nil
}

// cutDirective 는 "Key = Value" 를 가른다. mc 는 등호 앞뒤 공백을 가리지 않는다.
func cutDirective(line string) (key, value string, ok bool) {
	key, value, ok = strings.Cut(line, "=")
	if !ok {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" {
		return "", "", false
	}
	for _, r := range key {
		if r != '_' && !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') {
			return "", "", false
		}
	}
	return key, value, true
}

func (p *parser) languageNames(value string) error {
	if p.catalog.Languages != nil {
		return p.errorf("LanguageNames appears twice")
	}
	if value != "" && value != "(" {
		return p.errorf("LanguageNames must open a parenthesised list")
	}
	if value == "" {
		if p.at >= len(p.lines) || strings.TrimSpace(p.lines[p.at]) != "(" {
			return p.errorf("LanguageNames must be followed by \"(\"")
		}
		p.at++
	}

	p.catalog.Languages = []Language{}
	for {
		if p.at >= len(p.lines) {
			return p.errorf("unterminated LanguageNames list")
		}
		line := strings.TrimSpace(p.lines[p.at])
		p.at++
		if line == ")" {
			break
		}
		if line == "" {
			continue
		}
		line = strings.TrimSuffix(line, ")")

		name, rest, ok := strings.Cut(line, "=")
		if !ok {
			return p.errorf("expected Name=0xID:File, got %q", clip(line))
		}
		id, file, ok := strings.Cut(rest, ":")
		if !ok {
			return p.errorf("expected Name=0xID:File, got %q", clip(line))
		}
		n, err := strconv.ParseUint(strings.TrimSpace(id), 0, 16)
		if err != nil {
			return p.errorf("bad language id %q", clip(id))
		}
		name = strings.TrimSpace(name)
		p.byName[name] = uint16(n)
		p.catalog.Languages = append(p.catalog.Languages, Language{
			Name: name,
			ID:   uint16(n),
			File: strings.TrimSpace(file),
		})
	}
	if len(p.catalog.Languages) == 0 {
		return p.errorf("LanguageNames list is empty")
	}
	return nil
}

func (p *parser) messageID(value string) error {
	// mc 는 세 갈래를 받는다: 절대값, "+n" 만큼 건너뛰기, 빈 값(바로 다음 번호).
	var code uint32
	switch {
	case value == "":
		code = p.lastCode + 1
	case strings.HasPrefix(value, "+"):
		step, err := strconv.ParseUint(strings.TrimSpace(value[1:]), 0, 32)
		if err != nil {
			return p.errorf("bad MessageId increment %q", clip(value))
		}
		code = p.lastCode + uint32(step)
	default:
		n, err := strconv.ParseUint(value, 0, 32)
		if err != nil {
			return p.errorf("bad MessageId %q", clip(value))
		}
		code = uint32(n)
	}
	if code > 0xffff {
		return p.errorf("MessageId %d does not fit in 16 bits", code)
	}

	p.lastCode = code
	p.catalog.Messages = append(p.catalog.Messages, Message{
		Code:     code,
		Severity: p.severity,
		Texts:    make(map[uint16]string),
	})
	p.current = &p.catalog.Messages[len(p.catalog.Messages)-1]
	return nil
}

func (p *parser) symbolicName(value string) error {
	if p.current == nil {
		return p.errorf("SymbolicName before any MessageId")
	}
	if value == "" {
		return p.errorf("empty SymbolicName")
	}
	p.current.Symbol = value
	return nil
}

func (p *parser) severityDirective(value string) error {
	s, ok := severityNames[value]
	if !ok {
		return p.errorf("unknown Severity %q", clip(value))
	}
	// mc 에서 Severity 는 다음 메시지들에도 이어진다.
	p.severity = s
	if p.current != nil {
		p.current.Severity = s
	}
	return nil
}

func (p *parser) language(value string) error {
	if p.current == nil {
		return p.errorf("Language before any MessageId")
	}
	id, ok := p.byName[value]
	if !ok {
		return p.errorf("unknown language %q", clip(value))
	}
	if _, dup := p.current.Texts[id]; dup {
		return p.errorf("language %q appears twice for message %d", clip(value), p.current.Code)
	}

	start := p.at
	var body []string
	for {
		if p.at >= len(p.lines) {
			return p.errorf("unterminated text for message %d (started at line %d)", p.current.Code, start)
		}
		line := p.lines[p.at]
		p.at++
		if line == "." {
			break
		}
		body = append(body, line)
	}
	// 이벤트 로그와 콘솔 모두 CRLF 를 기대한다. 저장소 사본의 줄끝이 무엇이든
	// 리소스에 실리는 문구는 원본과 같아야 한다.
	p.current.Texts[id] = strings.Join(body, "\r\n")
	return nil
}

func (p *parser) errorf(format string, args ...any) error {
	return &syntaxError{line: p.at, msg: fmt.Sprintf(format, args...)}
}

func clip(s string) string {
	const max = 40
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// ParseFile 은 경로에서 목록을 읽는다.
func ParseFile(path string) (*Catalog, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	c, err := Parse(bufio.NewReader(f))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}
