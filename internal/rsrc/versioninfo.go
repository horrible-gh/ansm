package rsrc

import (
	"encoding/binary"
	"fmt"
	"sort"
	"unicode/utf16"
)

// VersionInfo follows the documented behavioral contract. See VersionInfo, VERSIONINFO.
type VersionInfo struct {
	// FileVersion follows the documented behavioral contract. See FileVersion, ProductVersion.
	FileVersion    [4]uint16
	ProductVersion [4]uint16
	// PreRelease follows the documented behavioral contract. See PreRelease.
	PreRelease bool
	// Strings follows the documented behavioral contract. See Strings.
	Strings map[string]string
	// Translations follows the documented behavioral contract. See Translations.
	Translations []Translation
}

// Translation follows the documented behavioral contract. See Translation, VarFileInfo.
type Translation struct {
	Language uint16
	CodePage uint16
}

// This section follows the documented behavioral contract.
const (
	fixedFileInfoSignature = 0xfeef04bd
	fixedFileInfoStruct    = 0x00010000
	fileFlagPreRelease     = 0x00000002
	fileFlagsMask          = 0x0000003f
	fileOSNTWindows32      = 0x00040004
	fileTypeApp            = 0x00000001
)

// This section follows the documented behavioral contract.
const (
	valueBinary = 0
	valueText   = 1
)

// Build follows the documented behavioral contract. See Build.
func (v VersionInfo) Build() ([]byte, error) {
	if len(v.Translations) == 0 {
		return nil, fmt.Errorf("version info: at least one translation is required")
	}
	if len(v.Strings) == 0 {
		return nil, fmt.Errorf("version info: at least one string is required")
	}

	fixed := make([]byte, 52)
	put := func(at int, value uint32) { binary.LittleEndian.PutUint32(fixed[at:], value) }
	put(0, fixedFileInfoSignature)
	put(4, fixedFileInfoStruct)
	put(8, uint32(v.FileVersion[0])<<16|uint32(v.FileVersion[1]))
	put(12, uint32(v.FileVersion[2])<<16|uint32(v.FileVersion[3]))
	put(16, uint32(v.ProductVersion[0])<<16|uint32(v.ProductVersion[1]))
	put(20, uint32(v.ProductVersion[2])<<16|uint32(v.ProductVersion[3]))
	put(24, fileFlagsMask)
	if v.PreRelease {
		put(28, fileFlagPreRelease)
	}
	put(32, fileOSNTWindows32)
	put(36, fileTypeApp)
	// This section follows the documented behavioral contract.

	first := v.Translations[0]
	table := &node{
		key:       fmt.Sprintf("%04X%04X", first.Language, first.CodePage),
		valueType: valueText,
	}
	// This section follows the documented behavioral contract.
	names := make([]string, 0, len(v.Strings))
	for name := range v.Strings {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		table.children = append(table.children, &node{
			key:       name,
			valueType: valueText,
			text:      v.Strings[name],
		})
	}

	var translations []byte
	for _, t := range v.Translations {
		translations = binary.LittleEndian.AppendUint16(translations, t.Language)
		translations = binary.LittleEndian.AppendUint16(translations, t.CodePage)
	}

	root := &node{
		key:       "VS_VERSION_INFO",
		valueType: valueBinary,
		binary:    fixed,
		children: []*node{
			{key: "StringFileInfo", valueType: valueText, children: []*node{table}},
			{key: "VarFileInfo", valueType: valueText, children: []*node{
				{key: "Translation", valueType: valueBinary, binary: translations},
			}},
		},
	}
	return root.build(), nil
}

// node follows the documented behavioral contract. See WORD, WCHAR, NUL, BYTE, Value.
type node struct {
	key       string
	valueType uint16
	text      string
	binary    []byte
	children  []*node
}

func (n *node) build() []byte {
	out := make([]byte, 6)
	out = appendUTF16NUL(out, n.key)
	out = padTo4(out)

	var valueLength int
	switch {
	case n.text != "":
		before := len(out)
		out = appendUTF16NUL(out, n.text)
		valueLength = (len(out) - before) / 2 // valueLength follows the documented contract.
	case len(n.binary) > 0:
		out = append(out, n.binary...)
		valueLength = len(n.binary)
	}
	out = padTo4(out)

	for _, c := range n.children {
		out = append(out, c.build()...)
		out = padTo4(out)
	}

	binary.LittleEndian.PutUint16(out[0:], uint16(len(out)))
	binary.LittleEndian.PutUint16(out[2:], uint16(valueLength))
	binary.LittleEndian.PutUint16(out[4:], n.valueType)
	return out
}

func appendUTF16NUL(out []byte, s string) []byte {
	for _, u := range utf16.Encode([]rune(s)) {
		out = binary.LittleEndian.AppendUint16(out, u)
	}
	return binary.LittleEndian.AppendUint16(out, 0)
}

func padTo4(out []byte) []byte {
	for len(out)%4 != 0 {
		out = append(out, 0)
	}
	return out
}
