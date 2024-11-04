// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"sync"
	"time"
)

// CapacityMonitor samples the average capacity of a component over a given interval.
// Capacity is calculated as the difference between the ingress and egress of a payload.
// Because data moves very quickly through components, we need to sample and aggregate this value over time.
type CapacityMonitor struct {
	sync.Mutex
	ingress      int64
	ingressBytes int64
	egress       int64
	egressBytes  int64
	avg          float64
	avgBytes     float64
	samples      float64
	name         string
	instance     string
	ticker       *time.Ticker
}

// NewCapacityMonitor creates a new CapacityMonitor
func NewCapacityMonitor(name, instance string, interval time.Duration) *CapacityMonitor {
	return &CapacityMonitor{
		name:     name,
		instance: instance,
		ticker:   time.NewTicker(interval),
	}
}

// AddIngress records the ingress of a payload
func (i *CapacityMonitor) AddIngress(pl MeasurablePayload) {
	i.Lock()
	defer i.Unlock()
	i.ingress += pl.Count()
	i.ingressBytes += pl.Size()
	i.sample()
}

// AddEgress records the egress of a payload
func (i *CapacityMonitor) AddEgress(pl MeasurablePayload) {
	i.Lock()
	defer i.Unlock()
	i.egress += pl.Count()
	i.egressBytes += pl.Size()
	i.sample()

}

func (i *CapacityMonitor) sample() {
	i.samples++
	i.avg = (i.avg*(i.samples-1) + float64(i.ingress-i.egress)) / i.samples
	i.avgBytes = (i.avgBytes*(i.samples-1) + float64(i.ingressBytes-i.egressBytes)) / i.samples
	i.reportIfNeeded()
}

func (i *CapacityMonitor) reportIfNeeded() {
	select {
	case <-i.ticker.C:
		TlmCapacity.Set(float64(i.avg), i.name, i.instance)
		TlmCapacityBytes.Set(float64(i.avgBytes), i.name, i.instance)
		i.avg = 0
		i.avgBytes = 0
		i.samples = 0
	default:
	}
}
