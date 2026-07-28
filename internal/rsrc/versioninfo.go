package rsrc

import (
	"encoding/binary"
	"fmt"
	"sort"
	"unicode/utf16"
)

// VersionInfo 는 파일 속성 창에 보이는 값이다.
//
// 원본 나씀의 VERSIONINFO 를 그대로 옮긴 자리 구성이다. 값은 빌드할 때
// 채운다(tools/mkrsrc 참고).
type VersionInfo struct {
	// FileVersion 과 ProductVersion 은 네 자리 수 버전이다.
	FileVersion    [4]uint16
	ProductVersion [4]uint16
	// PreRelease 는 태그에 정확히 맞지 않는 빌드다(VS_FF_PRERELEASE).
	PreRelease bool
	// Strings 는 문자열 표에 넣을 이름-값 쌍이다.
	Strings map[string]string
	// Translations 는 (언어, 코드페이지) 쌍이다. 문자열 표는 첫 쌍의 이름으로 만든다.
	Translations []Translation
}

// Translation 은 VarFileInfo 의 한 쌍이다.
type Translation struct {
	Language uint16
	CodePage uint16
}

// VS_FIXEDFILEINFO 의 상수. winver.h.
const (
	fixedFileInfoSignature = 0xfeef04bd
	fixedFileInfoStruct    = 0x00010000
	fileFlagPreRelease     = 0x00000002
	fileFlagsMask          = 0x0000003f
	fileOSNTWindows32      = 0x00040004
	fileTypeApp            = 0x00000001
)

// 자료 종류. 0 은 이진, 1 은 글자.
const (
	valueBinary = 0
	valueText   = 1
)

// Build 는 VS_VERSIONINFO 자료를 만든다.
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
	// dwFileSubtype·dwFileDate 는 응용 프로그램에서 0 이다.

	first := v.Translations[0]
	table := &node{
		key:       fmt.Sprintf("%04X%04X", first.Language, first.CodePage),
		valueType: valueText,
	}
	// 지도 순회 순서가 산출물을 바꾸지 않도록 이름으로 정렬한다.
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

// node 는 버전 자료의 한 칸이다. 모든 칸이 같은 머리글을 쓴다.
//
//	WORD wLength      머리글까지 포함한 이 칸의 길이
//	WORD wValueLength 값의 길이(글자 값이면 글자 수, 이진 값이면 바이트 수)
//	WORD wType        0 이진, 1 글자
//	WCHAR szKey[]     이름, NUL 로 끝남
//	                  4바이트 자리 맞춤
//	BYTE  Value[]     값
//	                  4바이트 자리 맞춤
//	칸들                자식
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
		valueLength = (len(out) - before) / 2 // 글자 값은 글자 수로 센다
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
