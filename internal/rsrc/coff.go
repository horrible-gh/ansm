package rsrc

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Arch follows the documented behavioral contract. See Arch, COFF, Machine.
type Arch struct {
	// GOARCH follows the documented behavioral contract. See GOARCH, Go.
	GOARCH string
	// machine follows the documented behavioral contract. See COFF.
	machine uint16
	// relocType follows the documented behavioral contract.
	relocType uint16
}

var (
	// This section follows the documented behavioral contract. See AMD64.
	AMD64 = Arch{GOARCH: "amd64", machine: 0x8664, relocType: 0x0003} // IMAGE_REL_AMD64_ADDR32NB
	// This section follows the documented behavioral contract. See I386, P0007 2.1, NSSM_CONFIGURATION.
	I386 = Arch{GOARCH: "386", machine: 0x014c, relocType: 0x0007} // IMAGE_REL_I386_DIR32NB
)

// Arches follows the documented behavioral contract. See Arches.
var Arches = []Arch{AMD64, I386}

// ArchByName follows the documented behavioral contract. See ArchByName, GOARCH.
func ArchByName(goarch string) (Arch, error) {
	for _, a := range Arches {
		if a.GOARCH == goarch {
			return a, nil
		}
	}
	return Arch{}, fmt.Errorf("unsupported GOARCH %q for resources", goarch)
}

// This section follows the documented behavioral contract.
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

// WriteObject follows the documented behavioral contract. See WriteObject, OffsetToData, Go, COFF.
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

	// This section follows the documented behavioral contract. See TimeDateStamp.
	put16(arch.machine)
	put16(1) // NumberOfSections
	put32(0) // TimeDateStamp
	put32(uint32(symbolsAt))
	put32(1) // NumberOfSymbols
	put16(0) // SizeOfOptionalHeader
	put16(0) // Characteristics

	// IMAGE_SECTION_HEADER.
	buf = append(buf, []byte(".rsrc\x00\x00\x00")...)
	put32(0) // put32 follows the documented contract. See VirtualSize.
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
		put32(0) // put32 follows the documented contract. See SymbolTableIndex.
		put16(arch.relocType)
	}

	// This section follows the documented behavioral contract.
	buf = append(buf, []byte(".rsrc\x00\x00\x00")...)
	put32(0)             // Value
	put16(1)             // put16 follows the documented contract. See SectionNumber.
	put16(0)             // Type
	buf = append(buf, 3) // StorageClass = IMAGE_SYM_CLASS_STATIC
	buf = append(buf, 0) // NumberOfAuxSymbols

	// This section follows the documented behavioral contract.
	put32(4)

	_, err = w.Write(buf)
	return err
}
