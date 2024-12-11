// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package cgroups holds cgroups related files
package cgroups

import (
	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
)

// Monitor defines a cgroup monitor
type Monitor struct {
	statsdClient    statsd.ClientInterface
	cgroupsResolver *cgroup.Resolver
}

// SendStats send stats
func (cm *Monitor) SendStats() error {
	count := cm.cgroupsResolver.Len()
	_ = cm.statsdClient.Gauge(metrics.MetricRuntimeCgroupsRunning, float64(count), []string{}, 1.0)
	return nil
}

// NewCgroupsMonitor returns a new cgroups monitor
func NewCgroupsMonitor(statsdClient statsd.ClientInterface, cgrouspResolver *cgroup.Resolver) *Monitor {
	return &Monitor{
		statsdClient:    statsdClient,
		cgroupsResolver: cgrouspResolver,
	}
}
