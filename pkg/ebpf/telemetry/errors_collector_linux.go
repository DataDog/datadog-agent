// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package telemetry

import (
	"fmt"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	maxErrno    = 64
	maxErrnoStr = "other"

	ebpfMapTelemetryNS    = "ebpf_maps"
	ebpfHelperTelemetryNS = "ebpf_helpers"
)

// EBPFErrorsCollector implements the prometheus Collector interface
// for collecting statistics about errors of ebpf helpers and ebpf maps operations.
type EBPFErrorsCollector struct {
	T                *EBPFTelemetry
	ebpfMapOpsErrors *prometheus.Desc
	ebpfHelperErrors *prometheus.Desc
	//we can use one map for both map errors and ebpf helpers errors, as the keys are different
	lastValues map[string]uint64
}

// NewEBPFErrorsCollector initializes a new Collector object for ebpf helper and map operations errors
func NewEBPFErrorsCollector() prometheus.Collector {
	if supported, _ := ebpfTelemetrySupported(); !supported {
		return nil
	}
	return &EBPFErrorsCollector{
		T:                newEBPFTelemetry(),
		ebpfMapOpsErrors: prometheus.NewDesc(fmt.Sprintf("%s__errors", ebpfMapTelemetryNS), "Failures of map operations for a specific ebpf map reported per error.", []string{"map_name", "error"}, nil),
		ebpfHelperErrors: prometheus.NewDesc(fmt.Sprintf("%s__errors", ebpfHelperTelemetryNS), "Failures of bpf helper operations reported per helper per error for each probe.", []string{"helper", "probe_name", "error"}, nil),
		lastValues:       make(map[string]uint64),
	}
}

// Describe returns all descriptions of the collector
func (e *EBPFErrorsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.ebpfMapOpsErrors
	ch <- e.ebpfHelperErrors
}

// Collect returns the current state of all metrics of the collector
func (e *EBPFErrorsCollector) Collect(ch chan<- prometheus.Metric) {
	e.T.mtx.Lock()
	defer e.T.mtx.Unlock()

	if e.T.helperErrMap != nil {
		var hval HelperErrTelemetry
		for probeName, k := range e.T.probeKeys {
			err := e.T.helperErrMap.Lookup(&k, &hval)
			if err != nil {
				log.Debugf("failed to get telemetry for probe:key %s:%d\n", probeName, k)
				continue
			}
			for index, helperName := range helperNames {
				base := maxErrno * index
				if count := getErrCount(hval.Count[base : base+maxErrno]); len(count) > 0 {
					for errStr, errCount := range count {
						errorsDelta := float64(errCount - e.lastValues[errStr])
						if errorsDelta > 0 {
							ch <- prometheus.MustNewConstMetric(e.ebpfHelperErrors, prometheus.CounterValue, errorsDelta, helperName, probeName, errStr)
						}
						e.lastValues[errStr] = errCount
					}
				}
			}
		}
	}

	if e.T.mapErrMap != nil {
		var val MapErrTelemetry
		for m, k := range e.T.mapKeys {
			err := e.T.mapErrMap.Lookup(&k, &val)
			if err != nil {
				log.Debugf("failed to get telemetry for map:key %s:%d\n", m, k)
				continue
			}
			if count := getErrCount(val.Count[:]); len(count) > 0 {
				for errStr, errCount := range count {
					errorsDelta := float64(errCount - e.lastValues[errStr])
					if errorsDelta > 0 {
						ch <- prometheus.MustNewConstMetric(e.ebpfMapOpsErrors, prometheus.CounterValue, errorsDelta, m, errStr)
					}
					e.lastValues[errStr] = errCount
				}
			}
		}
	}
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
