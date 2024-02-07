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

type SafeSubscriberMap struct {
	mu          sync.RWMutex
	subscribers map[ProviderType]Subscriber
}

func (s *SafeSubscriberMap) Add(provider ProviderType, sub Subscriber) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscribers[provider] = sub
}

func (s *SafeSubscriberMap) Remove(provider ProviderType) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.subscribers, provider)
}

func (s *SafeSubscriberMap) Get(provider ProviderType) (Subscriber, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sub, ok := s.subscribers[provider]
	return sub, ok
}

func (s *SafeSubscriberMap) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.subscribers)
}

var (
	safeSubscribers = &SafeSubscriberMap{
		mu:          sync.Mutex{},
		subscribers: make(map[ProviderType]Subscriber),
	}
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

//export etwCallbackC
func etwCallbackC(eventInfo *C.DD_ETW_EVENT_INFO) {
	switch eventInfo.provider {
	case C.DD_ETW_TRACE_PROVIDER_HttpService:
		if sub, ok := safeSubscribers.Get(EtwProviderHTTPService); ok {
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
		safeSubscribers.Add(etwProviders, sub)
		sub.OnStart()
	}

	ret := C.StartEtwSubscription(
		C.CString(subscriptionName),
		providersToNativeProviders(etwProviders),
		flagsToNativeFlags(0),
		(C.ETW_EVENT_CALLBACK)(unsafe.Pointer(C.etwCallbackC)))

	if isHTTPServiceSubscriptionEnabled(etwProviders) {
		safeSubscribers.Remove(etwProviders)
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
	if safeSubscribers.Len() != 0 {
		C.StopEtwSubscription()

		if sub, ok := safeSubscribers.Get(EtwProviderHTTPService); ok {
			sub.OnStop()
		}

	}
}
