// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package windowsevent

/*
#cgo LDFLAGS: -l wevtapi
#include "event.h"
*/
import "C"

import (
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

// Start starts tailing the event log.
func (t *Tailer) Start() {
	log.Infof("Starting windows event log tailing for channel %s query %s", t.config.ChannelPath, t.config.Query)
	go t.forwardMessages()
	t.decoder.Start()
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
	t.decoder.Stop()
	t.done <- struct{}{}
	return
}

func (t *Tailer) forwardMessages() {
	for decodedMessage := range t.decoder.OutputChan {
		if len(decodedMessage.GetContent()) > 0 {
			t.outputChan <- decodedMessage
		}
	}
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

	richEvt, err := EvtRender(handle)
	if err != nil {
		log.Warnf("Error rendering xml: %v", err)
		return
	}
	t, exists := tailerForIndex(goctx.id)
	if !exists {
		log.Warnf("Got invalid eventContext id %d when map is %v", goctx.id, eventContextToTailerMap)
		return
	}
	msg, err := t.toMessage(richEvt)
	if err != nil {
		log.Warnf("Couldn't convert xml to json: %s for event %s", err, richEvt.xmlEvent)
		return
	}

	t.source.RecordBytes(int64(len(msg.GetContent())))
	t.decoder.InputChan <- msg
}

var (
	modWinEvtAPI = windows.NewLazyDLL("wevtapi.dll")

	procEvtRender = modWinEvtAPI.NewProc("EvtRender")
)

// EvtRender takes an event handle and renders it to XML
func EvtRender(h C.ULONGLONG) (richEvt *richEvent, err error) {
	var bufSize uint32
	var bufUsed uint32

	_, _, err = procEvtRender.Call(uintptr(0), // this handle is always null for XML renders
		uintptr(h),                 // handle of event we're rendering
		uintptr(EvtRenderEventXml), // for now, always render in xml
		uintptr(bufSize),
		uintptr(0),                        // no buffer for now, just getting necessary size
		uintptr(unsafe.Pointer(&bufUsed)), // filled in with necessary buffer size
		uintptr(0))                        // not used but must be provided
	if err != error(windows.ERROR_INSUFFICIENT_BUFFER) {
		log.Warnf("Couldn't render xml event: %s", err)
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
	buf = buf[:bufUsed]
	// Call will set error anyway.  Clear it so we don't return an error
	err = nil

	xml := winutil.ConvertWindowsString(buf)

	richEvt = enrichEvent(h, xml)

	return

}

// enrichEvent renders data, and set the rendered fields to the richEvent.
// We need this some fields in the Windows Events are coded with numerical
// value. We then call a function in the Windows API that match the code to
// a human readable value.
// enrichEvent also takes care of freeing the memory allocated in the C code
func enrichEvent(h C.ULONGLONG, xml string) *richEvent {
	var message, task, opcode, level string
	// Enrich event with rendered
	richEvtCStruct := C.EnrichEvent(h)
	if richEvtCStruct != nil {
		if richEvtCStruct.message != nil {
			message = LPWSTRToString(richEvtCStruct.message)
		}
		if richEvtCStruct.task != nil {
			task = LPWSTRToString(richEvtCStruct.task)
		}
		if richEvtCStruct.opcode != nil {
			opcode = LPWSTRToString(richEvtCStruct.opcode)
		}
		if richEvtCStruct.level != nil {
			level = LPWSTRToString(richEvtCStruct.level)
		}

		C.free(unsafe.Pointer(richEvtCStruct.message))
		C.free(unsafe.Pointer(richEvtCStruct.task))
		C.free(unsafe.Pointer(richEvtCStruct.opcode))
		C.free(unsafe.Pointer(richEvtCStruct.level))
		C.free(unsafe.Pointer(richEvtCStruct))
	}

	if len(message) >= maxRunes {
		message = message + truncatedFlag
	}

	return &richEvent{
		xmlEvent: xml,
		message:  message,
		task:     task,
		opcode:   opcode,
		level:    level,
	}
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

	maxRunes      = 1<<17 - 1 // 128 kB
	truncatedFlag = "...TRUNCATED..."
)

type EVT_SUBSCRIBE_FLAGS int

const (
	_ = iota
	EvtSubscribeToFutureEvents
	EvtSubscribeStartAtOldestRecord
	EvtSubscribeStartAfterBookmark
)

// LPWSTRToString converts a C.LPWSTR to a string. It also truncates the
// strings to 128kB as a basic protection mechanism to avoid allocating an
// array too big. Messages with more than 128kB are likely to be bigger
// than 256kB when serialized and then dropped
func LPWSTRToString(cwstr C.LPWSTR) string {
	ptr := unsafe.Pointer(cwstr)
	sz := C.wcslen((*C.wchar_t)(ptr))
	sz = min(sz, maxRunes)
	wstr := (*[maxRunes]uint16)(ptr)[:sz:sz]
	return string(utf16.Decode(wstr))
}

func min(x, y C.size_t) C.size_t {
	if x > y {
		return y
	}
	return x
}
