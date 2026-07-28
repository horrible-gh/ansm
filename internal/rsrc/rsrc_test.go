package rsrc_test

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"path/filepath"
	"testing"
	"unicode/utf16"

	"ansm/internal/msgcat"
	"ansm/internal/rsrc"
)

// --- 메시지 표 ---

// FormatMessage 는 블록 목록에서 번호가 든 구간을 찾은 뒤, 그 구간의 첫
// 항목부터 Length 만큼씩 건너뛴다. 그래서 이어지는 번호는 한 블록에 모여야
// 하고, 끊긴 자리에서는 블록이 갈라져야 한다.
func TestMessageTableGroupsConsecutiveIdsIntoOneBlock(t *testing.T) {
	data, err := rsrc.MessageTable(map[uint32]string{1: "a", 2: "b", 5: "c"})
	if err != nil {
		t.Fatalf("MessageTable: %v", err)
	}

	blocks := readBlocks(t, data)
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(blocks))
	}
	if blocks[0].lo != 1 || blocks[0].hi != 2 {
		t.Errorf("first block = %d..%d, want 1..2", blocks[0].lo, blocks[0].hi)
	}
	if blocks[1].lo != 5 || blocks[1].hi != 5 {
		t.Errorf("second block = %d..%d, want 5..5", blocks[1].lo, blocks[1].hi)
	}
}

func TestMessageTableRoundTrips(t *testing.T) {
	want := map[uint32]string{
		0x40000001: "one\r\n",
		0x40000002: "two lines\r\nsecond\r\n",
		0xc0000009: "unicode: 나씀\r\n",
	}
	data, err := rsrc.MessageTable(want)
	if err != nil {
		t.Fatalf("MessageTable: %v", err)
	}

	got := readMessages(t, data)
	for id, text := range want {
		if got[id] != text {
			t.Errorf("message %#x = %q, want %q", id, got[id], text)
		}
	}
	if len(got) != len(want) {
		t.Errorf("messages = %d, want %d", len(got), len(want))
	}
}

// 항목 길이는 머리 4바이트를 포함하며 4의 배수다. 원본 나씀 실행 파일의
// 항목도 그렇게 맞춰져 있다. 한 바이트만 어긋나도 그 뒤 문구가 전부 밀린다.
func TestMessageEntriesArePaddedToFourBytes(t *testing.T) {
	for _, text := range []string{"", "a", "ab", "abc", "abcd", "hello\r\n"} {
		data, err := rsrc.MessageTable(map[uint32]string{1: text})
		if err != nil {
			t.Fatalf("MessageTable(%q): %v", text, err)
		}
		entry := data[16:] // 블록 하나짜리 표의 첫 항목
		length := binary.LittleEndian.Uint16(entry)
		if length%4 != 0 {
			t.Errorf("entry length for %q is %d, which is not a multiple of 4", text, length)
		}
		if want := uint16(4 + 2*(len([]rune(text))+1)); length < want {
			t.Errorf("entry length for %q is %d, too short for %d", text, length, want)
		}
		if flags := binary.LittleEndian.Uint16(entry[2:]); flags != 1 {
			t.Errorf("entry flags = %d, want 1 (unicode)", flags)
		}
	}
}

func TestEmptyMessageTableIsRejected(t *testing.T) {
	if _, err := rsrc.MessageTable(nil); err == nil {
		t.Fatal("empty table was accepted")
	}
}

// --- 목록에서 리소스로 ---

func TestAddMessageTablesMakesOneResourcePerLanguage(t *testing.T) {
	catalog, err := msgcat.ParseFile(filepath.Join("..", "..", "resources", "messages.mc"))
	if err != nil {
		t.Fatalf("parse catalogue: %v", err)
	}

	set := &rsrc.Set{}
	if err := rsrc.AddMessageTables(set, catalog); err != nil {
		t.Fatalf("AddMessageTables: %v", err)
	}

	entries := set.Entries()
	if len(entries) != len(catalog.Languages) {
		t.Fatalf("entries = %d, want %d", len(entries), len(catalog.Languages))
	}
	for _, e := range entries {
		if e.Type != rsrc.TypeMessageTable || e.Name != 1 {
			t.Errorf("entry type %d name %d, want %d/1", e.Type, e.Name, rsrc.TypeMessageTable)
		}
	}

	english := readMessages(t, entries[0].Data)
	// 원본 실행 파일에서 읽어 확인한 문구다. 마지막 줄바꿈과 NUL 까지 같다.
	if got, want := english[1073742832], "Started %1 %2 for service %3 in %4.\r\n"; got != want {
		t.Errorf("1008 = %q, want %q", got, want)
	}
}

