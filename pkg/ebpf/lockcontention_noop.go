// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux_bpf

package ebpf

import (
	"github.com/prometheus/client_golang/prometheus"
)

// LockContentionCollector is just a placeholder
type LockContentionCollector struct{}

// NewLockContentionCollector returns nil
func NewLockContentionCollector() *LockContentionCollector {
	return nil
}

// Collect does nothing
func (l *LockContentionCollector) Collect(_ chan<- prometheus.Metric) {}

// Describe does nothing
func (l *LockContentionCollector) Describe(_ chan<- *prometheus.Desc) {}
