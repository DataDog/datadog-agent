// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"sync"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
)

// CgroupsMonitor defines a cgroup monitor
type CgroupsMonitor struct {
	sync.Mutex
	statsdClient statsd.ClientInterface
	ids          map[string]uint64 // pid having a container ID
}

// AddID add a new ID
func (cm *CgroupsMonitor) AddID(id string) {
	cm.Lock()
	if count, exists := cm.ids[id]; exists {
		cm.ids[id] = count + 1
	} else {
		cm.ids[id] = 1
	}
	cm.Unlock()
}

// AddID add a new ID
func (cm *CgroupsMonitor) DelID(id string) {
	cm.Lock()
	if count, exists := cm.ids[id]; exists {
		if count == 1 {
			delete(cm.ids, id)
		} else {
			cm.ids[id] = count - 1
		}
	}
	cm.Unlock()
}

// SendStats send stats
func (cm *CgroupsMonitor) SendStats() error {
	cm.Lock()
	defer cm.Unlock()

	count := len(cm.ids)
	_ = cm.statsdClient.Gauge(metrics.MetricRuntimeCgroupsRunning, float64(count), []string{}, 1.0)

	return nil
}

// NewCgroupsMonitor returns a new cgroups monitor
func NewCgroupsMonitor(statsdClient statsd.ClientInterface) *CgroupsMonitor {
	return &CgroupsMonitor{
		statsdClient: statsdClient,
		ids:          make(map[string]uint64),
	}
}
