// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(EBPF) Fix revive linter
package telemetry

import (
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

// StatCounterWrapper is a convenience type that allows for migrating telemetry to
// prometheus Counters while continuing to make the underlying values available for reading
type StatCounterWrapper struct {
	stat    *atomic.Int64
	counter telemetry.Counter
}

//nolint:revive // TODO(EBPF) Fix revive linter
func (sgw *StatCounterWrapper) Inc(tags ...string) {
	sgw.stat.Inc()
	sgw.counter.Inc(tags...)
}

//nolint:revive // TODO(EBPF) Fix revive linter
func (sgw *StatCounterWrapper) Delete() {
	sgw.stat.Store(0)
	sgw.counter.Delete()
}

//nolint:revive // TODO(EBPF) Fix revive linter
func (sgw *StatCounterWrapper) Add(v int64, tags ...string) {
	sgw.stat.Add(v)
	sgw.counter.Add(float64(v), tags...)
}

//nolint:revive // TODO(EBPF) Fix revive linter
func (sgw *StatCounterWrapper) Load() int64 {
	return sgw.stat.Load()
}

func NewStatCounterWrapper(subsystem string, statName string, tags []string, description string) *StatCounterWrapper {
	return &StatCounterWrapper{
		stat:    atomic.NewInt64(0),
		counter: telemetry.NewCounter(subsystem, statName, tags, description),
	}
}
