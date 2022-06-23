// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm
// +build windows,npm

package http

type noOpMonitor struct{}

// NewNoOpMonitor creates a monitor which always returns empty information
func NewNoOpMonitor() Monitor {
	return &noOpMonitor{}
}

func (*noOpMonitor) Start() {}

func (*noOpMonitor) GetHTTPStats() map[Key]*RequestStats {
	return nil
}

func (*noOpMonitor) GetStats() (map[string]int64, error) {
	return nil, nil
}

func (*noOpMonitor) Stop() error {
	return nil
}
