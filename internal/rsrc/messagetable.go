package rsrc

import (
	"encoding/binary"
	"fmt"
	"sort"
	"unicode/utf16"

	"ansm/internal/msgcat"
)

// 메시지 항목의 Flags. 1 은 문구가 UTF-16 이라는 뜻이다(MESSAGE_RESOURCE_UNICODE).
const messageResourceUnicode = 1

// MessageTable 은 한 언어의 MESSAGETABLE 자료다.
//
// 구조는 winnt.h 의 MESSAGE_RESOURCE_DATA 다.
//
//	DWORD NumberOfBlocks
//	MESSAGE_RESOURCE_BLOCK[NumberOfBlocks]  { DWORD LowId; DWORD HighId; DWORD OffsetToEntries }
//	MESSAGE_RESOURCE_ENTRY[]               { WORD Length; WORD Flags; WCHAR Text[] }
//
// 번호가 이어지는 구간마다 블록 하나를 만든다. FormatMessage 는 블록 목록을
// 훑어 번호가 든 구간을 찾고, 그 구간의 첫 항목부터 Length 만큼씩 건너뛰어
// 원하는 항목에 닿는다. 그래서 항목 길이가 한 바이트라도 틀리면 그 뒤 문구가
// 전부 밀린다.
func MessageTable(texts map[uint32]string) ([]byte, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("message table: %w", ErrEmpty)
	}

	ids := make([]uint32, 0, len(texts))
	for id := range texts {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	// 이어지는 번호를 블록으로 묶는다.
	type block struct{ lo, hi uint32 }
	var blocks []block
	for i, id := range ids {
		if i > 0 && id == ids[i-1]+1 {
			blocks[len(blocks)-1].hi = id
			continue
		}
		blocks = append(blocks, block{lo: id, hi: id})
	}

	entries := make(map[uint32][]byte, len(ids))
	for _, id := range ids {
		e, err := messageEntry(texts[id])
		if err != nil {
			return nil, fmt.Errorf("message %#x: %w", id, err)
		}
		entries[id] = e
	}

	header := 4 + 12*len(blocks)
	out := make([]byte, header, header+len(ids)*64)
	binary.LittleEndian.PutUint32(out[0:], uint32(len(blocks)))

	for i, b := range blocks {
		off := 4 + 12*i
		binary.LittleEndian.PutUint32(out[off+0:], b.lo)
		binary.LittleEndian.PutUint32(out[off+4:], b.hi)
		binary.LittleEndian.PutUint32(out[off+8:], uint32(len(out)))
		for id := b.lo; ; id++ {
			out = append(out, entries[id]...)
			if id == b.hi {
				break
			}
		}
	}
	return out, nil
}

// messageEntry 는 MESSAGE_RESOURCE_ENTRY 하나를 만든다.
//
// 문구는 UTF-16 이고 NUL 로 끝난다. Length 는 머리 4바이트를 포함한 전체
// 길이이며 4의 배수로 맞춘다. 남는 자리는 NUL 문자로 채운다 — 원본 나씀
// 실행 파일에서 확인한 채움 방식과 같다.
func messageEntry(text string) ([]byte, error) {
	units := utf16.Encode([]rune(text))
	units = append(units, 0)
	for (4+2*len(units))%4 != 0 {
		units = append(units, 0)
	}

	length := 4 + 2*len(units)
	if length > 0xffff {
		return nil, fmt.Errorf("text is %d bytes, which does not fit in the 16-bit Length field", length)
	}

	out := make([]byte, length)
	binary.LittleEndian.PutUint16(out[0:], uint16(length))
	binary.LittleEndian.PutUint16(out[2:], messageResourceUnicode)
	for i, u := range units {
		binary.LittleEndian.PutUint16(out[4+2*i:], u)
	}
	return out, nil
}

// AddMessageTables 는 목록의 언어마다 MESSAGETABLE 리소스 하나를 더한다.
//
// 원본과 같이 언어를 한 표에 섞지 않고 언어별 리소스로 나눈다. 그래야
// 이벤트 뷰어가 보는 사람의 언어에 맞는 문구를 고른다.
func AddMessageTables(set *Set, catalog *msgcat.Catalog) error {
	for _, lang := range catalog.Languages {
		texts := make(map[uint32]string, len(catalog.Messages))
		for _, m := range catalog.Messages {
			text, ok := m.Texts[lang.ID]
			if !ok {
				return fmt.Errorf("message %d (%s) has no %s text", m.Code, m.Symbol, lang.Name)
			}
			id := m.ID()
			if _, dup := texts[id]; dup {
				return fmt.Errorf("message id %#x is defined twice", id)
			}
			// 원본이 심는 문구는 마지막 줄바꿈까지 포함한다. 목록 파일의
			// 마침표 줄은 문구의 일부가 아니므로 여기서 줄바꿈을 되살린다.
			texts[id] = text + "\r\n"
		}
		data, err := MessageTable(texts)
		if err != nil {
			return fmt.Errorf("%s message table: %w", lang.Name, err)
		}
		if err := set.Add(Entry{
			Type:     TypeMessageTable,
			Name:     1,
			Language: lang.ID,
			Data:     data,
		}); err != nil {
			return err
		}
	}
	return nil
}
