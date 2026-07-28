package rsrc

import (
	"encoding/binary"
	"fmt"
)

// dataAlignment 은 리소스 자료를 놓는 간격이다. 원본 실행 파일의 리소스도
// 8바이트 자리에 놓여 있다. VS_VERSIONINFO 는 4바이트 정렬을 요구하므로
// 8은 그 조건을 덮는다.
const dataAlignment = 8

// blob 은 `.rsrc` 구역 하나의 내용과, 실행 파일 주소로 고쳐야 할 자리들이다.
type blob struct {
	data []byte
	// fixups 는 IMAGE_RESOURCE_DATA_ENTRY.OffsetToData 필드의 구역 내 위치다.
	// 이 필드에는 구역 시작을 0 으로 본 상대 위치를 적어 두고, 링커가 구역이
	// 놓인 실제 주소를 더하게 한다.
	fixups []uint32
}

// buildDirectory 는 종류·이름·언어 세 층의 리소스 디렉터리와 자료를 이어 붙인다.
//
// 배치는 층별로 모은다. 1층 디렉터리, 2층 디렉터리들, 3층 디렉터리들,
// 자료 항목들, 그리고 자료 본문. 자료 본문만 정렬 간격을 맞춘다.
func buildDirectory(entries []Entry) (blob, error) {
	if len(entries) == 0 {
		return blob{}, ErrEmpty
	}

	// 층별로 묶는다. entries 는 이미 정렬되어 있다.
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

	// 1차: 자리 계산.
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

	// 2차: 쓰기.
	out := blob{data: make([]byte, offset)}
	putDirectory := func(at, count int) {
		// Characteristics·TimeDateStamp·버전은 원본 리소스 컴파일러도 0 으로 둔다.
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
				// 3층 항목은 디렉터리가 아니라 자료 항목을 가리키므로 최상위 비트가 없다.
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
