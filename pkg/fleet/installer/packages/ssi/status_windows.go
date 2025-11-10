// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package ssi

// GetInstrumentationStatus contains the status of the APM auto-instrumentation.
func GetInstrumentationStatus() (status APMInstrumentationStatus, err error) {
	return status, nil // TBD on Windows
}
