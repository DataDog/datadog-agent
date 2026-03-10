// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metrics provides telemetry metrics for the logs agent
package metrics

import (
	"sync"
	"time"
)

// CapacityMonitor samples the average capacity of a component over a given interval.
// Capacity is calculated as the difference between the ingress and egress of a payload.
// Because data moves very quickly through components, we need to sample and aggregate this value over time.
// When channelCapacity > 0, the monitor also computes a fill percentage and records it to
// GlobalSaturationHistory for bottleneck detection and profile recommendation.
type CapacityMonitor struct {
	sync.Mutex
	ingress         int64
	ingressBytes    int64
	egress          int64
	egressBytes     int64
	avgItems        float64
	avgBytes        float64
	name            string
	instance        string
	channelCapacity int // 0 means capacity unknown; no fill % computed
	tickChan        <-chan time.Time
}

// NewCapacityMonitor creates a new CapacityMonitor. channelCapacity is the size of the
// upstream channel feeding this component; pass 0 if unknown.
func NewCapacityMonitor(name, instance string, channelCapacity int) *CapacityMonitor {
	return newCapacityMonitorWithTick(name, instance, channelCapacity, time.NewTicker(1*time.Second).C)
}

// newCapacityMonitorWithTick is used for testing.
func newCapacityMonitorWithTick(name, instance string, channelCapacity int, tickChan <-chan time.Time) *CapacityMonitor {
	return &CapacityMonitor{
		name:            name,
		instance:        instance,
		channelCapacity: channelCapacity,
		avgItems:        0,
		avgBytes:        0,
		tickChan:        tickChan,
	}
}

// AddIngress records the ingress of a payload
func (i *CapacityMonitor) AddIngress(pl MeasurablePayload) {
	if i == nil {
		return
	}
	i.Lock()
	defer i.Unlock()
	i.ingress += pl.Count()
	i.ingressBytes += pl.Size()
	i.sample()
}

// AddEgress records the egress of a payload
func (i *CapacityMonitor) AddEgress(pl MeasurablePayload) {
	if i == nil {
		return
	}
	i.Lock()
	defer i.Unlock()
	i.egress += pl.Count()
	i.egressBytes += pl.Size()
	i.sample()

}

func (i *CapacityMonitor) sample() {
	if i == nil {
		return
	}
	select {
	case <-i.tickChan:
		i.avgItems = ewma(float64(i.ingress-i.egress), i.avgItems)
		i.avgBytes = ewma(float64(i.ingressBytes-i.egressBytes), i.avgBytes)
		i.report()
	default:
	}
}

func ewma(newValue float64, oldValue float64) float64 {
	return newValue*ewmaAlpha + (oldValue * (1 - ewmaAlpha))
}

func (i *CapacityMonitor) report() {
	if i == nil {
		return
	}
	TlmUtilizationItems.Set(i.avgItems, i.name, i.instance)
	TlmUtilizationBytes.Set(i.avgBytes, i.name, i.instance)
	if i.channelCapacity > 0 {
		fillPct := i.avgItems / float64(i.channelCapacity)
		if fillPct < 0 {
			fillPct = 0
		} else if fillPct > 1.0 {
			fillPct = 1.0
		}
		TlmUtilizationFillPct.Set(fillPct*100, i.name, i.instance)
		GlobalSaturationHistory.RecordFill(i.name, fillPct)
	}
}
