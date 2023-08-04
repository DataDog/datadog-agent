// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package etw

import (
	"syscall"
	"unsafe"
)

/*
#include "etw.h"

void etwCallbackC(DD_ETW_EVENT_INFO* eventInfo);
*/
import "C"

type Subscriber interface {
	OnStart()
	OnStop()
	OnEvent(*DDEtwEventInfo)
}

var (
	subscribers = make(map[EtwProviderType]Subscriber)
)

type EtwProviderType uint64

const (
	EtwProviderHttpService EtwProviderType = 1 << iota
)

const (
	EtwTraceFlagDefault uint64 = 0
	EtwTraceFlagAsync   uint64 = 1 << iota
)

func providersToNativeProviders(etwProviders EtwProviderType) C.int64_t {
	var etwNativeProviders C.int64_t = 0

	if (etwProviders & EtwProviderHttpService) == EtwProviderHttpService {
		etwNativeProviders |= C.DD_ETW_TRACE_PROVIDER_HttpService
	}

	return etwNativeProviders
}

func flagsToNativeFlags(etwFlags uint64) C.int64_t {
	if etwFlags == 0 {
		return 0
	}

	var etwNativeFlags C.int64_t = 0

	if (etwFlags & EtwTraceFlagAsync) == EtwTraceFlagAsync {
		etwNativeFlags |= C.DD_ETW_TRACE_FLAG_ASYNC_EVENTS
	}

	return etwNativeFlags
}

func isHttpServiceSubscriptionEnabled(etwProviders EtwProviderType) bool {
	return (etwProviders & EtwProviderHttpService) == EtwProviderHttpService
}

//export etwCallbackC
func etwCallbackC(eventInfo *C.DD_ETW_EVENT_INFO) {
	switch eventInfo.provider {
	case C.DD_ETW_TRACE_PROVIDER_HttpService:
		if sub, ok := subscribers[EtwProviderHttpService]; ok {
			sub.OnEvent((*DDEtwEventInfo)(unsafe.Pointer(eventInfo)))
		}
	}
}

// StartEtw starts the ETW service
//
// as currently constructed, it's still very much HTTP tracing only. Groundwork
// has been laid for adding additional tracing types (by using the map for callbacks, etc)
// but it's still HTTP centric.
//
// to add additional tracing will require ability to start, stop, add and remove specific
// tracing types
func StartEtw(subscriptionName string, etwProviders EtwProviderType, sub Subscriber) error {

	if isHttpServiceSubscriptionEnabled(etwProviders) {
		subscribers[etwProviders] = sub
		sub.OnStart()
	}

	ret := C.StartEtwSubscription(
		C.CString(subscriptionName),
		providersToNativeProviders(etwProviders),
		flagsToNativeFlags(0),
		(C.ETW_EVENT_CALLBACK)(unsafe.Pointer(C.etwCallbackC)))

	if isHttpServiceSubscriptionEnabled(etwProviders) {
		delete(subscribers, etwProviders)
		sub.OnStop()

	}

	if ret != 0 {
		return syscall.Errno(ret)
	}

	return nil
}

// StopEtw stops the tracing service
//
// See above note about http-centrism
func StopEtw(subscriptionName string) {
	if len(subscribers) != 0 {
		C.StopEtwSubscription()

		if sub, ok := subscribers[EtwProviderHttpService]; ok {
			sub.OnStop()
		}

	}
}
