// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf && test

package telemetry

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetHelpersTelemetry returns a map of error telemetry for each ebpf program
func (b *EBPFTelemetry) GetHelpersTelemetry() map[string]interface{} {
	helperTelemMap := make(map[string]interface{})
	if b.EBPFInstrumentationMap == nil {
		return helperTelemMap
	}

	var key uint32
	val := new(InstrumentationBlob)
	err := b.EBPFInstrumentationMap.Lookup(&key, val)
	if err != nil {
		log.Warnf("error looking up instrumentation blob: %v", err)
		return helperTelemMap
	}

	for programName, programIndex := range b.probeKeys {
		t := make(map[string]interface{})
		for index, helperName := range helperNames {
			base := maxErrno * index
			if count := getErrCount(val.Helper_err_telemetry[programIndex].Count[base : base+maxErrno]); len(count) > 0 {
				t[helperName] = count
			}
		}

		if len(t) > 0 {
			helperTelemMap[programName] = t
		}
	}

	return helperTelemMap
}

// GetMapsTelemetry returns a map of error telemetry for each ebpf map
func (b *EBPFTelemetry) GetMapsTelemetry() map[string]interface{} {
	t := make(map[string]interface{})
	if b.EBPFInstrumentationMap == nil {
		log.Warn("Instrumentation map is nil")
		return t
	}

	var key uint32
	val := new(InstrumentationBlob)
	err := b.EBPFInstrumentationMap.Lookup(&key, val)
	if err != nil {
		log.Warnf("failed to lookup instrumentation blob: %v", err)
		return t
	}

	for mapName, mapIndx := range b.mapKeys {
		if count := getErrCount(val.Map_err_telemetry[mapIndx].Count[:]); len(count) > 0 {
			t[mapName] = count
		}
	}

	return t
}
