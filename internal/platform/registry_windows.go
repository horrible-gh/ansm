//go:build windows

package platform

import (
	"encoding/binary"
	"errors"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"ansm/internal/settings"
)

const (
	hkeyLocalMachine        = syscall.Handle(0x80000002)
	keyRead          uint32 = 0x20019
	keyWrite         uint32 = 0x20006
	keyReadWrite     uint32 = keyRead | keyWrite
	regSZ            uint32 = 1
	regExpandSZ      uint32 = 2
	regDWORD         uint32 = 4
	regMultiSZ       uint32 = 7
)

var (
	registryAdvapi32    = syscall.NewLazyDLL("advapi32.dll")
	procRegCreateKeyExW = registryAdvapi32.NewProc("RegCreateKeyExW")
	procRegEnumKeyExW   = registryAdvapi32.NewProc("RegEnumKeyExW")
	procRegEnumValueW   = registryAdvapi32.NewProc("RegEnumValueW")
	procRegDeleteTreeW  = registryAdvapi32.NewProc("RegDeleteTreeW")
	procRegSetValueExW  = registryAdvapi32.NewProc("RegSetValueExW")
	procRegDeleteValueW = registryAdvapi32.NewProc("RegDeleteValueW")
)

func openRegistryKey(path string, access uint32) (syscall.Handle, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var h syscall.Handle
	if err = syscall.RegOpenKeyEx(hkeyLocalMachine, p, 0, access, &h); err != nil {
		return 0, err
	}
	return h, nil
}

func createRegistryKey(path string) (syscall.Handle, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var h syscall.Handle
	var disposition uint32
	r, _, _ := procRegCreateKeyExW.Call(uintptr(hkeyLocalMachine), uintptr(unsafe.Pointer(p)), 0, 0, 0, uintptr(keyReadWrite), 0, uintptr(unsafe.Pointer(&h)), uintptr(unsafe.Pointer(&disposition)))
	if r != 0 {
		return 0, syscall.Errno(r)
	}
	return h, nil
}

func closeRegistryKey(h syscall.Handle) { _ = syscall.RegCloseKey(h) }

func readRegistryValue(path, name string) (Value, bool, error) {
	h, err := openRegistryKey(path, keyRead)
	if err != nil {
		if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
			return Value{}, false, nil
		}
		return Value{}, false, err
	}
	defer closeRegistryKey(h)
	n, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return Value{}, false, err
	}
	var typ, size uint32
	if err = syscall.RegQueryValueEx(h, n, nil, &typ, nil, &size); err != nil {
		if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
			return Value{}, false, nil
		}
		return Value{}, false, err
	}
	data := make([]byte, size)
	if size > 0 {
		if err = syscall.RegQueryValueEx(h, n, nil, &typ, &data[0], &size); err != nil {
			return Value{}, false, err
		}
	}
	data = data[:size]
	switch typ {
	case regDWORD:
		if len(data) < 4 {
			return Value{}, false, syscall.EINVAL
		}
		return Value{Kind: settings.KindDWORD, Number: binary.LittleEndian.Uint32(data)}, true, nil
	case regSZ:
		return Value{Kind: settings.KindSZ, Text: decodeUTF16(data)}, true, nil
	case regExpandSZ:
		return Value{Kind: settings.KindExpandSZ, Text: decodeUTF16(data)}, true, nil
	case regMultiSZ:
		return Value{Kind: settings.KindMultiSZ, Strings: decodeMultiSZ(data)}, true, nil
	default:
		return Value{}, false, nil
	}
}

func writeRegistryValue(path, name string, value Value) error {
	h, err := createRegistryKey(path)
	if err != nil {
		return err
	}
	defer closeRegistryKey(h)
	n, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	var typ uint32
	var data []byte
	switch value.Kind {
	case settings.KindDWORD:
		typ = regDWORD
		data = make([]byte, 4)
		binary.LittleEndian.PutUint32(data, value.Number)
	case settings.KindSZ:
		typ = regSZ
		data = encodeUTF16(value.Text, false)
	case settings.KindExpandSZ:
		typ = regExpandSZ
		data = encodeUTF16(value.Text, false)
	case settings.KindMultiSZ:
		typ = regMultiSZ
		data = encodeMultiSZ(value.Strings)
	default:
		return syscall.EINVAL
	}
	var p *byte
	if len(data) > 0 {
		p = &data[0]
	}
	r, _, _ := procRegSetValueExW.Call(uintptr(h), uintptr(unsafe.Pointer(n)), 0, uintptr(typ), uintptr(unsafe.Pointer(p)), uintptr(len(data)))
	if r != 0 {
		return syscall.Errno(r)
	}
	return nil
}

