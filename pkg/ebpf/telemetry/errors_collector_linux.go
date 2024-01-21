// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package telemetry

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/prometheus/client_golang/prometheus"
	"unsafe"
)

type EbpfErrorsCollector struct {
	EBPFTelemetry
}

// NewEbpfErrorsCollector initializes a new Collector object for ebpf helper and map operations errors
func NewEbpfErrorsCollector() prometheus.Collector {
	if supported, _ := ebpfTelemetrySupported(); !supported {
		log.Debug("ebpf errors collector not supported")
		return &NoopEbpfErrorsCollector{}
	}
	return &EbpfErrorsCollector{
		EBPFTelemetry{
			mapKeys:   make(map[string]uint64),
			probeKeys: make(map[string]uint64),
		},
	}
}

// Describe returns all descriptions of the collector
func (e *EbpfErrorsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- ebpfMapOpsErrorsGauge
	ch <- ebpfHelperErrorsGauge
}

// Collect returns the current state of all metrics of the collector
func (e *EbpfErrorsCollector) Collect(ch chan<- prometheus.Metric) {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	if e.helperErrMap != nil {
		var hval HelperErrTelemetry
		for probeName, k := range e.probeKeys {
			err := e.helperErrMap.Lookup(unsafe.Pointer(&k), unsafe.Pointer(&hval))
			if err != nil {
				log.Debugf("failed to get telemetry for probe:key %s:%d\n", probeName, k)
				continue
			}
			for indx, helperName := range helperNames {
				base := maxErrno * indx
				if count := getErrCount(hval.Count[base : base+maxErrno]); len(count) > 0 {
					for errStr, errCount := range count {
						ch <- prometheus.MustNewConstMetric(ebpfHelperErrorsGauge, prometheus.GaugeValue, float64(errCount), helperName, probeName, errStr)
					}
				}
			}
		}
	}

	if e.mapErrMap != nil {
		var val MapErrTelemetry
		for m, k := range e.mapKeys {
			err := e.mapErrMap.Lookup(unsafe.Pointer(&k), unsafe.Pointer(&val))
			if err != nil {
				log.Debugf("failed to get telemetry for map:key %s:%d\n", m, k)
				continue
			}
			if count := getErrCount(val.Count[:]); len(count) > 0 {
				for errStr, errCount := range count {
					ch <- prometheus.MustNewConstMetric(ebpfMapOpsErrorsGauge, prometheus.GaugeValue, float64(errCount), m, errStr)
				}
			}
		}
	}
}
