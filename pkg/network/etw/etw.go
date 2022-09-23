// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm
// +build windows,npm

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

var (
	curEtwProviders uint64 = 0
	curEtwFlags     uint64 = 0
)

const (
	EtwProviderHttpService uint64 = 1 << iota
)

const (
	EtwTraceFlagDefault uint64 = 0
	EtwTraceFlagAsync   uint64 = 1 << iota
)

func providersToNativeProviders(etwProviders uint64) C.int64_t {
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

func isHttpServiceSubscriptionEnabled(etwProviders uint64) bool {
	return (etwProviders & EtwProviderHttpService) == EtwProviderHttpService
}

//export etwCallbackC
func etwCallbackC(eventInfo *C.DD_ETW_EVENT_INFO) {
	switch eventInfo.provider {
	case C.DD_ETW_TRACE_PROVIDER_HttpService:
		etwHttpServiceCallback(eventInfo)
	}
}

func StartEtw(subscriptionName string, etwProviders uint64, etwFlags uint64) error {

	if isHttpServiceSubscriptionEnabled(etwProviders) {
		startEtwHttpServiceSubscription()
	}

	curEtwProviders = etwProviders
	curEtwFlags = etwFlags

	ret := C.StartEtwSubscription(
		C.CString(subscriptionName),
		providersToNativeProviders(etwProviders),
		flagsToNativeFlags(etwFlags),
		(C.ETW_EVENT_CALLBACK)(unsafe.Pointer(C.etwCallbackC)))

	if isHttpServiceSubscriptionEnabled(etwProviders) {
		stopEtwHttpServiceSubscription()
	}

	if ret != 0 {
		return syscall.Errno(ret)
	}

	return nil
}

func StopEtw(subscriptionName string) {
	if curEtwProviders != 0 {
		C.StopEtwSubscription()

		if isHttpServiceSubscriptionEnabled(curEtwProviders) {
			stopEtwHttpServiceSubscription()
		}

		curEtwProviders = 0
		curEtwFlags = 0
	}
}
