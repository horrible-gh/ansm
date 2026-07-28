package messages_test

import (
	"path/filepath"
	"testing"

	"ansm/internal/messages"
	"ansm/internal/msgcat"
)

// catalogPath 는 리소스를 만들 때 쓰는 것과 같은 파일이다. 이 시험이 지키는
// 것은 "Go 상수 → 이벤트 번호" 와 "메시지 목록 → 실행 파일 안의 문구" 가 같은
// 번호를 가리킨다는 사실이다. 어긋나면 이벤트 뷰어가 문구를 못 찾는다.
func catalog(t *testing.T) *msgcat.Catalog {
	t.Helper()
	c, err := msgcat.ParseFile(filepath.Join("..", "..", "resources", "messages.mc"))
	if err != nil {
		t.Fatalf("parse catalogue: %v", err)
	}
	return c
}

func TestEveryDeclaredEventExistsInTheCatalogue(t *testing.T) {
	c := catalog(t)

	ids := messages.EventIDs()
	if len(ids) != 81 {
		t.Fatalf("declared %d event ids, want 81 (P0007 7.2: 1001-1081)", len(ids))
	}

	for _, id := range ids {
		m, ok := c.Lookup(uint32(id))
		if !ok {
			t.Errorf("event %d is declared in Go but missing from the catalogue", id)
			continue
		}
		if got, want := messages.EventValue(id), m.ID(); got != want {
			t.Errorf("event %d (%s): Go writes %#x, catalogue compiles to %#x", id, m.Symbol, got, want)
		}
	}
}

func TestCatalogueEventsAreAllDeclared(t *testing.T) {
	c := catalog(t)

	declared := make(map[uint32]bool)
	for _, id := range messages.EventIDs() {
		declared[uint32(id)] = true
	}
	for _, m := range c.Messages {
		if m.Code < uint32(messages.EventFirst) || m.Code > uint32(messages.EventLast) {
			continue
		}
		if !declared[m.Code] {
			t.Errorf("catalogue has event %d (%s) which no Go constant names", m.Code, m.Symbol)
		}
	}
}

// 원본의 log_event 호출부가 목록의 심각도와 다른 수준을 넘기는 9개를 뺀
// 나머지는 목록을 그대로 따라야 한다. 이 목록이 늘거나 줄면 어느 쪽이든
// 원본과 어긋난 것이므로 여기서 멈춘다.
var typeOverrides = map[messages.ID]messages.Severity{
	messages.EventConfigFailureActionsFailed:   messages.SeverityError,
	messages.EventBogusPriority:                messages.SeverityWarning,
	messages.EventConfigDescriptionFailed:      messages.SeverityError,
	messages.EventConfigDelayedAutoStartFailed: messages.SeverityError,
	messages.EventGetProcessAffinityMaskFailed: messages.SeverityError,
	messages.EventSetProcessAffinityMaskFailed: messages.SeverityWarning,
	messages.EventPrestartHookAbort:            messages.SeverityError,
	messages.EventHookCreateProcessFailed:      messages.SeverityError,
	messages.EventGetHookFailed:                messages.SeverityError,
}

func TestEventTypeMatchesCatalogueSeverityExceptWhereNSSMDiffers(t *testing.T) {
	c := catalog(t)

	fromCatalogue := map[msgcat.Severity]messages.Severity{
		msgcat.SeverityError:         messages.SeverityError,
		msgcat.SeverityWarning:       messages.SeverityWarning,
		msgcat.SeverityInformational: messages.SeverityInformation,
	}
	for _, id := range messages.EventIDs() {
		m, ok := c.Lookup(uint32(id))
		if !ok {
			continue
		}
		want, overridden := typeOverrides[id]
		if !overridden {
			want = fromCatalogue[m.Severity]
		}
		if got := messages.EventType(id); got != want {
			t.Errorf("event %d (%s): type %d, want %d (catalogue severity %s, overridden %v)",
				id, m.Symbol, got, want, m.Severity, overridden)
		}
		if overridden && fromCatalogue[m.Severity] == want {
			t.Errorf("event %d (%s) is listed as an override but agrees with the catalogue", id, m.Symbol)
		}
	}
}

// 이 값들은 이 기계에 남아 있던 원본 나씀의 이벤트 기록에서 직접 읽은 것이다.
// 이식본이 같은 번호를 써야 과거 기록과 새 기록이 같은 문구를 찾는다.
func TestEventValueMatchesRecordedNSSMValues(t *testing.T) {
	for _, tc := range []struct {
		id    messages.ID
		value uint32
	}{
		{messages.EventStartedService, 1073742832},        // 0x40000000|1008
		{messages.EventTerminateProcess, 1073742835},      // 0x40000000|1011
		{messages.EventKilling, 1073742847},               // 0x40000000|1023
		{messages.EventKillProcessTree, 1073742851},       // 0x40000000|1027
		{messages.EventServiceControlHandled, 1073742864}, // 0x40000000|1040
	} {
		if got := messages.EventValue(tc.id); got != tc.value {
			t.Errorf("event %d: value %d, recorded by NSSM as %d", tc.id, got, tc.value)
		}
	}
}
