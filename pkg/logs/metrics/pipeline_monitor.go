// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"sync"
)

const (
	ewmaAlpha = 2 / (float64(30) + 1) // ~ 0.0645 for a 30s window

	// ProcessorTlmName is the telemetry name for processor components
	ProcessorTlmName = "processor"
	// StrategyTlmName is the telemetry name for strategy components
	StrategyTlmName = "strategy"
	// SenderTlmName is the telemetry name for sender components
	SenderTlmName = "sender"
	// WorkerTlmName is the telemetry name for worker components
	WorkerTlmName = "worker"
	// SenderTlmInstanceID is the default instance ID for sender components
	SenderTlmInstanceID = "0"
)

// MeasurablePayload represents a payload that can be measured in bytes and count
type MeasurablePayload interface {
	Size() int64
	Count() int64
}

// PipelineMonitor is an interface for monitoring the capacity of a pipeline.
// Pipeline monitors are used to measure both capacity and utilization of components.
type PipelineMonitor interface {
	GetCapacityMonitor(name string, instanceID string) *CapacityMonitor
	ReportComponentIngress(size MeasurablePayload, name string, instanceID string)
	ReportComponentEgress(size MeasurablePayload, name string, instanceID string)
	MakeUtilizationMonitor(name string, instanceID string) UtilizationMonitor
}

// NoopPipelineMonitor is a no-op implementation of PipelineMonitor.
// Some instances of logs components do not need to report capacity metrics and
// should use this implementation.
type NoopPipelineMonitor struct {
	instanceID string
}

// NewNoopPipelineMonitor creates a new no-op pipeline monitor
func NewNoopPipelineMonitor(instanceID string) *NoopPipelineMonitor {
	return &NoopPipelineMonitor{
		instanceID: instanceID,
	}
}

// GetCapacityMonitor returns the capacity monitor for the given name and instance ID.
// Returns nil for NoopPipelineMonitor as it doesn't track capacity.
func (n *NoopPipelineMonitor) GetCapacityMonitor(_ string, _ string) *CapacityMonitor {
	return nil
}

// ReportComponentIngress does nothing.
func (n *NoopPipelineMonitor) ReportComponentIngress(_ MeasurablePayload, _ string, _ string) {}

// ReportComponentEgress does nothing.
func (n *NoopPipelineMonitor) ReportComponentEgress(_ MeasurablePayload, _ string, _ string) {}

// MakeUtilizationMonitor returns a no-op utilization monitor.
func (n *NoopPipelineMonitor) MakeUtilizationMonitor(_ string, _ string) UtilizationMonitor {
	return &NoopUtilizationMonitor{}
}

// TelemetryPipelineMonitor is a PipelineMonitor that reports capacity metrics to telemetry
type TelemetryPipelineMonitor struct {
	monitors map[string]*CapacityMonitor
	lock     sync.RWMutex
}

// NewTelemetryPipelineMonitor creates a new pipeline monitort that reports capacity and utiilization metrics as telemetry
func NewTelemetryPipelineMonitor() *TelemetryPipelineMonitor {
	return &TelemetryPipelineMonitor{
		monitors: make(map[string]*CapacityMonitor),
		lock:     sync.RWMutex{},
	}
}

func (c *TelemetryPipelineMonitor) getMonitor(name string, instanceID string) *CapacityMonitor {
	key := name + instanceID

	c.lock.RLock()
	monitor, exists := c.monitors[key]
	c.lock.RUnlock()

	if !exists {
		c.lock.Lock()
		if c.monitors[key] == nil {
			c.monitors[key] = NewCapacityMonitor(name, instanceID)
		}
		monitor = c.monitors[key]
		c.lock.Unlock()
	}

	return monitor
}

// MakeUtilizationMonitor creates a new utilization monitor for a component.
func (c *TelemetryPipelineMonitor) MakeUtilizationMonitor(name string, instanceID string) UtilizationMonitor {
	return NewTelemetryUtilizationMonitor(name, instanceID)
}

// GetCapacityMonitor returns the capacity monitor for the given name and instance ID.
func (c *TelemetryPipelineMonitor) GetCapacityMonitor(name string, instanceID string) *CapacityMonitor {
	return c.getMonitor(name, instanceID)
}

// ReportComponentIngress reports the ingress of a payload to a component.
func (c *TelemetryPipelineMonitor) ReportComponentIngress(pl MeasurablePayload, name string, instanceID string) {
	c.getMonitor(name, instanceID).AddIngress(pl)
}

// ReportComponentEgress reports the egress of a payload from a component.
func (c *TelemetryPipelineMonitor) ReportComponentEgress(pl MeasurablePayload, name string, instanceID string) {
	c.getMonitor(name, instanceID).AddEgress(pl)
}