func deleteRegistryValue(path, name string) error {
	h, err := openRegistryKey(path, keyWrite)
	if err != nil {
		if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
			return nil
		}
		return err
	}
	defer closeRegistryKey(h)
	n, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	r, _, _ := procRegDeleteValueW.Call(uintptr(h), uintptr(unsafe.Pointer(n)))
	err = syscall.Errno(r)
	if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		return nil
	}
	return err
}

func deleteRegistryTree(path string) error {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	r, _, _ := procRegDeleteTreeW.Call(uintptr(hkeyLocalMachine), uintptr(unsafe.Pointer(p)))
	if r == 0 || syscall.Errno(r) == syscall.ERROR_FILE_NOT_FOUND {
		return nil
	}
	return syscall.Errno(r)
}

func enumRegistryKeys(path string) ([]string, error) {
	h, err := openRegistryKey(path, keyRead)
	if err != nil {
		return nil, err
	}
	defer closeRegistryKey(h)
	var out []string
	for index := uint32(0); ; index++ {
		buf := make([]uint16, 256)
		n := uint32(len(buf))
		r, _, _ := procRegEnumKeyExW.Call(uintptr(h), uintptr(index), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)), 0, 0, 0, 0)
		if syscall.Errno(r) == syscall.Errno(259) {
			break
		}
		if r != 0 {
			return nil, syscall.Errno(r)
		}
		out = append(out, syscall.UTF16ToString(buf[:n]))
	}
	return out, nil
}

func enumRegistryValues(path string) ([]string, error) {
	h, err := openRegistryKey(path, keyRead)
	if err != nil {
		if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
			return nil, nil
		}
		return nil, err
	}
	defer closeRegistryKey(h)
	var out []string
	for index := uint32(0); ; index++ {
		buf := make([]uint16, 256)
		n := uint32(len(buf))
		r, _, _ := procRegEnumValueW.Call(uintptr(h), uintptr(index), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)), 0, 0, 0, 0)
		if syscall.Errno(r) == syscall.Errno(259) {
			break
		}
		if r != 0 {
			return nil, syscall.Errno(r)
		}
		out = append(out, syscall.UTF16ToString(buf[:n]))
	}
	return out, nil
}

func encodeUTF16(s string, extraNUL bool) []byte {
	u := utf16.Encode([]rune(s))
	u = append(u, 0)
	if extraNUL {
		u = append(u, 0)
	}
	data := make([]byte, len(u)*2)
	for i, v := range u {
		binary.LittleEndian.PutUint16(data[i*2:], v)
	}
	return data
}

func encodeMultiSZ(values []string) []byte {
	var u []uint16
	for _, s := range values {
		u = append(u, utf16.Encode([]rune(s))...)
		u = append(u, 0)
	}
	u = append(u, 0)
	if len(values) == 0 {
		u = append(u, 0)
	}
	data := make([]byte, len(u)*2)
	for i, v := range u {
		binary.LittleEndian.PutUint16(data[i*2:], v)
	}
	return data
}

func decodeUTF16(data []byte) string {
	u := bytesToUTF16(data)
	for len(u) > 0 && u[len(u)-1] == 0 {
		u = u[:len(u)-1]
	}
	return string(utf16.Decode(u))
}

func decodeMultiSZ(data []byte) []string {
	u := bytesToUTF16(data)
	var out []string
	for len(u) > 0 {
		end := 0
		for end < len(u) && u[end] != 0 {
			end++
		}
		if end == 0 {
			break
		}
		out = append(out, string(utf16.Decode(u[:end])))
		u = u[end+1:]
	}
	return out
}

func bytesToUTF16(data []byte) []uint16 {
	u := make([]uint16, len(data)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(data[i*2:])
	}
	return u
}
