// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package status

import (
	"sync"
)

type jmxCheckStatus struct {
	InitializedChecks map[string]interface{} `json:"initialized_checks"`
	FailedChecks      map[string]interface{} `json:"failed_checks"`
}

// JMXStatus holds status for JMX checks
type JMXStatus struct {
	ChecksStatus jmxCheckStatus `json:"checks"`
	Timestamp    int64          `json:"timestamp"`
}

// JMXStartupError holds startup status and errors
type JMXStartupError struct {
	LastError string
	Timestamp int64
}

var (
	lastJMXStatus            JMXStatus
	lastJMXStatusMutex       sync.RWMutex
	lastJMXStartupError      JMXStartupError
	lastJMXStartupErrorMutex sync.RWMutex
)

// SetJMXStatus sets the last JMX Status
func SetJMXStatus(s JMXStatus) {
	lastJMXStatusMutex.Lock()
	defer lastJMXStatusMutex.Unlock()

	lastJMXStatus = s
}

// GetJMXStatus retrieves latest JMX Status
func GetJMXStatus() JMXStatus {
	lastJMXStatusMutex.RLock()
	defer lastJMXStatusMutex.RUnlock()

	return lastJMXStatus
}

// SetJMXStartupError sets the last JMX startup error
func SetJMXStartupError(s JMXStartupError) {
	lastJMXStatusMutex.Lock()
	defer lastJMXStatusMutex.Unlock()

	lastJMXStartupError = s
}

// GetJMXStartupError retrieves latest JMX startup error
func GetJMXStartupError() JMXStartupError {
	lastJMXStartupErrorMutex.RLock()
	defer lastJMXStartupErrorMutex.RUnlock()
	copy := JMXStartupError{lastJMXStartupError.LastError, lastJMXStartupError.Timestamp}
	return copy
}
