// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build windows

package windowsevent

/*
#cgo LDFLAGS: -l wevtapi
#include "event.h"
*/
import "C"

import (
	"syscall"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
)

// Start starts tailing the event log.
func (t *Tailer) Start() {
	log.Infof("Starting windows event log tailing for channel %s query %s", t.config.ChannelPath, t.config.Query)
	go t.tail()
}

// Stop stops the tailer
func (t *Tailer) Stop() {
	log.Info("Stop tailing windows event log")
	t.stop <- struct{}{}
	<-t.done
}

// tail subscribes to the channel for the windows events
func (t *Tailer) tail() {
	t.context = &eventContext{
		id: indexForTailer(t),
	}
	C.startEventSubscribe(
		C.CString(t.config.ChannelPath),
		C.CString(t.config.Query),
		C.ULONGLONG(0),
		C.int(EvtSubscribeToFutureEvents),
		C.PVOID(uintptr(unsafe.Pointer(t.context))),
	)
	t.source.Status.Success()

	// wait for stop signal
	<-t.stop
	t.done <- struct{}{}
	return
}

/*
	Windows related methods
*/

/* These are entry points for the callback to hand the pointer to Go-land.
   Note: handles are only valid within the callback. Don't pass them out. */

//export goStaleCallback
func goStaleCallback(errCode C.ULONGLONG, ctx C.PVOID) {
	log.Warn("EventLog tailer got Stale callback")
}

//export goErrorCallback
func goErrorCallback(errCode C.ULONGLONG, ctx C.PVOID) {
	log.Warn("EventLog tailer got Error callback with code ", errCode)
}

//export goNotificationCallback
func goNotificationCallback(handle C.ULONGLONG, ctx C.PVOID) {
	goctx := *(*eventContext)(unsafe.Pointer(uintptr(ctx)))
	log.Debug("Callback from ", goctx.id)

	xml, err := EvtRender(handle)
	if err != nil {
		log.Warn("Error rendering xml: %v", err)
		return
	}
	t, exists := tailerForIndex(goctx.id)
	if !exists {
		log.Warnf("Got invalid eventContext id %s when map is %s", goctx.id, eventContextToTailerMap)
		return
	}
	msg, err := t.toMessage(xml)
	if err != nil {
		log.Warnf("Couldn't convert xml to json: %s for event %s", err, xml)
		return
	}

	metrics.LogsCollected.Add(1)
	t.outputChan <- msg
}

var (
	modWinEvtAPI = syscall.NewLazyDLL("wevtapi.dll")

	procEvtSubscribe       = modWinEvtAPI.NewProc("EvtSubscribe")
	procEvtClose           = modWinEvtAPI.NewProc("EvtClose")
	procEvtRender          = modWinEvtAPI.NewProc("EvtRender")
	procEvtOpenChannelEnum = modWinEvtAPI.NewProc("EvtOpenChannelEnum")
	procEvtNextChannelPath = modWinEvtAPI.NewProc("EvtNextChannelPath")
	procEvtNext            = modWinEvtAPI.NewProc("EvtNext")
)

// EvtRender takes an event handle and reders it to XML
func EvtRender(h C.ULONGLONG) (xml string, err error) {
	var bufSize uint32
	var bufUsed uint32

	_, _, err = procEvtRender.Call(uintptr(0), // this handle is always null for XML renders
		uintptr(h),                 // handle of event we're rendering
		uintptr(EvtRenderEventXml), // for now, always render in xml
		uintptr(bufSize),
		uintptr(0),                        // no buffer for now, just getting necessary size
		uintptr(unsafe.Pointer(&bufUsed)), // filled in with necessary buffer size
		uintptr(0))                        // not used but must be provided
	if err != error(syscall.ERROR_INSUFFICIENT_BUFFER) {
		log.Warnf("Couldn't render xml event: ", err)
		return
	}
	bufSize = bufUsed
	buf := make([]uint8, bufSize)
	ret, _, err := procEvtRender.Call(uintptr(0), // this handle is always null for XML renders
		uintptr(h),                 // handle of event we're rendering
		uintptr(EvtRenderEventXml), // for now, always render in xml
		uintptr(bufSize),
		uintptr(unsafe.Pointer(&buf[0])),  // actual buffer used
		uintptr(unsafe.Pointer(&bufUsed)), // filled in with necessary buffer size
		uintptr(0))                        // not used but must be provided
	if ret == 0 {
		return
	}
	// Call will set error anyway.  Clear it so we don't return an error
	err = nil
	xml = ConvertWindowsString(buf)
	return

}

type evtSubscribeNotifyAction int32
type evtSubscribeFlags int32

const (
	EvtSubscribeActionError   evtSubscribeNotifyAction = 0
	EvtSubscribeActionDeliver evtSubscribeNotifyAction = 1

	EvtSubscribeOriginMask          evtSubscribeFlags = 0x3
	EvtSubscribeTolerateQueryErrors evtSubscribeFlags = 0x1000
	EvtSubscribeStrict              evtSubscribeFlags = 0x10000

	EvtRenderEventValues = 0 // Variants
	EvtRenderEventXml    = 1 // XML
	EvtRenderBookmark    = 2 // Bookmark

	ERROR_NO_MORE_ITEMS syscall.Errno = 259
)

type EVT_SUBSCRIBE_FLAGS int

const (
	_ = iota
	EvtSubscribeToFutureEvents
	EvtSubscribeStartAtOldestRecord
	EvtSubscribeStartAfterBookmark
)

// ConvertWindowsString converts a windows c-string
// into a go string.  Even though the input is array
// of uint8, the underlying data is expected to be
// uint16 (unicode)
func ConvertWindowsString(winput []uint8) string {
	var retstring string
	for i := 0; i < len(winput); i += 2 {
		dbyte := (uint16(winput[i+1]) << 8) + uint16(winput[i])
		if dbyte == 0 {
			break
		}
		retstring += string(rune(dbyte))
	}
	return retstring
}