// --- 버전 자료 ---

func TestVersionInfoIsSelfDescribing(t *testing.T) {
	info := rsrc.VersionInfo{
		FileVersion:    [4]uint16{2, 24, 101, 0},
		ProductVersion: [4]uint16{2, 24, 101, 0},
		PreRelease:     true,
		Strings:        map[string]string{"FileVersion": "2.24-101-g897c7ad", "ProductName": "NSSM 64-bit"},
		Translations:   []rsrc.Translation{{Language: 0x0409, CodePage: 0x04b0}},
	}
	data, err := info.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if got := int(binary.LittleEndian.Uint16(data)); got != len(data) {
		t.Errorf("root length field = %d, actual %d", got, len(data))
	}
	if key := utf16String(data[6:]); key != "VS_VERSION_INFO" {
		t.Errorf("root key = %q", key)
	}
	// VS_FIXEDFILEINFO 는 4바이트 자리에 놓이고 서명으로 시작한다.
	at := 6 + 2*(len("VS_VERSION_INFO")+1)
	at += (4 - at%4) % 4
	if got := binary.LittleEndian.Uint32(data[at:]); got != 0xfeef04bd {
		t.Fatalf("fixed file info signature = %#x", got)
	}
	if got, want := binary.LittleEndian.Uint32(data[at+8:]), uint32(2<<16|24); got != want {
		t.Errorf("file version high = %#x, want %#x", got, want)
	}
	if got, want := binary.LittleEndian.Uint32(data[at+12:]), uint32(101<<16); got != want {
		t.Errorf("file version low = %#x, want %#x", got, want)
	}
	if got := binary.LittleEndian.Uint32(data[at+28:]); got != 2 {
		t.Errorf("file flags = %#x, want VS_FF_PRERELEASE", got)
	}
	if !bytes.Contains(data, utf16Bytes("2.24-101-g897c7ad")) {
		t.Error("version string is missing from the string table")
	}
	if !bytes.Contains(data, utf16Bytes("Translation")) {
		t.Error("VarFileInfo is missing")
	}
}

// 지도 순회 순서가 산출물을 바꾸면 같은 입력으로 만든 배포본이 서로 달라진다.
func TestVersionInfoIsReproducible(t *testing.T) {
	info := rsrc.VersionInfo{
		FileVersion:  [4]uint16{1, 2, 3, 4},
		Strings:      map[string]string{"a": "1", "b": "2", "c": "3", "d": "4", "e": "5"},
		Translations: []rsrc.Translation{{Language: 0x0409, CodePage: 0x04b0}},
	}
	first, err := info.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for i := 0; i < 20; i++ {
		again, err := info.Build()
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		if !bytes.Equal(first, again) {
			t.Fatal("two builds of the same version info differ")
		}
	}
}

// --- 아이콘 ---

func TestAddIconSplitsImagesAndGroup(t *testing.T) {
	set := &rsrc.Set{}
	if err := rsrc.AddIcon(set, fakeIcon(2), 101, 1, 0x0409); err != nil {
		t.Fatalf("AddIcon: %v", err)
	}

	entries := set.Entries()
	if len(entries) != 3 {
		t.Fatalf("entries = %d, want 2 icons and 1 group", len(entries))
	}
	var group rsrc.Entry
	icons := 0
	for _, e := range entries {
		switch e.Type {
		case rsrc.TypeIcon:
			icons++
		case rsrc.TypeGroupIcon:
			group = e
		}
	}
	if icons != 2 {
		t.Errorf("icon resources = %d, want 2", icons)
	}
	if len(group.Data) != 6+14*2 {
		t.Fatalf("group is %d bytes, want %d", len(group.Data), 6+14*2)
	}
	for i := 0; i < 2; i++ {
		if got, want := binary.LittleEndian.Uint16(group.Data[6+14*i+12:]), uint16(1+i); got != want {
			t.Errorf("group entry %d points at icon %d, want %d", i, got, want)
		}
	}
}

