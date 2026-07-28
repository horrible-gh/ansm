package msgcat_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"ansm/internal/msgcat"
)

const sample = `LanguageNames =
(
English=0x0409:MSG00409
French=0x40C:MSG0040C
)

MessageId = 501
SymbolicName = FIRST
Severity = Informational
Language = English
first
.
Language = French
premier
.

MessageId =
SymbolicName = SECOND
Severity = Error
Language = English
second line one
second line two
.
Language = French
deuxième
.

MessageId = +2
SymbolicName = THIRD
Language = English
third
.
Language = French
troisième
.
`

func parse(t *testing.T, text string) *msgcat.Catalog {
	t.Helper()
	c, err := msgcat.Parse(strings.NewReader(text))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return c
}

func TestParseReadsLanguagesAndMessages(t *testing.T) {
	c := parse(t, sample)

	if len(c.Languages) != 2 {
		t.Fatalf("languages = %d, want 2", len(c.Languages))
	}
	if c.Languages[0].ID != 0x0409 || c.Languages[1].ID != 0x040c {
		t.Errorf("language ids = %#x %#x", c.Languages[0].ID, c.Languages[1].ID)
	}
	if len(c.Messages) != 3 {
		t.Fatalf("messages = %d, want 3", len(c.Messages))
	}
}

// 빈 MessageId 는 바로 다음 번호, "+n" 은 n 만큼 건너뛴 번호다.
func TestMessageIdCountsOnFromThePreviousMessage(t *testing.T) {
	c := parse(t, sample)

	for i, want := range []uint32{501, 502, 504} {
		if got := c.Messages[i].Code; got != want {
			t.Errorf("message %d code = %d, want %d", i, got, want)
		}
	}
}

// Severity 는 다음 메시지로 이어진다. THIRD 는 Severity 줄이 없으므로
// SECOND 의 Error 를 물려받는다.
func TestSeverityCarriesToLaterMessages(t *testing.T) {
	c := parse(t, sample)

	for i, want := range []msgcat.Severity{
		msgcat.SeverityInformational, msgcat.SeverityError, msgcat.SeverityError,
	} {
		if got := c.Messages[i].Severity; got != want {
			t.Errorf("message %d severity = %s, want %s", i, got, want)
		}
	}
}

func TestIDCarriesSeverityInTheTopTwoBits(t *testing.T) {
	c := parse(t, sample)

	if got, want := c.Messages[0].ID(), uint32(0x40000000|501); got != want {
		t.Errorf("informational id = %#x, want %#x", got, want)
	}
	if got, want := c.Messages[1].ID(), uint32(0xc0000000|502); got != want {
		t.Errorf("error id = %#x, want %#x", got, want)
	}
}

// 여러 줄 문구는 이벤트 로그가 기대하는 CRLF 로 이어 붙는다. 저장소 사본의
// 줄끝이 LF 여도 마찬가지다.
func TestMultipleLinesJoinWithCRLF(t *testing.T) {
	c := parse(t, sample)

	if got, want := c.Messages[1].Texts[0x0409], "second line one\r\nsecond line two"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestParseAcceptsUTF16WithBOM(t *testing.T) {
	var utf16le []byte
	utf16le = append(utf16le, 0xff, 0xfe)
	for _, r := range sample {
		utf16le = append(utf16le, byte(r), byte(r>>8))
	}

	c, err := msgcat.Parse(strings.NewReader(string(utf16le)))
	if err != nil {
		t.Fatalf("parse UTF-16: %v", err)
	}
	if len(c.Messages) != 3 {
		t.Errorf("messages = %d, want 3", len(c.Messages))
	}
}

// 모르는 지시자를 조용히 넘기면 문구가 빠진 리소스가 만들어진다. 그 사실은
// 이벤트 뷰어에서야 드러나므로 여기서 멈춘다.
func TestUnknownDirectiveIsRejected(t *testing.T) {
	_, err := msgcat.Parse(strings.NewReader(strings.Replace(sample,
		"MessageId = 501", "OutputBase = 16\nMessageId = 501", 1)))
	if !errors.Is(err, msgcat.ErrSyntax) {
		t.Fatalf("error = %v, want a syntax error", err)
	}
}

func TestMessageMissingALanguageIsRejected(t *testing.T) {
	_, err := msgcat.Parse(strings.NewReader(strings.Replace(sample,
		"Language = French\npremier\n.\n", "", 1)))
	if !errors.Is(err, msgcat.ErrSyntax) {
		t.Fatalf("error = %v, want a syntax error", err)
	}
}

func TestUnterminatedTextIsRejected(t *testing.T) {
	_, err := msgcat.Parse(strings.NewReader(strings.TrimSuffix(sample, "troisième\n.\n") + "troisième\n"))
	if !errors.Is(err, msgcat.ErrSyntax) {
		t.Fatalf("error = %v, want a syntax error", err)
	}
}

// 저장소에 넣은 실제 목록이다. 이 파일이 리소스의 원본이므로 갈아엎어지면
// 이벤트 문구가 통째로 바뀐다.
func TestVendoredCatalogue(t *testing.T) {
	c, err := msgcat.ParseFile(filepath.Join("..", "..", "resources", "messages.mc"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if len(c.Languages) != 3 {
		t.Fatalf("languages = %d, want 3 (English, French, Italian)", len(c.Languages))
	}
	if len(c.Messages) != 205 {
		t.Errorf("messages = %d, want 205", len(c.Messages))
	}

	// 원본 나씀 실행 파일에서 읽어 확인한 문구다.
	started, ok := c.Lookup(1008)
	if !ok {
		t.Fatal("message 1008 is missing")
	}
	if got, want := started.Texts[0x0409], "Started %1 %2 for service %3 in %4."; got != want {
		t.Errorf("1008 English = %q, want %q", got, want)
	}
	if got, want := started.ID(), uint32(1073742832); got != want {
		t.Errorf("1008 id = %d, want %d as recorded by NSSM", got, want)
	}
	if got, want := started.Symbol, "NSSM_EVENT_STARTED_SERVICE"; got != want {
		t.Errorf("1008 symbol = %q, want %q", got, want)
	}
}
