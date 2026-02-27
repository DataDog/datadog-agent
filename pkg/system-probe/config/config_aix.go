// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build aix

package config

import "github.com/DataDog/datadog-agent/pkg/config/model"

// eBPFMapPreallocationSupported returns false on AIX (no eBPF support).
func eBPFMapPreallocationSupported() bool {
	return false
}

// ProcessEventDataStreamSupported returns false on AIX.
func ProcessEventDataStreamSupported() bool {
	return false
}

// RedisMonitoringSupported returns false on AIX.
func RedisMonitoringSupported() bool {
	return false
}

// HTTP2MonitoringSupported returns false on AIX.
func HTTP2MonitoringSupported() bool {
	return false
}

func allowPrebuiltEbpfFallback(_ model.Config) {
}
