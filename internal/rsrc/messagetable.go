package rsrc

import (
	"encoding/binary"
	"fmt"
	"sort"
	"unicode/utf16"

	"ansm/internal/msgcat"
)

// messageResourceUnicode follows the documented behavioral contract. See Flags, UTF.
const messageResourceUnicode = 1

// MessageTable follows the documented behavioral contract. See MessageTable, MESSAGETABLE, DWORD, NumberOfBlocks, LowId, HighId.
func MessageTable(texts map[uint32]string) ([]byte, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("message table: %w", ErrEmpty)
	}

	ids := make([]uint32, 0, len(texts))
	for id := range texts {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	// block follows the documented behavioral contract.
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

// messageEntry follows the documented behavioral contract. See UTF, NUL, Length.
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

// AddMessageTables follows the documented behavioral contract. See AddMessageTables, MESSAGETABLE.
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
			// This section follows the documented behavioral contract.
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
