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
	var outdata = (*[24]byte)(unsafe.Pointer(data))[:]
	si101.sv101_platform_id = binary.LittleEndian.Uint32(outdata)

	//stringptr := (*[]uint16)(unsafe.Pointer(uintptr(binary.LittleEndian.Uint64(outdata[4:]))))
	//si101.sv101_name = convertWindowsString(*stringptr)

	si101.sv101_version_major = binary.LittleEndian.Uint32(outdata[8:])
	si101.sv101_version_minor = binary.LittleEndian.Uint32(outdata[12:])
	si101.sv101_type = binary.LittleEndian.Uint32(outdata[16:])

	//stringptr = (*[]uint16)(unsafe.Pointer(uintptr(binary.LittleEndian.Uint32(outdata[20:]))))
	//si101.sv101_comment = convertWindowsString(*stringptr)
	return

}
