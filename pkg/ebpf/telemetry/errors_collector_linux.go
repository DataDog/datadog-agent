// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package telemetry

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// EBPFErrorsCollector implements the prometheus Collector interface
// for collecting statistics about errors of ebpf helpers and ebpf maps operations.
type EBPFErrorsCollector struct {
	*EBPFTelemetry
	ebpfMapOpsErrorsGauge *prometheus.Desc
	ebpfHelperErrorsGauge *prometheus.Desc
}

// NewEBPFErrorsCollector initializes a new Collector object for ebpf helper and map operations errors
func NewEBPFErrorsCollector() prometheus.Collector {
	if supported, _ := ebpfTelemetrySupported(); !supported {
		return nil
	}
	return &EBPFErrorsCollector{
		EBPFTelemetry: &EBPFTelemetry{
			mapKeys:   make(map[string]uint64),
			probeKeys: make(map[string]uint64),
		},
		ebpfMapOpsErrorsGauge: prometheus.NewDesc(fmt.Sprintf("%s__errors", ebpfMapTelemetryNS), "Failures of map operations for a specific ebpf map reported per error.", []string{"map_name", "error"}, nil),
		ebpfHelperErrorsGauge: prometheus.NewDesc(fmt.Sprintf("%s__errors", ebpfHelperTelemetryNS), "Failures of bpf helper operations reported per helper per error for each probe.", []string{"helper", "probe_name", "error"}, nil),
	}
}

// Describe returns all descriptions of the collector
func (e *EBPFErrorsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.ebpfHelperErrorsGauge
	ch <- e.ebpfHelperErrorsGauge
}

// Collect returns the current state of all metrics of the collector
func (e *EBPFErrorsCollector) Collect(ch chan<- prometheus.Metric) {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	if e.helperErrMap != nil {
		var hval HelperErrTelemetry
		for probeName, k := range e.probeKeys {
			err := e.helperErrMap.Lookup(&k, &hval)
			if err != nil {
				log.Debugf("failed to get telemetry for probe:key %s:%d\n", probeName, k)
				continue
			}
			for indx, helperName := range helperNames {
				base := maxErrno * indx
				if count := getErrCount(hval.Count[base : base+maxErrno]); len(count) > 0 {
					for errStr, errCount := range count {
						ch <- prometheus.MustNewConstMetric(e.ebpfHelperErrorsGauge, prometheus.GaugeValue, float64(errCount), helperName, probeName, errStr)
					}
				}
			}
		}
	}

	if e.mapErrMap != nil {
		var val MapErrTelemetry
		for m, k := range e.mapKeys {
			err := e.mapErrMap.Lookup(&k, &val)
			if err != nil {
				log.Debugf("failed to get telemetry for map:key %s:%d\n", m, k)
				continue
			}
			if count := getErrCount(val.Count[:]); len(count) > 0 {
				for errStr, errCount := range count {
					ch <- prometheus.MustNewConstMetric(e.ebpfMapOpsErrorsGauge, prometheus.GaugeValue, float64(errCount), m, errStr)
				}
			}
		}
	}
}