func TestTruncatedIconIsRejected(t *testing.T) {
	icon := fakeIcon(2)
	if err := rsrc.AddIcon(&rsrc.Set{}, icon[:len(icon)-4], 101, 1, 0x0409); err == nil {
		t.Fatal("truncated icon was accepted")
	}
	if err := rsrc.AddIcon(&rsrc.Set{}, []byte{0, 0, 9, 0, 1, 0}, 101, 1, 0x0409); err == nil {
		t.Fatal("non-icon file was accepted")
	}
}

// --- COFF 오브젝트 ---

func TestWriteObjectProducesALinkableObject(t *testing.T) {
	for _, arch := range rsrc.Arches {
		set := &rsrc.Set{}
		if err := rsrc.AddManifest(set, rsrc.DefaultManifest, 0x0409); err != nil {
			t.Fatalf("AddManifest: %v", err)
		}
		table, err := rsrc.MessageTable(map[uint32]string{0x40000001: "hello\r\n"})
		if err != nil {
			t.Fatalf("MessageTable: %v", err)
		}
		if err := set.Add(rsrc.Entry{Type: rsrc.TypeMessageTable, Name: 1, Language: 0x0409, Data: table}); err != nil {
			t.Fatalf("Add: %v", err)
		}

		var buf bytes.Buffer
		if err := rsrc.WriteObject(&buf, arch, set); err != nil {
			t.Fatalf("%s: WriteObject: %v", arch.GOARCH, err)
		}

		file, err := pe.NewFile(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatalf("%s: the object does not parse as COFF: %v", arch.GOARCH, err)
		}
		defer file.Close()

		if len(file.Sections) != 1 || file.Sections[0].Name != ".rsrc" {
			t.Fatalf("%s: sections = %v", arch.GOARCH, file.Sections)
		}
		section := file.Sections[0]
		if len(section.Relocs) != 2 {
			t.Errorf("%s: relocations = %d, want one per resource", arch.GOARCH, len(section.Relocs))
		}
		if len(file.COFFSymbols) != 1 {
			t.Fatalf("%s: symbols = %d, want 1", arch.GOARCH, len(file.COFFSymbols))
		}
		for _, r := range section.Relocs {
			if r.SymbolTableIndex != 0 {
				t.Errorf("%s: relocation points at symbol %d", arch.GOARCH, r.SymbolTableIndex)
			}
		}

		data, err := section.Data()
		if err != nil {
			t.Fatalf("%s: section data: %v", arch.GOARCH, err)
		}
		// 재배치 자리에는 링커가 더할 값, 곧 구역 안에서의 위치가 들어 있다.
		for _, r := range section.Relocs {
			offset := binary.LittleEndian.Uint32(data[r.VirtualAddress:])
			if offset == 0 || uint64(offset) >= uint64(len(data)) {
				t.Errorf("%s: relocation addend %d is outside the section", arch.GOARCH, offset)
			}
		}
		if !bytes.Contains(data, []byte("<assembly")) {
			t.Errorf("%s: the manifest is missing from the section", arch.GOARCH)
		}
	}
}

// 같은 입력이면 언제 만들어도 같은 바이트여야 배포 산출물을 다시 만들어
// 견줄 수 있다. 시각 도장이 들어가면 그러지 못한다.
func TestWriteObjectIsReproducible(t *testing.T) {
	build := func() []byte {
		set := &rsrc.Set{}
		if err := rsrc.AddManifest(set, rsrc.DefaultManifest, 0x0409); err != nil {
			t.Fatalf("AddManifest: %v", err)
		}
		var buf bytes.Buffer
		if err := rsrc.WriteObject(&buf, rsrc.AMD64, set); err != nil {
			t.Fatalf("WriteObject: %v", err)
		}
		return buf.Bytes()
	}

	first := build()
	for i := 0; i < 5; i++ {
		if !bytes.Equal(first, build()) {
			t.Fatal("two objects built from the same resources differ")
		}
	}
	// TimeDateStamp 는 0 이어야 한다.
	if stamp := binary.LittleEndian.Uint32(first[4:]); stamp != 0 {
		t.Errorf("TimeDateStamp = %d, want 0", stamp)
	}
}

