// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package telemetry

import (
	"syscall"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sys/unix"
)

const (
	maxErrno    = 64
	maxErrnoStr = "other"
)

// A singleton instance of the ebpf telemetry struct. Used by the collector and the ebpf managers (via ErrorsTelemetryModifier).
var errorsTelemetry ebpfErrorsTelemetry

// EBPFErrorsCollector implements the prometheus Collector interface
// for collecting statistics about errors of ebpf helpers and ebpf maps operations.
type EBPFErrorsCollector struct {
	T            ebpfErrorsTelemetry
	mapOpsErrors *prometheus.CounterVec
	helperErrors *prometheus.CounterVec
	//ebpfMapOpsErrors *prometheus.Desc
	//ebpfHelperErrors *prometheus.Desc
	lastValues map[metricKey]uint64
}

type metricKey struct {
	hash uint64
	id   int
	err  string
}

// NewEBPFErrorsCollector initializes a new Collector object for ebpf helper and map operations errors
func NewEBPFErrorsCollector() prometheus.Collector {
	if supported, _ := ebpfTelemetrySupported(); !supported {
		return nil
	}

	return &EBPFErrorsCollector{
		T: newEBPFTelemetry(),
		mapOpsErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: "ebpf__maps",
				Name:      "_errors",
				Help:      "Failures of map operations for a specific ebpf map reported per error",
			},
			[]string{"map_name", "error"},
		),
		helperErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: "ebpf__helpers",
				Name:      "_errors",
				Help:      "Failures of bpf helper operations reported per helper per error for each probe",
			},
			[]string{"helper", "probe_name", "error"},
		),
		lastValues: make(map[metricKey]uint64),
	}
}

// Describe returns all descriptions of the collector
func (e *EBPFErrorsCollector) Describe(ch chan<- *prometheus.Desc) {
	e.mapOpsErrors.Describe(ch)
	e.helperErrors.Describe(ch)
}

// Collect returns the current state of all metrics of the collector
func (e *EBPFErrorsCollector) Collect(ch chan<- prometheus.Metric) {
	e.T.Lock()
	defer e.T.Unlock()

	if !e.T.isInitialized() {
		return // no telemetry to collect
	}

	e.T.forEachMapEntry(func(index telemetryIndex, val MapErrTelemetry) bool {
		if count := getErrCount(val.Count[:]); len(count) > 0 {
			for errStr, errCount := range count {
				key := metricKey{
					hash: index.key,
					id:   mapErr,
					err:  errStr,
				}
				delta := float64(errCount - e.lastValues[key])
				if delta > 0 {
					e.mapOpsErrors.WithLabelValues(index.name, errStr).Add(delta)
				}
				e.lastValues[key] = errCount
			}
		}
		return true
	})

	e.T.forEachHelperEntry(func(index telemetryIndex, val HelperErrTelemetry) bool {
		for i, helperName := range helperNames {
			base := maxErrno * i
			if count := getErrCount(val.Count[base : base+maxErrno]); len(count) > 0 {
				for errStr, errCount := range count {
					key := metricKey{
						hash: index.key,
						id:   i,
						err:  errStr,
					}
					delta := float64(errCount - e.lastValues[key])
					if delta > 0 {
						e.helperErrors.WithLabelValues(helperName, index.name, errStr).Add(delta)
					}
					e.lastValues[key] = errCount
				}
			}
		}
		return true
	})

	e.mapOpsErrors.Collect(ch)
	e.helperErrors.Collect(ch)
}

func getErrCount(v []uint64) map[string]uint64 {
	errCount := make(map[string]uint64)
	for i, count := range v {
		if count == 0 {
			continue
		}

		if (i + 1) == maxErrno {
			errCount[maxErrnoStr] = count
		} else if name := unix.ErrnoName(syscall.Errno(i)); name != "" {
			errCount[name] = count
		} else {
			errCount[syscall.Errno(i).Error()] = count
		}
	}
	return errCount
}
