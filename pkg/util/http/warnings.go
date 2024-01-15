// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// noProxyIgnoredWarningMap map containing URL's who will ignore the proxy in the future
	noProxyIgnoredWarningMap = make(map[string]bool)

	// noProxyUsedInFuture map containing URL's that will use a proxy in the future
	noProxyUsedInFuture = make(map[string]bool)

	// noProxyChanged map containing URL's whose proxy behavior will change in the future
	noProxyChanged = make(map[string]bool)

	// noProxyMapMutex Lock for all no proxy maps
	noProxyMapMutex = sync.Mutex{}
)

// GetNumberOfWarnings returns the total number of warnings
func GetNumberOfWarnings() int {
	noProxyMapMutex.Lock()
	defer noProxyMapMutex.Unlock()

	return len(noProxyIgnoredWarningMap) + len(noProxyUsedInFuture) + len(noProxyChanged)
}

// GetProxyIgnoredWarnings returns the list of URL which will ignore the proxy in the future
func GetProxyIgnoredWarnings() []string {
	noProxyMapMutex.Lock()
	defer noProxyMapMutex.Unlock()

	ignoredWarnings := []string{}
	for warn := range noProxyIgnoredWarningMap {
		ignoredWarnings = append(ignoredWarnings, warn)
	}
	return ignoredWarnings
}

// GetProxyUsedInFutureWarnings returns the list of URL which will use a proxy in the future
func GetProxyUsedInFutureWarnings() []string {
	noProxyMapMutex.Lock()
	defer noProxyMapMutex.Unlock()

	usedInFuture := []string{}
	for warn := range noProxyUsedInFuture {
		usedInFuture = append(usedInFuture, warn)
	}
	return usedInFuture
}

// GetProxyChangedWarnings returns the list of URL whose proxy behavior will change in the future
func GetProxyChangedWarnings() []string {
	noProxyMapMutex.Lock()
	defer noProxyMapMutex.Unlock()

	proxyChanged := []string{}
	for warn := range noProxyChanged {
		proxyChanged = append(proxyChanged, warn)
	}

	return proxyChanged
}

func warnOnce(warnMap map[string]bool, key string, format string, params ...interface{}) {
	noProxyMapMutex.Lock()
	defer noProxyMapMutex.Unlock()
	if _, ok := warnMap[key]; !ok {
		warnMap[key] = true
		log.Warnf(format, params...)
	}
}
