package rsrc

import (
	"encoding/binary"
	"fmt"
)

// AddIcon 은 `.ico` 파일 하나를 아이콘 리소스로 바꿔 넣는다.
//
// 아이콘은 실행 파일 안에서 두 조각으로 나뉜다. 그림마다 ICON 리소스가
// 하나씩 생기고, 어느 그림이 어떤 크기인지 적은 GROUP_ICON 리소스가 그것들을
// 번호로 가리킨다. 탐색기가 크기에 맞는 그림을 고르는 곳이 GROUP_ICON 이다.
//
// firstIconName 은 ICON 리소스에 붙일 첫 번호다. GROUP_ICON 의 이름과 겹치지
// 않는 값을 쓴다.
func AddIcon(set *Set, data []byte, groupName, firstIconName uint16, language uint16) error {
	count, err := iconCount(data)
	if err != nil {
		return err
	}

	group := make([]byte, 6, 6+14*count)
	copy(group, data[:6])

	for i := 0; i < count; i++ {
		at := 6 + 16*i
		size := binary.LittleEndian.Uint32(data[at+8:])
		offset := binary.LittleEndian.Uint32(data[at+12:])
		if uint64(offset)+uint64(size) > uint64(len(data)) {
			return fmt.Errorf("icon %d runs past the end of the file", i)
		}

		name := firstIconName + uint16(i)
		if err := set.Add(Entry{
			Type:     TypeIcon,
			Name:     name,
			Language: language,
			Data:     data[offset : offset+size],
		}); err != nil {
			return err
		}

		// GRPICONDIRENTRY 는 ICONDIRENTRY 에서 파일 위치(4바이트)를 빼고
		// 리소스 번호(2바이트)를 붙인 것이다.
		group = append(group, data[at:at+12]...)
		group = binary.LittleEndian.AppendUint16(group, name)
	}

	return set.Add(Entry{
		Type:     TypeGroupIcon,
		Name:     groupName,
		Language: language,
		Data:     group,
	})
}

func iconCount(data []byte) (int, error) {
	if len(data) < 6 {
		return 0, fmt.Errorf("icon file is %d bytes, too short for a header", len(data))
	}
	if reserved := binary.LittleEndian.Uint16(data[0:]); reserved != 0 {
		return 0, fmt.Errorf("icon file header reserved field is %d, not 0", reserved)
	}
	if kind := binary.LittleEndian.Uint16(data[2:]); kind != 1 {
		return 0, fmt.Errorf("icon file type is %d, not 1 (icon)", kind)
	}
	count := int(binary.LittleEndian.Uint16(data[4:]))
	if count == 0 {
		return 0, fmt.Errorf("icon file holds no images")
	}
	if len(data) < 6+16*count {
		return 0, fmt.Errorf("icon file is %d bytes, too short for %d directory entries", len(data), count)
	}
	return count, nil
}

// AddManifest 는 실행 수준을 적은 매니페스트를 넣는다.
//
// 원본과 같이 asInvoker 다. ANSM 은 관리자 권한이 필요한 명령에서만 스스로
// 다시 띄우므로(L0008 2.2), 실행 파일 전체에 상승을 요구하지 않는다.
func AddManifest(set *Set, xml string, language uint16) error {
	return set.Add(Entry{
		Type:     TypeManifest,
		Name:     1, // CREATEPROCESS_MANIFEST_RESOURCE_ID
		Language: language,
		Data:     []byte(xml),
	})
}

// DefaultManifest 는 원본 나씀 실행 파일에 든 것과 같은 매니페스트다.
const DefaultManifest = `<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <trustInfo xmlns="urn:schemas-microsoft-com:asm.v3">
    <security>
      <requestedPrivileges>
        <requestedExecutionLevel level="asInvoker" uiAccess="false"></requestedExecutionLevel>
      </requestedPrivileges>
    </security>
  </trustInfo>
</assembly>`