func TestWriteObjectRejectsAnEmptySet(t *testing.T) {
	if err := rsrc.WriteObject(&bytes.Buffer{}, rsrc.AMD64, &rsrc.Set{}); err == nil {
		t.Fatal("an empty set was accepted")
	}
}

func TestDuplicateResourceIsRejected(t *testing.T) {
	set := &rsrc.Set{}
	entry := rsrc.Entry{Type: rsrc.TypeVersion, Name: 1, Language: 0x0409, Data: []byte{1, 2, 3, 4}}
	if err := set.Add(entry); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := set.Add(entry); err == nil {
		t.Fatal("the same resource was accepted twice")
	}
}

func TestArchByName(t *testing.T) {
	if a, err := rsrc.ArchByName("386"); err != nil || a.GOARCH != "386" {
		t.Errorf("ArchByName(386) = %v, %v", a, err)
	}
	if _, err := rsrc.ArchByName("arm64"); err == nil {
		t.Error("an unsupported architecture was accepted")
	}
}

// --- 시험 거들기 ---

type block struct{ lo, hi uint32 }

func readBlocks(t *testing.T, data []byte) []block {
	t.Helper()
	count := binary.LittleEndian.Uint32(data)
	out := make([]block, count)
	for i := range out {
		at := 4 + 12*i
		out[i] = block{
			lo: binary.LittleEndian.Uint32(data[at:]),
			hi: binary.LittleEndian.Uint32(data[at+4:]),
		}
	}
	return out
}

// readMessages 는 FormatMessage 와 같은 방식으로 표를 훑는다.
func readMessages(t *testing.T, data []byte) map[uint32]string {
	t.Helper()
	out := make(map[uint32]string)
	count := binary.LittleEndian.Uint32(data)
	for i := 0; i < int(count); i++ {
		at := 4 + 12*i
		lo := binary.LittleEndian.Uint32(data[at:])
		hi := binary.LittleEndian.Uint32(data[at+4:])
		cursor := binary.LittleEndian.Uint32(data[at+8:])
		for id := lo; ; id++ {
			length := binary.LittleEndian.Uint16(data[cursor:])
			body := data[cursor+4 : cursor+uint32(length)]
			out[id] = utf16String(body)
			cursor += uint32(length)
			if id == hi {
				break
			}
		}
	}
	return out
}

// utf16String 은 NUL 로 끝나는 UTF-16 문자열을 읽는다. 뒤의 채움은 버린다.
func utf16String(b []byte) string {
	units := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		u := binary.LittleEndian.Uint16(b[i:])
		if u == 0 {
			break
		}
		units = append(units, u)
	}
	return string(utf16.Decode(units))
}

func utf16Bytes(s string) []byte {
	var out []byte
	for _, u := range utf16.Encode([]rune(s)) {
		out = binary.LittleEndian.AppendUint16(out, u)
	}
	return out
}

// fakeIcon 은 그림 count 개를 담은 최소한의 .ico 다.
func fakeIcon(count int) []byte {
	header := make([]byte, 6+16*count)
	binary.LittleEndian.PutUint16(header[2:], 1)
	binary.LittleEndian.PutUint16(header[4:], uint16(count))

	body := []byte{}
	for i := 0; i < count; i++ {
		image := bytes.Repeat([]byte{byte(i + 1)}, 8+i)
		at := 6 + 16*i
		header[at] = 16                                  // width
		header[at+1] = 16                                // height
		binary.LittleEndian.PutUint16(header[at+4:], 1)  // planes
		binary.LittleEndian.PutUint16(header[at+6:], 32) // bit count
		binary.LittleEndian.PutUint32(header[at+8:], uint32(len(image)))
		binary.LittleEndian.PutUint32(header[at+12:], uint32(len(header)+len(body)))
		body = append(body, image...)
	}
	return append(header, body...)
}
