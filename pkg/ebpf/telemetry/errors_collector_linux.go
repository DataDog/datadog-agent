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
}

// NewEBPFErrorsCollector initializes a new Collector object for ebpf helper and map operations errors
func NewEBPFErrorsCollector(bpfDir string) prometheus.Collector {
	var supported bool
	var err error

	// EBPFTelemetry needs to be initialized so we can patch the bytecode correctly
	initEBPFTelemetry(bpfDir)

	if supported, err = EBPFTelemetrySupported(); !supported || err != nil {
		return nil
	}
	return &EBPFErrorsCollector{
		T:                errorsTelemetry,
		ebpfMapOpsErrors: prometheus.NewDesc(fmt.Sprintf("%s__errors", ebpfMapTelemetryNS), "Failures of map operations for a specific ebpf map reported per error.", []string{"map_name", "error"}, nil),
		ebpfHelperErrors: prometheus.NewDesc(fmt.Sprintf("%s__errors", ebpfHelperTelemetryNS), "Failures of bpf helper operations reported per helper per error for each probe.", []string{"helper", "probe_name", "error"}, nil),
	}
}

// Describe returns all descriptions of the collector
func (e *EBPFErrorsCollector) Describe(ch chan<- *prometheus.Desc) {
	if e == nil {
		return
	}

	ch <- e.ebpfMapOpsErrors
	ch <- e.ebpfHelperErrors
}

// Collect returns the current state of all metrics of the collector
func (e *EBPFErrorsCollector) Collect(ch chan<- prometheus.Metric) {
	if e == nil {
		return
	}

	e.T.mtx.Lock()
	defer e.T.mtx.Unlock()

	if e.T.EBPFInstrumentationMap == nil {
		return
	}

	var val InstrumentationBlob
	var key uint32
	err := e.T.EBPFInstrumentationMap.Lookup(&key, &val)
	if err != nil {
		log.Warnf("failed to get instrumentation blob: %v", err)
		return
	}

	for mapName, mapIndx := range e.T.mapKeys {
		if count := getErrCount(val.Map_err_telemetry[mapIndx].Count[:]); len(count) > 0 {
			for errStr, errCount := range count {
				ch <- prometheus.MustNewConstMetric(e.ebpfMapOpsErrors, prometheus.GaugeValue, float64(errCount), mapName, errStr)
			}
		}
	}

	for programName, programIndex := range e.T.probeKeys {
		for index, helperName := range helperNames {
			base := maxErrno * index
			if count := getErrCount(val.Helper_err_telemetry[programIndex].Count[base : base+maxErrno]); len(count) > 0 {
				for errStr, errCount := range count {
					ch <- prometheus.MustNewConstMetric(e.ebpfHelperErrors, prometheus.GaugeValue, float64(errCount), helperName, programName, errStr)
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
