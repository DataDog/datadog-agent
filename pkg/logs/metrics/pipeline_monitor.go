// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"sync"
	"time"
)

// MeasurablePayload represents a payload that can be measured in bytes and count
type MeasurablePayload interface {
	Size() int64
	Count() int64
}

// PipelineMonitor is an interface for monitoring the capacity of a pipeline.
// Pipeline monitors are used to measure both capacity and utilization of components.
type PipelineMonitor interface {
	ID() string
	ReportComponentIngress(size MeasurablePayload, name string)
	ReportComponentEgress(size MeasurablePayload, name string)
	MakeUtilizationMonitor(name string) UtilizationMonitor
}

// NoopPipelineMonitor is a no-op implementation of PipelineMonitor.
// Some instances of logs components do not need to report capacity metrics and
// should use this implementation.
type NoopPipelineMonitor struct {
	instanceID string
}

// NewNoopPipelineMonitor creates a new no-op pipeline monitor
func NewNoopPipelineMonitor(id string) *NoopPipelineMonitor {
	return &NoopPipelineMonitor{
		instanceID: id,
	}
}

// ID returns the instance id of the monitor
func (n *NoopPipelineMonitor) ID() string {
	return n.instanceID
}

// ReportComponentIngress does nothing.
func (n *NoopPipelineMonitor) ReportComponentIngress(_ MeasurablePayload, _ string) {}

// ReportComponentEgress does nothing.
func (n *NoopPipelineMonitor) ReportComponentEgress(_ MeasurablePayload, _ string) {}

// MakeUtilizationMonitor returns a no-op utilization monitor.
func (n *NoopPipelineMonitor) MakeUtilizationMonitor(_ string) UtilizationMonitor {
	return &NoopUtilizationMonitor{}
}

// TelemetryPipelineMonitor is a PipelineMonitor that reports capacity metrics to telemetry
type TelemetryPipelineMonitor struct {
	monitors   map[string]*CapacityMonitor
	interval   time.Duration
	instanceID string
	lock       sync.RWMutex
}

// NewTelemetryPipelineMonitor creates a new pipeline monitort that reports capacity and utiilization metrics as telemetry
func NewTelemetryPipelineMonitor(interval time.Duration, instanceID string) *TelemetryPipelineMonitor {
	return &TelemetryPipelineMonitor{
		monitors:   make(map[string]*CapacityMonitor),
		interval:   interval,
		instanceID: instanceID,
		lock:       sync.RWMutex{},
	}
}

func (c *TelemetryPipelineMonitor) getMonitor(name string) *CapacityMonitor {
	key := name + c.instanceID

	c.lock.RLock()
	monitor, exists := c.monitors[key]
	c.lock.RUnlock()

	if !exists {
		c.lock.Lock()
		if c.monitors[key] == nil {
			c.monitors[key] = NewCapacityMonitor(name, c.instanceID)
		}
		monitor = c.monitors[key]
		c.lock.Unlock()
	}

	return monitor
}

// ID returns the instance id of the monitor
func (c *TelemetryPipelineMonitor) ID() string {
	return c.instanceID
}

// MakeUtilizationMonitor creates a new utilization monitor for a component.
func (c *TelemetryPipelineMonitor) MakeUtilizationMonitor(name string) UtilizationMonitor {
	return NewTelemetryUtilizationMonitor(name, c.instanceID, c.interval)
}

// ReportComponentIngress reports the ingress of a payload to a component.
func (c *TelemetryPipelineMonitor) ReportComponentIngress(pl MeasurablePayload, name string) {
	c.getMonitor(name).AddIngress(pl)
}

// ReportComponentEgress reports the egress of a payload from a component.
func (c *TelemetryPipelineMonitor) ReportComponentEgress(pl MeasurablePayload, name string) {
	c.getMonitor(name).AddEgress(pl)
}
