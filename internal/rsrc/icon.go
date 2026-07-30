package rsrc

import (
	"encoding/binary"
	"fmt"
)

// AddIcon follows the documented behavioral contract. See AddIcon, ICON.
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

		// This section follows the documented behavioral contract. See GRPICONDIRENTRY, ICONDIRENTRY.
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

// AddManifest follows the documented behavioral contract. See AddManifest, ANSM, L0008 2.2.
func AddManifest(set *Set, xml string, language uint16) error {
	return set.Add(Entry{
		Type:     TypeManifest,
		Name:     1, // CREATEPROCESS_MANIFEST_RESOURCE_ID
		Language: language,
		Data:     []byte(xml),
	})
}

// DefaultManifest follows the documented behavioral contract. See DefaultManifest.
const DefaultManifest = `<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <trustInfo xmlns="urn:schemas-microsoft-com:asm.v3">
    <security>
      <requestedPrivileges>
        <requestedExecutionLevel level="asInvoker" uiAccess="false"></requestedExecutionLevel>
      </requestedPrivileges>
    </security>
  </trustInfo>
</assembly>`
