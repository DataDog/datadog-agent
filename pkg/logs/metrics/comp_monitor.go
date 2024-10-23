// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package metrics

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

type Size interface {
	Size() int64
}

var TlmIngressBytes = telemetry.NewCounter("logs_component", "ingress_bytes", []string{"name", "instance"}, "")
var TlmEgressBytes = telemetry.NewCounter("logs_component", "egress_bytes", []string{"name", "instance"}, "")
var TlmUtilization = telemetry.NewGauge("logs_component", "utilization", []string{"name", "instance"}, "")

func ReportComponentIngress(size Size, name string, instance string) {
	TlmIngressBytes.Add(float64(size.Size()), name, instance)
}

func ReportComponentEgress(size Size, name string, instance string) {
	TlmEgressBytes.Add(float64(size.Size()), name, instance)
}

type UtilizationMonitor struct {
	inUse      float64
	idle       float64
	startIdle  time.Time
	startInUse time.Time
	name       string
	instance   string
}

func NewUtilizationMonitor(name, instance string) *UtilizationMonitor {
	return &UtilizationMonitor{
		startIdle:  time.Now(),
		startInUse: time.Now(),
		name:       name,
		instance:   instance,
	}
}

func (u *UtilizationMonitor) Start() {
	u.idle += float64(time.Since(u.startIdle) / time.Millisecond)
	u.startInUse = time.Now()
}

func (u *UtilizationMonitor) Stop() {
	u.inUse += float64(time.Since(u.startInUse) / time.Millisecond)
	u.startIdle = time.Now()
	TlmUtilization.Set(u.inUse/(u.idle+u.inUse), u.name, u.instance)
}
