package messages_test

import (
	"path/filepath"
	"testing"

	"ansm/internal/messages"
	"ansm/internal/msgcat"
)

// catalog follows the documented behavioral contract. See Go.
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

// typeOverrides follows the documented behavioral contract.
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

// TestEventValueMatchesRecordedNSSMValues follows the documented behavioral contract.
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
