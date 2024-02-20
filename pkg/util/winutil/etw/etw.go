// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package etw

import (
	"sync"
	"syscall"
	"unsafe"
)

/*
#include "etw.h"

void etwCallbackC(DD_ETW_EVENT_INFO* eventInfo);
*/
import "C"

// Subscriber defines the interface for subscribing to ETW events
type Subscriber interface {
	OnStart()
	OnStop()
	OnEvent(*DDEtwEventInfo)
}

var (
	subscribers      = make(map[ProviderType]Subscriber)
	subscribersMutex sync.Mutex
)

// ProviderType identifies an ETW provider
type ProviderType uint64

// Supported ProviderTypes
const (
	EtwProviderHTTPService ProviderType = 1 << iota
)

// Etw trace flags
const (
	EtwTraceFlagDefault uint64 = 0
	EtwTraceFlagAsync   uint64 = 1 << iota
)

func providersToNativeProviders(etwProviders ProviderType) C.int64_t {
	var etwNativeProviders C.int64_t

	if (etwProviders & EtwProviderHTTPService) == EtwProviderHTTPService {
		etwNativeProviders |= C.DD_ETW_TRACE_PROVIDER_HttpService
	}

	return etwNativeProviders
}

func flagsToNativeFlags(etwFlags uint64) C.int64_t {
	if etwFlags == 0 {
		return 0
	}

	var etwNativeFlags C.int64_t

	if (etwFlags & EtwTraceFlagAsync) == EtwTraceFlagAsync {
		etwNativeFlags |= C.DD_ETW_TRACE_FLAG_ASYNC_EVENTS
	}

	return etwNativeFlags
}

func isHTTPServiceSubscriptionEnabled(etwProviders ProviderType) bool {
	return (etwProviders & EtwProviderHTTPService) == EtwProviderHTTPService
}

func addToSubscriber(etwProviders ProviderType, sub Subscriber) {
	subscribersMutex.Lock()
	defer subscribersMutex.Unlock()

	subscribers[etwProviders] = sub
}

func getSubscribers() []Subscriber {
	subscribersMutex.Lock()
	defer subscribersMutex.Unlock()

	var subs []Subscriber
	for _, s := range subscribers {
		subs = append(subs, s)
	}

	return subs
}

func deleteSubscribers() {
	subscribersMutex.Lock()
	defer subscribersMutex.Unlock()

	subscribers = make(map[ProviderType]Subscriber)
}

//export etwCallbackC
func etwCallbackC(eventInfo *C.DD_ETW_EVENT_INFO) {
	switch eventInfo.provider {
	case C.DD_ETW_TRACE_PROVIDER_HttpService:
		// This function needs to be as fast as possible because HTTP ETW providers send a
		// very high volume of events so we don't want to block the ETW thread. On the other
		// hand it is safe to access global map here because system invokes this function
		// but only if ETW session is not closed, which means that while a call to
		// C.StartEtwSubscription() from StartEtw() is in progress, this function is safe
		// to access global map and reversely when before and after a call to
		// C.StartEtwSubscription(), this function will not be invoked. It is also safe to
		// presume that this function will not be invoked after a call to
		// C.StopEtwSubscription() initiated from StopEtw() is completed.
		if sub, ok := subscribers[EtwProviderHTTPService]; ok {
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
func StartEtw(subscriptionName string, etwProviders ProviderType, sub Subscriber) error {

	if isHTTPServiceSubscriptionEnabled(etwProviders) {
		addToSubscriber(etwProviders, sub)
		sub.OnStart()
	}

	// This call is blocking and will not return until the ETW session is stopped
	// or an error occurs. This is by design. There is a race condition between
	// this function and StopEtw() which will unblock C.StartEtwSubscription().
	ret := C.StartEtwSubscription(
		C.CString(subscriptionName),
		providersToNativeProviders(etwProviders),
		flagsToNativeFlags(0),
		(C.ETW_EVENT_CALLBACK)(unsafe.Pointer(C.etwCallbackC)))

	if isHTTPServiceSubscriptionEnabled(etwProviders) {
		deleteSubscribers()
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
	subs := getSubscribers()
	if len(subs) != 0 {
		C.StopEtwSubscription()
		for _, s := range subs {
			s.OnStop()
		}
	}
}
