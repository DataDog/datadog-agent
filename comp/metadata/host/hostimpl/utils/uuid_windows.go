// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Portions of this code are taken from the gopsutil project
// https://github.com/shirou/gopsutil .  This code is licensed under the New BSD License
// copyright WAKAYAMA Shirou, and the gopsutil contributors

//go:build windows

package utils

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetUUID returns the machine GUID on windows; copied from gopsutil
func getUUID() string {
	guid, _ := cache.Get[string](
		guidCacheKey,
		func() (string, error) {
			var h windows.Handle
			err := windows.RegOpenKeyEx(
				windows.HKEY_LOCAL_MACHINE,
				windows.StringToUTF16Ptr(`SOFTWARE\Microsoft\Cryptography`),
				0,
				windows.KEY_READ|windows.KEY_WOW64_64KEY,
				&h)
			if err != nil {
				return "", log.Warnf("Failed to open registry key Cryptography: %v", err)
			}

			defer windows.RegCloseKey(h)

			// len(`{`) + len(`abcdefgh-1234-456789012-123345456671` * 2) + len(`}`) // 2 == bytes/UTF16
			const windowsRegBufLen = 74
			const uuidLen = 36

			var regBuf [windowsRegBufLen]uint16
			var valType uint32
			bufLen := uint32(windowsRegBufLen)

			err = windows.RegQueryValueEx(
				h,
				windows.StringToUTF16Ptr(`MachineGuid`),
				nil,
				&valType,
				(*byte)(unsafe.Pointer(&regBuf[0])),
				&bufLen)
			if err != nil {
				return "", log.Warnf("Could not find machineguid in the registry %v", err)
			}

			hostID := windows.UTF16ToString(regBuf[:])
			hostIDLen := len(hostID)
			if hostIDLen != uuidLen {
				return "", log.Warnf("the hostid was unexpected length (%d != %d)", hostIDLen, uuidLen)
			}
			return strings.ToLower(hostID), nil
		})
	return guid
}
