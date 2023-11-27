// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package jmx allows to set and collect information about JMX check
package jmx

import "sync"

var (
	lastJMXStatus            Status
	lastJMXStatusMutex       sync.RWMutex
	lastJMXStartupError      StartupError
	lastJMXStartupErrorMutex sync.RWMutex
)

// SetStatus sets the last JMX Status
func SetStatus(s Status) {
	lastJMXStatusMutex.Lock()
	defer lastJMXStatusMutex.Unlock()

	lastJMXStatus = s
}

// SetStartupError sets the last JMX startup error
func SetStartupError(s StartupError) {
	lastJMXStatusMutex.Lock()
	defer lastJMXStatusMutex.Unlock()

	lastJMXStartupError = s
}
