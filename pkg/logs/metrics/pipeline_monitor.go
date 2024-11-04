// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"sync"
	"time"
)

// MeasurablePayload representes a payload that can be measured in bytes and count
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
	instanceId string
}

func NewNoopPipelineMonitor(id string) *NoopPipelineMonitor {
	return &NoopPipelineMonitor{}
}

func (c *NoopPipelineMonitor) ID() string {
	return c.instanceId
}
func (n *NoopPipelineMonitor) ReportComponentIngress(size MeasurablePayload, name string) {}
func (n *NoopPipelineMonitor) ReportComponentEgress(size MeasurablePayload, name string)  {}
func (n *NoopPipelineMonitor) MakeUtilizationMonitor(name string) UtilizationMonitor {
	return &NoopUtilizationMonitor{}
}

// TelemetryPipelineMonitor is a PipelineMonitor that reports capacity metrics to telemetry
type TelemetryPipelineMonitor struct {
	monitors   map[string]*CapacityMonitor
	interval   time.Duration
	instanceId string
	lock       sync.RWMutex
}

// NewTelemetryPipelineMonitor creates a new pipeline monitort that reports capacity and utiilization metrics as telemetry
func NewTelemetryPipelineMonitor(interval time.Duration, instanceId string) *TelemetryPipelineMonitor {
	return &TelemetryPipelineMonitor{
		monitors:   make(map[string]*CapacityMonitor),
		interval:   interval,
		instanceId: instanceId,
		lock:       sync.RWMutex{},
	}
}

func (c *TelemetryPipelineMonitor) getMonitor(name string) *CapacityMonitor {
	key := name + c.instanceId
	c.lock.RLock()
	if c.monitors[key] == nil {
		c.lock.RUnlock()
		c.lock.Lock()
		c.monitors[key] = NewCapacityMonitor(name, c.instanceId, c.interval)
		c.lock.Unlock()
		c.lock.RLock()
	}
	defer c.lock.RUnlock()
	return c.monitors[key]
}

// ID returns the instance id of the monitor
func (c *TelemetryPipelineMonitor) ID() string {
	return c.instanceId
}

// MakeUtilizationMonitor creates a new utilization monitor for a component.
func (c *TelemetryPipelineMonitor) MakeUtilizationMonitor(name string) UtilizationMonitor {
	return NewTelemetryUtilizationMonitor(name, c.instanceId, c.interval)
}

// ReportComponentIngress reports the ingress of a payload to a component.
func (c *TelemetryPipelineMonitor) ReportComponentIngress(pl MeasurablePayload, name string) {
	c.getMonitor(name).AddIngress(pl)
}

// ReportComponentEgress reports the egress of a payload from a component.
func (c *TelemetryPipelineMonitor) ReportComponentEgress(pl MeasurablePayload, name string) {
	c.getMonitor(name).AddEgress(pl)
}
