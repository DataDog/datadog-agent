// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

package platform

import (
	"encoding/binary"
	"unsafe"
)

// WKSTA_INFO_100 contains platform-specific information
// see https://learn.microsoft.com/en-us/windows/win32/api/lmwksta/ns-lmwksta-wksta_info_100
// type WKSTA_INFO_100 struct {
// 	wki100_platform_id  uint32
// 	wki100_computername string
// 	wki100_langroup     string
// 	wki100_ver_major    uint32
// 	wki100_ver_minor    uint32
// }

// func byteArrayToWksaInfo(data []byte) (info WKSTA_INFO_100) {
// 	info.wki100_platform_id = binary.LittleEndian.Uint32(data)

// 	// if necessary, convert the pointer to a c-string into a GO string.
// 	// Not using at this time.  However, leaving as  a placeholder, to
// 	// show why we're skipping 4 bytes of the buffer here...

// 	//addr := (*byte)(unsafe.Pointer(uintptr(binary.LittleEndian.Uint64(data[4:]))))
// 	//info.wki100_computername = addr

// 	// ... and again here for the lan group name.
// 	//stringptr = (*[]byte)(unsafe.Pointer(uintptr(binary.LittleEndian.Uint64(data[8:]))))
// 	//info.wki100_langroup = convertWindowsString(stringptr)

// 	info.wki100_ver_major = binary.LittleEndian.Uint32(data[12:])
// 	info.wki100_ver_minor = binary.LittleEndian.Uint32(data[16:])
// 	return
// }

// func platGetVersion(outdata *byte) (maj uint64, min uint64, err error) {
// 	var info WKSTA_INFO_100
// 	var dataptr []byte
// 	dataptr = (*[20]byte)(unsafe.Pointer(outdata))[:]

// 	info = byteArrayToWksaInfo(dataptr)
// 	maj = uint64(info.wki100_ver_major)
// 	min = uint64(info.wki100_ver_minor)
// 	return
// }

func platGetServerInfo(data *byte) (si101 serverInfo101) {
	var outdata []byte
	outdata = (*[24]byte)(unsafe.Pointer(data))[:]
	si101.sv101PlatformID = binary.LittleEndian.Uint32(outdata)

	//stringptr := (*[]uint16)(unsafe.Pointer(uintptr(binary.LittleEndian.Uint64(outdata[4:]))))
	//si101.sv101Name = convertWindowsString(*stringptr)

	si101.sv101VersionMajor = binary.LittleEndian.Uint32(outdata[8:])
	si101.sv101VersionMinor = binary.LittleEndian.Uint32(outdata[12:])
	si101.sv101Type = binary.LittleEndian.Uint32(outdata[16:])

	//stringptr = (*[]uint16)(unsafe.Pointer(uintptr(binary.LittleEndian.Uint32(outdata[20:]))))
	//si101.sv101Comment = convertWindowsString(*stringptr)
	return

}
