// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package status

import (
	"sync"
)

// type instanceStatus struct {
// 	InstanceName string `json:"instance_name"`
// 	MCount       int    `json:"metric_count"`
// 	ScCount      int    `json:"service_check_count"`
// 	Message      string `json:"initialized_checks"`
// }

type jmxCheckStatus struct {
	InitializedChecks map[string]interface{} `json:"initialized_checks"`
	FailedChecks      map[string]interface{} `json:"failed_checks"`
}

type JMXStatus struct {
	ChecksStatus jmxCheckStatus `json:"checks"`
	Timestamp    int64          `json:"timestamp"`
}

var (
	lastJMXStatus JMXStatus
	m             sync.RWMutex
)

func SetJMXStatus(s JMXStatus) {
	m.Lock()
	defer m.Unlock()

	lastJMXStatus = s
}

func GetJMXStatus() JMXStatus {
	m.RLock()
	defer m.RUnlock()

	return lastJMXStatus
}
