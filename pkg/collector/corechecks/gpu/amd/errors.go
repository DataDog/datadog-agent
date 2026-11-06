// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package amd

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
	amdgpu "github.com/DataDog/datadog-agent/pkg/gpu/amd"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// errorsMetricName counts the AMD GPU errors of the collection interval,
	// like errors.xid for NVIDIA GPUs.
	errorsMetricName = "errors.amdgpu"
	// errorsTotalMetricName counts the AMD GPU errors since the check started.
	errorsTotalMetricName = "errors.amdgpu.total"

	originDriver   = "driver"
	originHardware = "hardware"

	// kfdEventsWarningInterval limits the warnings about unavailable KFD events.
	kfdEventsWarningInterval = 10 * time.Minute
)

// errorType is the type tag of an AMD GPU error, with its origin tag.
type errorType struct {
	name   string
	origin string
}

var (
	errVMFault         = errorType{"vm_fault", originDriver}
	errGPUReset        = errorType{"gpu_reset", originDriver}
	errThermalThrottle = errorType{"thermal_throttle", originHardware}
	errRASUncorrected  = errorType{"ras_uncorrectable", originHardware}
	errRASCorrected    = errorType{"ras_correctable", originHardware}
	errRASDeferred     = errorType{"ras_deferred", originHardware}
	errRASFatal        = errorType{"ras_fatal", originHardware}
	errPoisonCreated   = errorType{"ras_poison_creation", originHardware}
	errPoisonConsumed  = errorType{"ras_poison_consumption", originHardware}
)

// eventErrorTypes maps the counted KFD events to error types. A GPU reset is
// counted once, on its pre-reset event.
var eventErrorTypes = map[amdgpu.EventType]errorType{
	amdgpu.EventVMFault:         errVMFault,
	amdgpu.EventGPUPreReset:     errGPUReset,
	amdgpu.EventThermalThrottle: errThermalThrottle,
}

// eventSource provides the KFD events of AMD GPUs; tests replace it.
type eventSource interface {
	Sync(devices []*amdgpu.Device) error
	Flush() ([]amdgpu.Event, int)
	Close()
}

// errorCounter turns KFD events and RAS counters into error counts per device.
type errorCounter struct {
	events      eventSource
	syncLimit   *log.Limit                      // syncLimit limits the warnings about unavailable KFD events
	interval    map[string]map[errorType]uint64 // interval holds the errors of the current collection, by device UUID
	accumulated map[string]map[errorType]uint64 // accumulated holds the errors since the check started, by device UUID
	previousRAS map[string]amdgpu.RASCounters   // previousRAS holds the last RAS counters read, by device UUID
}

func newErrorCounter(events eventSource) *errorCounter {
	return &errorCounter{
		events:      events,
		syncLimit:   log.NewLogLimit(1, kfdEventsWarningInterval),
		interval:    make(map[string]map[errorType]uint64),
		accumulated: make(map[string]map[errorType]uint64),
		previousRAS: make(map[string]amdgpu.RASCounters),
	}
}

// startCollection subscribes to the events of the current devices, forgets
// removed devices, and counts the events received since the last collection.
func (e *errorCounter) startCollection(devices []*amdgpu.Device) {
	present := make(map[string]struct{}, len(devices))
	for _, dev := range devices {
		present[dev.UUID] = struct{}{}
	}
	for uuid := range e.accumulated {
		if _, ok := present[uuid]; !ok {
			delete(e.accumulated, uuid)
		}
	}
	for uuid := range e.previousRAS {
		if _, ok := present[uuid]; !ok {
			delete(e.previousRAS, uuid)
		}
	}
	clear(e.interval)

	if e.events == nil {
		return
	}
	if err := e.events.Sync(devices); err != nil && e.syncLimit.ShouldLog() {
		log.Warnf("AMD GPU error events (VM faults, GPU resets, thermal throttling) are unavailable: %v. "+
			"The Agent needs read/write access to /dev/kfd: in Kubernetes, use the AMD container runtime with AMD_VISIBLE_DEVICES; on a host, add the dd-agent user to the render group", err)
	}
	events, dropped := e.events.Flush()
	if dropped > 0 {
		log.Debugf("dropped %d AMD GPU events received between two collections", dropped)
	}
	for _, evt := range events {
		if _, ok := present[evt.DeviceUUID]; !ok {
			continue
		}
		if t, ok := eventErrorTypes[evt.Type]; ok {
			e.add(evt.DeviceUUID, t, 1)
		}
	}
}

// addRAS counts the increase of the RAS counters of a device since the last
// reading. The first reading, and a reading lower than the last one (the
// driver was reloaded), only set the baseline.
func (e *errorCounter) addRAS(deviceUUID string, current amdgpu.RASCounters) {
	previous, seen := e.previousRAS[deviceUUID]
	merged := previous
	for _, c := range []struct {
		t        errorType
		cur      *uint64
		prev     *uint64
		baseline **uint64
	}{
		{errRASUncorrected, current.Uncorrectable, previous.Uncorrectable, &merged.Uncorrectable},
		{errRASCorrected, current.Correctable, previous.Correctable, &merged.Correctable},
		{errRASDeferred, current.Deferred, previous.Deferred, &merged.Deferred},
		{errRASFatal, current.Fatal, previous.Fatal, &merged.Fatal},
		{errPoisonCreated, current.PoisonCreated, previous.PoisonCreated, &merged.PoisonCreated},
		{errPoisonConsumed, current.PoisonConsumed, previous.PoisonConsumed, &merged.PoisonConsumed},
	} {
		if c.cur == nil {
			continue // not read this time: keep the last value
		}
		if seen && c.prev != nil && *c.cur > *c.prev {
			e.add(deviceUUID, c.t, *c.cur-*c.prev)
		}
		*c.baseline = c.cur
	}
	e.previousRAS[deviceUUID] = merged
}

func (e *errorCounter) add(deviceUUID string, t errorType, n uint64) {
	if e.interval[deviceUUID] == nil {
		e.interval[deviceUUID] = make(map[errorType]uint64)
	}
	if e.accumulated[deviceUUID] == nil {
		e.accumulated[deviceUUID] = make(map[errorType]uint64)
	}
	e.interval[deviceUUID][t] += n
	e.accumulated[deviceUUID][t] += n
}

// samples returns the error metrics of a device: for every error type seen
// since the check started, the count of the interval (possibly 0) and the
// total, as for NVIDIA XID errors.
func (e *errorCounter) samples(deviceUUID string) []nvidia.Sample {
	var samples []nvidia.Sample
	for t, total := range e.accumulated[deviceUUID] {
		tags := []string{"type:" + t.name, "origin:" + t.origin}
		samples = append(samples,
			nvidia.NewMetric(errorsMetricName, float64(e.interval[deviceUUID][t]), metrics.CountType, nvidia.Low, tags, nil),
			nvidia.NewMetric(errorsTotalMetricName, float64(total), metrics.GaugeType, nvidia.Low, tags, nil),
		)
	}
	return samples
}

func (e *errorCounter) close() {
	if e.events != nil {
		e.events.Close()
	}
}
