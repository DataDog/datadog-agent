// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cshared

/*
#cgo CFLAGS: -I./include
#include <stdlib.h>
#include "sender.h"
*/
import "C"
import (
	"runtime"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/cshared/pinner"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var senderPinner runtime.Pinner

func newCSenderManager(senderManager sender.SenderManager) *C.sender_manager_t {
	senderManagerPtr := unsafe.Pointer(&senderManager)
	senderPinner.Pin(senderManagerPtr)
	pinner.Pin(senderPinner, senderManager)
	return C.new_sender_manager(senderManagerPtr)
}

//export _call_sender_manager_get_sender
func _call_sender_manager_get_sender(handle unsafe.Pointer, id *C.char, ret_sender **C.sender_t) *C.char {
	log.Debug("c-shared sender_manager get_sender")
	senderManager := *(*sender.SenderManager)(handle)
	sender, err := senderManager.GetSender(checkid.ID(C.GoString(id)))
	if err != nil {
		return C.CString(err.Error())
	}

	pinner.Pin(senderPinner, sender)
	senderPtr := unsafe.Pointer(&sender)
	pinner.Pin(senderPinner, senderPtr)
	cSender := C.new_sender(senderPtr)

	*ret_sender = cSender
	return nil
}
