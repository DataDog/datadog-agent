// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux_bpf

package ebpf

import (
	manager "github.com/DataDog/ebpf-manager"
	"github.com/prometheus/client_golang/prometheus"
)

// NewPerfUsageCollector returns nil
func NewPerfUsageCollector() prometheus.Collector {
	return nil
}

// ReportPerfMapTelemetry starts reporting the telemetry for the provided PerfMap
func ReportPerfMapTelemetry(_ *manager.PerfMap) {}

// ReportRingBufferTelemetry starts reporting the telemetry for the provided RingBuffer
func ReportRingBufferTelemetry(_ *manager.RingBuffer) {}

// UnregisterTelemetry unregisters the PerfMap and RingBuffers from telemetry
func UnregisterTelemetry(_ *manager.Manager) {}
