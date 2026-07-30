// Package msgcat parses resources/messages.mc without the Windows SDK. It accepts only the directives used by NSSM and rejects unknown syntax so missing messages cannot remain hidden until Event Viewer lookup.
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

// Severity occupies the top two bits of the 32-bit message value; Facility and Customer remain zero, matching records written by NSSM.
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

// String follows the documented behavioral contract. See String.
func (s Severity) String() string {
	for name, value := range severityNames {
		if value == s {
			return name
		}
	}
	return "Severity(" + strconv.FormatUint(uint64(s), 10) + ")"
}

// Language follows the documented behavioral contract. See Language, LanguageNames.
type Language struct {
	Name string // "English"
	ID   uint16 // 0x0409
	File string // File follows the documented contract. See MSG00409.
}

// Message follows the documented behavioral contract. See Message.
type Message struct {
	Code     uint32 // Code follows the documented contract.
	Symbol   string // NSSM_EVENT_STARTED_SERVICE
	Severity Severity
	Texts    map[uint16]string // Texts follows the documented contract. See CRLF.
}

// ID follows the documented behavioral contract. See ID, ReportEvent, FormatMessage, Code.
func (m Message) ID() uint32 { return uint32(m.Severity)<<30 | m.Code }

// Catalog follows the documented behavioral contract. See Catalog.
type Catalog struct {
	Languages []Language
	Messages  []Message
}

// Lookup follows the documented behavioral contract. See Lookup.
func (c *Catalog) Lookup(code uint32) (Message, bool) {
	for _, m := range c.Messages {
		if m.Code == code {
			return m, true
		}
	}
	return Message{}, false
}

// ErrSyntax follows the documented behavioral contract. See ErrSyntax.
var ErrSyntax = errors.New("malformed message catalogue")

type syntaxError struct {
	line int
	msg  string
}

func (e *syntaxError) Error() string {
	return fmt.Sprintf("%s: line %d: %s", ErrSyntax.Error(), e.line, e.msg)
}
func (e *syntaxError) Unwrap() error { return ErrSyntax }

// Parse accepts UTF-16LE or UTF-8, with or without BOM. The committed UTF-8/LF copy and the original UTF-16LE catalog must produce identical resources.
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
	// if follows the documented behavioral contract.
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines
}

type parser struct {
	lines []string
	at    int

	catalog  Catalog
	byName   map[string]uint16 // byName follows the documented contract.
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

// cutDirective splits Key=Value while accepting whitespace around the equals sign, as mc syntax does.
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
	// code follows the documented behavioral contract.
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
	// This section follows the documented behavioral contract. See Severity.
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
	// This section follows the documented behavioral contract. See CRLF.
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

// ParseFile opens and parses a message-catalog path.
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
