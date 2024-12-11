// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

package platform

import (
	"encoding/binary"
	"unsafe"
)

func platGetServerInfo(data *byte) (si101 SERVER_INFO_101) {
	var outdata = (*[40]byte)(unsafe.Pointer(data))[:]
	si101.sv101_platform_id = binary.LittleEndian.Uint32(outdata)

	// due to 64 bit packing, skip 8 bytes to get to the name string
	//stringptr := *(*[]uint16)(unsafe.Pointer(uintptr(binary.LittleEndian.Uint64(outdata[8:]))))
	//si101.sv101_name = convertWindowsString(stringptr)

	si101.sv101_version_major = binary.LittleEndian.Uint32(outdata[16:])
	si101.sv101_version_minor = binary.LittleEndian.Uint32(outdata[20:])
	si101.sv101_type = binary.LittleEndian.Uint32(outdata[24:])

	// again skip 4 more for byte packing, so start at 32
	//stringptr = (*[]uint16)(unsafe.Pointer(uintptr(binary.LittleEndian.Uint32(outdata[32:]))))
	//si101.sv101_comment = convertWindowsString(*stringptr)
	return
}
