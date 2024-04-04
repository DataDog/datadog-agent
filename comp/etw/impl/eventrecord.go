// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package etwimpl

import (
	"encoding/binary"
	"unsafe"

	"github.com/DataDog/datadog-agent/comp/etw"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

// team: windows-agent

type userInfo struct {
	data []byte
}

// GetUserData gets the user data from the event record.
// Because this is a pointer into the event record, it will not persist beyond the lifetime of the event record.
// the data must be copied if it needs to be used after the event record out of scope.
func GetUserData(event *etw.DDEventRecord) etw.UserData {
	return &userInfo{
		data: unsafe.Slice(event.UserData, event.UserDataLength),
	}
}

// ParseUnicodeString reads from the userdata field, and returns the string
// the next offset in the buffer of the next field, whether the value was found,
// and if a terminating null was found, the index of that
func (u *userInfo) ParseUnicodeString(offset int) (val string, nextOffset int, valFound bool, foundTermZeroIdx int) {
	termZeroIdx := bytesIndexOfDoubleZero(u.data[offset:])
	var lenString int
	var skip int
	if termZeroIdx == 0 || termZeroIdx%2 == 1 {
		return "", -1, false, offset + termZeroIdx
	}
	if termZeroIdx == -1 {
		// wasn't null terminated.  Assume it's still a valid string though
		lenString = len(u.data) - offset
	} else {
		lenString = termZeroIdx
		skip = 2
	}
	val = winutil.ConvertWindowsString(u.data[offset : offset+lenString])
	nextOffset = offset + lenString + skip
	valFound = true
	foundTermZeroIdx = termZeroIdx
	return
}

func (u *userInfo) GetUint64(offset int) uint64 {
	return binary.LittleEndian.Uint64(u.data[offset:])
}

func (u *userInfo) GetUint32(offset int) uint32 {
	return binary.LittleEndian.Uint32(u.data[offset:])
}

func (u *userInfo) GetUint16(offset int) uint16 {
	return binary.LittleEndian.Uint16(u.data[offset:])
}

func (u *userInfo) Bytes(offset int, length int) []byte {
	return u.data[offset : offset+length]
}

func (u *userInfo) Length() int {
	return len(u.data)
}

func bytesIndexOfDoubleZero(data []byte) int {
	dataLen := len(data)
	if dataLen < 2 {
		return -1
	}

	for i := 0; i < dataLen-1; i += 2 {
		if data[i] == 0 && data[i+1] == 0 {
			return i
		}
	}

	return -1
}
