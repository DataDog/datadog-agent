// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package windowsevent

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

/*
	Windows related methods
*/

var (
	modWinEvtAPI = windows.NewLazyDLL("wevtapi.dll")

	procEvtClose           = modWinEvtAPI.NewProc("EvtClose")
	procEvtOpenChannelEnum = modWinEvtAPI.NewProc("EvtOpenChannelEnum")
	procEvtNextChannelPath = modWinEvtAPI.NewProc("EvtNextChannelPath")
)

type evtEnumHandle uintptr

// EnumerateChannels enumerates available log channels
func EnumerateChannels() (chans []string, err error) {
	err = nil

	ret, _, err := procEvtOpenChannelEnum.Call(uintptr(0), // local computer
		uintptr(0)) // must be zero

	hEnum := evtEnumHandle(ret)
	if hEnum == evtEnumHandle(0) {
		return
	}
	defer procEvtClose.Call(uintptr(hEnum)) //nolint:errcheck

	for {
		var str string
		str, err = evtNextChannel(hEnum)
		if err == nil {
			chans = append(chans, str)
		} else if err == error(windows.ERROR_NO_MORE_ITEMS) {
			err = nil
			break
		} else {
			break
		}
	}
	fmt.Printf("returning error %v\n", err)
	return
}

func evtNextChannel(h evtEnumHandle) (ch string, err error) {

	var bufSize uint32
	var bufUsed uint32

	ret, _, err := procEvtNextChannelPath.Call(uintptr(h), // this handle is always null for XML renders
		uintptr(bufSize),
		uintptr(0),                        //no buffer for now, just getting necessary size
		uintptr(unsafe.Pointer(&bufUsed))) // filled in with necessary buffer size
	if err != error(windows.ERROR_INSUFFICIENT_BUFFER) {
		log.Warnf("Next: %v %v", ret, err)
		return
	}
	bufSize = bufUsed
	buf := make([]uint8, bufSize*2)
	ret, _, err = procEvtNextChannelPath.Call(uintptr(h), // handle of event we're rendering
		uintptr(bufSize),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&bufUsed))) // filled in with necessary buffer size
	if ret == 0 {
		log.Warnf("Next: %v %v", ret, err)
		return
	}
	err = nil
	// Call will set error anyway.  Clear it so we don't return an error

	// make sure size of buffer is set
	buf = buf[:(bufUsed * 2)]
	ch = winutil.ConvertWindowsString(buf)
	return
}
