// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package ssi

import "context"

// GetInstrumentationStatus contains the status of the APM auto-instrumentation.
func GetInstrumentationStatus() (status APMInstrumentationStatus, err error) {
	return status, nil // TBD on Windows
}

// IsAutoInstrumentationEnabled checks if the APM auto-instrumentation is enabled on the host.
func IsAutoInstrumentationEnabled(_ context.Context) (bool, error) {
	return false, nil // TBD on Windows
}
