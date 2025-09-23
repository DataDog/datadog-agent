// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package jmx allows to set and collect information about JMX check
package jmx

const (
	jmxFetchVersion    = "version"
	javaRuntimeVersion = "runtime_version"
)

type jmxCheckStatus struct {
	InitializedChecks map[string]interface{} `json:"initialized_checks"`
	FailedChecks      map[string]interface{} `json:"failed_checks"`
}

// Status holds status for JMX checks
type Status struct {
	Info         map[string]interface{} `json:"info"`
	ChecksStatus jmxCheckStatus         `json:"checks"`
	Timestamp    int64                  `json:"timestamp"`
	Errors       int64                  `json:"errors"`
}

// GetInfo returns JMXFetch version and Java runtime version
func (s *Status) GetInfo() (version, runtimeVersion string) {
	if v, ok := s.Info[jmxFetchVersion]; ok {
		if vStr, isString := v.(string); isString {
			version = vStr
		}
	}

	if rv, ok := s.Info[javaRuntimeVersion]; ok {
		if rvStr, isString := rv.(string); isString {
			runtimeVersion = rvStr
		}
	}

	return version, runtimeVersion
}

// StartupError holds startup status and errors
type StartupError struct {
	LastError string
	Timestamp int64
}

// GetStartupError retrieves latest JMX startup error
func GetStartupError() StartupError {
	lastJMXStartupErrorMutex.RLock()
	defer lastJMXStartupErrorMutex.RUnlock()
	errorCopy := StartupError{lastJMXStartupError.LastError, lastJMXStartupError.Timestamp}
	return errorCopy
}

// PopulateStatus populate stats with JMX information
func PopulateStatus(stats map[string]interface{}) {
	stats["JMXStatus"] = getJMXStatus()
	stats["JMXStartupError"] = GetStartupError()
}

func getJMXStatus() Status {
	lastJMXStatusMutex.RLock()
	defer lastJMXStatusMutex.RUnlock()

	return lastJMXStatus
}
