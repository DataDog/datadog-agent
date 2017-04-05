package platform

import (
	"encoding/binary"
	"unsafe"
)

type WKSTA_INFO_100 struct {
	wki100_platform_id  uint32
	wki100_computername string
	wki100_langroup     string
	wki100_ver_major    uint32
	wki100_ver_minor    uint32
}

func byteArrayToWksaInfo(data []byte) (info WKSTA_INFO_100) {
	info.wki100_platform_id = binary.LittleEndian.Uint32(data)

	// if necessary, convert the pointer to a c-string into a GO string.
	// Not using at this time.  However, leaving as  a placeholder, to
	// show why we're skipping 4 bytes of the buffer here...

	//addr := (*byte)(unsafe.Pointer(uintptr(binary.LittleEndian.Uint64(data[4:]))))
	//info.wki100_computername = addr

	// ... and again here for the lan group name.
	//stringptr = (*[]byte)(unsafe.Pointer(uintptr(binary.LittleEndian.Uint64(data[8:]))))
	//info.wki100_langroup = convert_windows_string(stringptr)

	info.wki100_ver_major = binary.LittleEndian.Uint32(data[12:])
	info.wki100_ver_minor = binary.LittleEndian.Uint32(data[16:])
	return
}
func platGetVersion(outdata *byte) (maj uint64, min uint64, err error) {
	var info WKSTA_INFO_100
	var dataptr []byte
	dataptr = (*[20]byte)(unsafe.Pointer(outdata))[:]

	info = byteArrayToWksaInfo(dataptr)
	maj = uint64(info.wki100_ver_major)
	min = uint64(info.wki100_ver_minor)
	return
}
