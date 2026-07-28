package rsrc

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Arch 는 오브젝트를 링크할 대상이다. 파일 이름 규약(`_windows_amd64.syso`)과
// COFF 머리글의 Machine 값이 어긋나면 링커가 조용히 리소스를 버린다.
type Arch struct {
	// GOARCH 는 Go 가 쓰는 이름이다.
	GOARCH string
	// machine 은 COFF IMAGE_FILE_MACHINE_* 다.
	machine uint16
	// relocType 은 "이미지 시작을 0 으로 본 32비트 주소" 재배치다.
	relocType uint16
}

var (
	// AMD64 는 64비트 빌드다.
	AMD64 = Arch{GOARCH: "amd64", machine: 0x8664, relocType: 0x0003} // IMAGE_REL_AMD64_ADDR32NB
	// I386 는 32비트 빌드다. P0007 2.1 의 NSSM_CONFIGURATION "32-bit".
	I386 = Arch{GOARCH: "386", machine: 0x014c, relocType: 0x0007} // IMAGE_REL_I386_DIR32NB
)

// Arches 는 배포가 내는 모든 대상이다.
var Arches = []Arch{AMD64, I386}

// ArchByName 은 GOARCH 이름으로 대상을 찾는다.
func ArchByName(goarch string) (Arch, error) {
	for _, a := range Arches {
		if a.GOARCH == goarch {
			return a, nil
		}
	}
	return Arch{}, fmt.Errorf("unsupported GOARCH %q for resources", goarch)
}

// 구역 속성. winnt.h 의 IMAGE_SCN_*.
const (
	scnCntInitializedData = 0x00000040
	scnAlign8Bytes        = 0x00400000
	scnMemRead            = 0x40000000
)

const (
	coffHeaderSize    = 20
	sectionHeaderSize = 40
	relocationSize    = 10
	symbolSize        = 18
)

// WriteObject 는 리소스 모음을 `.syso` 오브젝트로 쓴다.
//
// 만드는 오브젝트는 구역 하나(`.rsrc`)와 심볼 하나(구역 자신)를 가진다.
// 자료 항목의 OffsetToData 자리마다 재배치를 하나 걸어 두면, Go 링커가
// 실행 파일에서 `.rsrc` 가 놓인 주소를 더해 완성한다. 더할 값은 재배치
// 자리에 미리 적어 둔 구역 내 상대 위치다(COFF 는 더할 값을 필드에 담는다).
func WriteObject(w io.Writer, arch Arch, set *Set) error {
	entries := set.Entries()
	if len(entries) == 0 {
		return ErrEmpty
	}
	body, err := buildDirectory(entries)
	if err != nil {
		return err
	}
	if len(body.fixups) > 0xffff {
		return fmt.Errorf("%d resources need more relocations than a COFF section can hold", len(body.fixups))
	}

	rawDataAt := coffHeaderSize + sectionHeaderSize
	relocationsAt := rawDataAt + len(body.data)
	symbolsAt := relocationsAt + relocationSize*len(body.fixups)

	buf := make([]byte, 0, symbolsAt+symbolSize+4)
	put16 := func(v uint16) { buf = binary.LittleEndian.AppendUint16(buf, v) }
	put32 := func(v uint32) { buf = binary.LittleEndian.AppendUint32(buf, v) }

	// IMAGE_FILE_HEADER. TimeDateStamp 는 0 으로 둔다. 같은 입력이 언제
	// 만들어도 같은 바이트를 내야 배포 산출물을 다시 만들어 견줄 수 있다.
	put16(arch.machine)
	put16(1) // NumberOfSections
	put32(0) // TimeDateStamp
	put32(uint32(symbolsAt))
	put32(1) // NumberOfSymbols
	put16(0) // SizeOfOptionalHeader
	put16(0) // Characteristics

	// IMAGE_SECTION_HEADER.
	buf = append(buf, []byte(".rsrc\x00\x00\x00")...)
	put32(0) // VirtualSize — 오브젝트에서는 쓰지 않는다.
	put32(0) // VirtualAddress
	put32(uint32(len(body.data)))
	put32(uint32(rawDataAt))
	put32(uint32(relocationsAt))
	put32(0) // PointerToLinenumbers
	put16(uint16(len(body.fixups)))
	put16(0) // NumberOfLinenumbers
	put32(scnCntInitializedData | scnAlign8Bytes | scnMemRead)

	buf = append(buf, body.data...)

	for _, at := range body.fixups {
		put32(at)
		put32(0) // SymbolTableIndex — 아래의 구역 심볼
		put16(arch.relocType)
	}

	// IMAGE_SYMBOL: 구역 자신을 가리키는 정적 심볼.
	buf = append(buf, []byte(".rsrc\x00\x00\x00")...)
	put32(0)             // Value
	put16(1)             // SectionNumber — 1부터 센다
	put16(0)             // Type
	buf = append(buf, 3) // StorageClass = IMAGE_SYM_CLASS_STATIC
	buf = append(buf, 0) // NumberOfAuxSymbols

	// 문자열 표. 비어 있어도 길이 4바이트는 있어야 한다.
	put32(4)

	_, err = w.Write(buf)
	return err
}
