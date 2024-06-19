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
func (e *EBPFTelemetry) GetHelpersTelemetry() map[string]interface{} {
	helperTelemMap := make(map[string]interface{})
	if e.helperErrMap == nil {
		return helperTelemMap
	}

	var val helperErrTelemetry
	for probeName, k := range e.probeKeys {
		err := e.helperErrMap.Lookup(&k, &val)
		if err != nil {
			log.Debugf("failed to get telemetry for map:key %s:%d\n", probeName, k)
			continue
		}

		t := make(map[string]interface{})
		for indx, helperName := range helperNames {
			base := maxErrno * indx
			if count := getErrCount(val.Count[base : base+maxErrno]); len(count) > 0 {
				t[helperName] = count
			}
		}
		if len(t) > 0 {
			helperTelemMap[probeName] = t
		}
	}
	return helperTelemMap
}
