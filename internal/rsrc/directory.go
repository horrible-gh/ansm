package rsrc

import (
	"encoding/binary"
	"fmt"
)

// dataAlignment follows the documented behavioral contract.
const dataAlignment = 8

// blob follows the documented behavioral contract.
type blob struct {
	data []byte
	// fixups follows the documented behavioral contract. See OffsetToData.
	fixups []uint32
}

// buildDirectory follows the documented behavioral contract.
func buildDirectory(entries []Entry) (blob, error) {
	if len(entries) == 0 {
		return blob{}, ErrEmpty
	}

	// nameGroup follows the documented behavioral contract.
	type nameGroup struct {
		name    uint16
		entries []Entry
	}
	type typeGroup struct {
		typ   uint16
		names []nameGroup
	}
	var types []typeGroup
	for _, e := range entries {
		if len(types) == 0 || types[len(types)-1].typ != e.Type {
			types = append(types, typeGroup{typ: e.Type})
		}
		t := &types[len(types)-1]
		if len(t.names) == 0 || t.names[len(t.names)-1].name != e.Name {
			t.names = append(t.names, nameGroup{name: e.Name})
		}
		n := &t.names[len(t.names)-1]
		n.entries = append(n.entries, e)
	}

	const dirSize = 16
	const dirEntrySize = 8
	const dataEntrySize = 16

	// This section follows the documented behavioral contract.
	offset := dirSize + dirEntrySize*len(types)
	nameDirOffset := make([]int, len(types))
	for i, t := range types {
		nameDirOffset[i] = offset
		offset += dirSize + dirEntrySize*len(t.names)
	}
	langDirOffset := make([][]int, len(types))
	for i, t := range types {
		langDirOffset[i] = make([]int, len(t.names))
		for j, n := range t.names {
			langDirOffset[i][j] = offset
			offset += dirSize + dirEntrySize*len(n.entries)
		}
	}
	dataEntryOffset := make([][][]int, len(types))
	for i, t := range types {
		dataEntryOffset[i] = make([][]int, len(t.names))
		for j, n := range t.names {
			dataEntryOffset[i][j] = make([]int, len(n.entries))
			for k := range n.entries {
				dataEntryOffset[i][j][k] = offset
				offset += dataEntrySize
			}
		}
	}
	dataOffset := make([][][]int, len(types))
	for i, t := range types {
		dataOffset[i] = make([][]int, len(t.names))
		for j, n := range t.names {
			dataOffset[i][j] = make([]int, len(n.entries))
			for k, e := range n.entries {
				offset = align(offset, dataAlignment)
				dataOffset[i][j][k] = offset
				offset += len(e.Data)
			}
		}
	}

	// This section follows the documented behavioral contract.
	out := blob{data: make([]byte, offset)}
	putDirectory := func(at, count int) {
		// This section follows the documented behavioral contract. See Characteristics, TimeDateStamp.
		binary.LittleEndian.PutUint16(out.data[at+12:], 0)             // NumberOfNamedEntries
		binary.LittleEndian.PutUint16(out.data[at+14:], uint16(count)) // NumberOfIdEntries
	}
	putSubdirEntry := func(at int, id uint16, target int) {
		binary.LittleEndian.PutUint32(out.data[at+0:], uint32(id))
		binary.LittleEndian.PutUint32(out.data[at+4:], uint32(target)|0x80000000)
	}

	putDirectory(0, len(types))
	for i, t := range types {
		putSubdirEntry(dirSize+dirEntrySize*i, t.typ, nameDirOffset[i])

		putDirectory(nameDirOffset[i], len(t.names))
		for j, n := range t.names {
			putSubdirEntry(nameDirOffset[i]+dirSize+dirEntrySize*j, n.name, langDirOffset[i][j])

			putDirectory(langDirOffset[i][j], len(n.entries))
			for k, e := range n.entries {
				at := langDirOffset[i][j] + dirSize + dirEntrySize*k
				binary.LittleEndian.PutUint32(out.data[at+0:], uint32(e.Language))
				// This section follows the documented behavioral contract.
				binary.LittleEndian.PutUint32(out.data[at+4:], uint32(dataEntryOffset[i][j][k]))

				de := dataEntryOffset[i][j][k]
				binary.LittleEndian.PutUint32(out.data[de+0:], uint32(dataOffset[i][j][k]))
				binary.LittleEndian.PutUint32(out.data[de+4:], uint32(len(e.Data)))
				binary.LittleEndian.PutUint32(out.data[de+8:], 0) // CodePage
				binary.LittleEndian.PutUint32(out.data[de+12:], 0)
				out.fixups = append(out.fixups, uint32(de))

				copy(out.data[dataOffset[i][j][k]:], e.Data)
			}
		}
	}

	if len(out.fixups) != len(entries) {
		return blob{}, fmt.Errorf("internal error: %d fixups for %d resources", len(out.fixups), len(entries))
	}
	return out, nil
}

func align(n, to int) int {
	if r := n % to; r != 0 {
		return n + to - r
	}
	return n
}
