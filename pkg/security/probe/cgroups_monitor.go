// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probe

import (
	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
)

// CgroupsMonitor defines a cgroup monitor
type CgroupsMonitor struct {
	statsdClient    statsd.ClientInterface
	cgroupsResolver *cgroup.Resolver
}

// SendStats send stats
func (cm *CgroupsMonitor) SendStats() error {
	count := cm.cgroupsResolver.Len()
	_ = cm.statsdClient.Gauge(metrics.MetricRuntimeCgroupsRunning, float64(count), []string{}, 1.0)
	return nil
}

// NewCgroupsMonitor returns a new cgroups monitor
func NewCgroupsMonitor(statsdClient statsd.ClientInterface, cgrouspResolver *cgroup.Resolver) *CgroupsMonitor {
	return &CgroupsMonitor{
		statsdClient:    statsdClient,
		cgroupsResolver: cgrouspResolver,
	}
}
