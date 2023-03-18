// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package common

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var getUUID = GetUUID

// GetUUID returns the machine GUID on windows; copied from gopsutil
func GetUUID() string {
	var h windows.Handle
	err := windows.RegOpenKeyEx(windows.HKEY_LOCAL_MACHINE, windows.StringToUTF16Ptr(`SOFTWARE\Microsoft\Cryptography`), 0, windows.KEY_READ|windows.KEY_WOW64_64KEY, &h)
	if err != nil {
		log.Warnf("Failed to open registry key Cryptography: %v", err)
		return ""
	}
	defer windows.RegCloseKey(h)

	const windowsRegBufLen = 74 // len(`{`) + len(`abcdefgh-1234-456789012-123345456671` * 2) + len(`}`) // 2 == bytes/UTF16
	const uuidLen = 36

	var regBuf [windowsRegBufLen]uint16
	bufLen := uint32(windowsRegBufLen)
	var valType uint32
	err = windows.RegQueryValueEx(h, windows.StringToUTF16Ptr(`MachineGuid`), nil, &valType, (*byte)(unsafe.Pointer(&regBuf[0])), &bufLen)
	if err != nil {
		log.Warnf("Could not find machineguid in the registry %v", err)
		return ""
	}

	hostID := windows.UTF16ToString(regBuf[:])
	hostIDLen := len(hostID)
	if hostIDLen != uuidLen {
		log.Warnf("the hostid was unexpected length (%d != %d)", hostIDLen, uuidLen)
		return ""
	}

	return strings.ToLower(hostID)
}
