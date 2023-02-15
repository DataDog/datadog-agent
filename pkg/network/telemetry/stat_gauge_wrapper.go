// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"go.uber.org/atomic"
)

type StatGaugeWrapper struct {
	stat  *atomic.Int64
	gauge telemetry.Gauge
}

func (sgw *StatGaugeWrapper) Inc() {
	sgw.stat.Inc()
	sgw.gauge.Inc()
}

func (sgw *StatGaugeWrapper) Dec() {
	sgw.stat.Dec()
	sgw.gauge.Dec()
}

func (sgw *StatGaugeWrapper) Add(v int64) {
	sgw.stat.Add(v)
	sgw.gauge.Add(float64(v))
}

func (sgw *StatGaugeWrapper) Set(v int64) {
	sgw.stat.Store(v)
	sgw.gauge.Set(float64(v))
}

func (sgw *StatGaugeWrapper) Load() int64 {
	return sgw.stat.Load()
}

func NewStatGaugeWrapper(subsystem string, statName string, tags []string, description string) StatGaugeWrapper {
	return StatGaugeWrapper{
		stat:  atomic.NewInt64(0),
		gauge: telemetry.NewGauge(subsystem, statName, tags, description),
	}
}
