// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package ssistatusimpl

// autoInstrumentationStatus checks if the APM auto-instrumentation is enabled on the host. This will return false on Kubernetes
func (c *ssiStatusComponent) autoInstrumentationStatus() (bool, []string, error) {
	return false, nil, nil // TBD on Windows
}
