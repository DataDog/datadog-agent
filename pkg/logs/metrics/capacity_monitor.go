// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"sync"

	"github.com/VividCortex/ewma"
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
	avg          ewma.MovingAverage
	avgBytes     ewma.MovingAverage
	samples      float64
	name         string
	instance     string
}

// NewCapacityMonitor creates a new CapacityMonitor
func NewCapacityMonitor(name, instance string) *CapacityMonitor {
	return &CapacityMonitor{
		name:     name,
		instance: instance,
		avg:      ewma.NewMovingAverage(),
		avgBytes: ewma.NewMovingAverage(),
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
	i.avg.Add(float64(i.ingress - i.egress))
	i.avgBytes.Add(float64(i.ingressBytes - i.egressBytes))
	i.report()
}

func (i *CapacityMonitor) report() {
	TlmUtilizationItems.Set(float64(i.avg.Value()), i.name, i.instance)
	TlmUtilizationBytes.Set(float64(i.avgBytes.Value()), i.name, i.instance)
}
