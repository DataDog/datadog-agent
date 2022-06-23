// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm
// +build windows,npm

package etw

import (
	"unsafe"
)

/*
#cgo LDFLAGS: -L "./c/etw.c"
#include "./c/etw.h"

void etwCallbackC(DD_ETW_EVENT_INFO* eventInfo);
*/
import "C"

var (
	providers C.int64_t = C.DD_ETW_TRACE_PROVIDER_HttpService //
	etwFlags  C.int64_t = 0                                   // DD_ETW_TRACE_FLAG_ASYNC_EVENTS
)

//export etwCallbackC
func etwCallbackC(eventInfo *C.DD_ETW_EVENT_INFO) {
	switch eventInfo.provider {
	case C.DD_ETW_TRACE_PROVIDER_HttpService:
		etwHttpServiceCallback(eventInfo)
	}
}

func StartEtw(subscriptionName string) error {
	_, err := C.StartEtwSubscription(
		C.CString(subscriptionName), providers, etwFlags, (C.ETW_EVENT_CALLBACK)(unsafe.Pointer(C.etwCallbackC)))

	return err
}

func StopEtw(subscriptionName string) {
	C.StopEtwSubscription()
}
