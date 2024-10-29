// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package metrics

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

type Size interface {
	Size() int64
	Count() int64
}

var TlmUtilization = telemetry.NewGauge("logs_component", "utilization", []string{"name", "instance"}, "")
var TlmCapacity = telemetry.NewGauge("logs_component", "capacity", []string{"name", "instance"}, "")
var TlmCapacityBytes = telemetry.NewGauge("logs_component", "capacity_bytes", []string{"name", "instance"}, "")

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

func (i *CapacityMonitor) AddIngress(size Size) {
	i.Lock()
	defer i.Unlock()
	i.ingress += size.Count()
	i.ingressBytes += size.Size()
	i.sample()
}

func (i *CapacityMonitor) AddEgress(size Size) {
	i.Lock()
	defer i.Unlock()
	i.egress += size.Count()
	i.egressBytes += size.Size()
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

type PipelineMonitor interface {
	ID() string
	ReportComponentIngress(size Size, name string)
	ReportComponentEgress(size Size, name string)
	MakeUtilizationMonitor(name string) UtilizationMonitor
}

type TelemetryPipelineMonitor struct {
	monitors   map[string]*CapacityMonitor
	interval   time.Duration
	instanceId string
	lock       sync.RWMutex
}

func NewTelemetryPipelineMonitor(interval time.Duration, instanceId string) *TelemetryPipelineMonitor {
	return &TelemetryPipelineMonitor{monitors: make(map[string]*CapacityMonitor)}
}

func (c *TelemetryPipelineMonitor) getMonitor(name string) *CapacityMonitor {
	key := name + c.instanceId
	c.lock.RLock()
	if c.monitors[key] == nil {
		c.lock.RUnlock()
		c.lock.Lock()
		c.monitors[key] = &CapacityMonitor{name: name, instance: c.instanceId, ticker: time.NewTicker(c.interval)}
		c.lock.Unlock()
		c.lock.RLock()
	}
	defer c.lock.RUnlock()
	return c.monitors[key]
}

func (c *TelemetryPipelineMonitor) ID() string {
	return c.instanceId
}

func (c *TelemetryPipelineMonitor) MakeUtilizationMonitor(name string) UtilizationMonitor {
	return NewTelemetryUtilizationMonitor(name, c.instanceId, c.interval)
}

func (c *TelemetryPipelineMonitor) ReportComponentIngress(size Size, name string) {
	m := c.getMonitor(name)
	m.AddIngress(size)
}

func (c *TelemetryPipelineMonitor) ReportComponentEgress(size Size, name string) {
	m := c.getMonitor(name)
	m.AddEgress(size)
}

type NoopPipelineMonitor struct {
	instanceId string
}

func NewNoopPipelineMonitor(id string) *NoopPipelineMonitor {
	return &NoopPipelineMonitor{}
}

func (c *NoopPipelineMonitor) ID() string {
	return c.instanceId
}
func (n *NoopPipelineMonitor) ReportComponentIngress(size Size, name string) {}
func (n *NoopPipelineMonitor) ReportComponentEgress(size Size, name string)  {}
func (n *NoopPipelineMonitor) MakeUtilizationMonitor(name string) UtilizationMonitor {
	return &NoopUtilizationMonitor{}
}

type UtilizationMonitor interface {
	Start()
	Stop()
}

type NoopUtilizationMonitor struct{}

func (n *NoopUtilizationMonitor) Start() {}
func (n *NoopUtilizationMonitor) Stop()  {}

type TelemetryUtilizationMonitor struct {
	sync.Mutex
	inUse      float64
	idle       float64
	startIdle  time.Time
	startInUse time.Time
	name       string
	instance   string
	ticker     *time.Ticker
}

func NewTelemetryUtilizationMonitor(name, instance string, interval time.Duration) *TelemetryUtilizationMonitor {
	return &TelemetryUtilizationMonitor{
		startIdle:  time.Now(),
		startInUse: time.Now(),
		name:       name,
		instance:   instance,
		ticker:     time.NewTicker(interval),
	}
}

func (u *TelemetryUtilizationMonitor) Start() {
	u.Lock()
	defer u.Unlock()
	u.idle += float64(time.Since(u.startIdle) / time.Millisecond)
	u.startInUse = time.Now()
}

func (u *TelemetryUtilizationMonitor) Stop() {
	u.Lock()
	defer u.Unlock()
	u.inUse += float64(time.Since(u.startInUse) / time.Millisecond)
	u.startIdle = time.Now()
	select {
	case <-u.ticker.C:
		TlmUtilization.Set(u.inUse/(u.idle+u.inUse), u.name, u.instance)
		u.idle = 0
		u.inUse = 0
	default:
	}

}
