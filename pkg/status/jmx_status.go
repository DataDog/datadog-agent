// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

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

var (
	lastJMXStatus JMXStatus
	m             sync.RWMutex
)

// SetJMXStatus sets the last JMX Status
func SetJMXStatus(s JMXStatus) {
	m.Lock()
	defer m.Unlock()

	lastJMXStatus = s
}

// GetJMXStatus retrieves latest JMX Status
func GetJMXStatus() JMXStatus {
	m.RLock()
	defer m.RUnlock()

	return lastJMXStatus
}
