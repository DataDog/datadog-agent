// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package pdhutil

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// PdhFormatter implements a formatter for PDH performance counters
type PdhFormatter struct {
}

// PdhCounterValue represents a counter value
type PdhCounterValue struct {
	CStatus uint32
	Double  float64
	Large   int64
	Long    int32
}

// PdhCounterValueItem contains the counter value for an instance
type PdhCounterValueItem struct {
	instance string
	value    PdhCounterValue
}

// ValueEnumFunc implements a callback for counter enumeration
type ValueEnumFunc func(s string, v PdhCounterValue)

// Enum enumerates performance counter values for a wildcard instance counter (e.g. `\Process(*)\% Processor Time`)
func (f *PdhFormatter) Enum(counterName string, hCounter PDH_HCOUNTER, format uint32, ignoreInstances []string, fn ValueEnumFunc) error {
	items, err := pdhGetFormattedCounterArray(hCounter, format)
	if err != nil {
		return err
	}
	for _, item := range items {
		skip := false
		for _, ignored := range ignoreInstances {
			if item.instance == ignored {
				skip = true
			}
		}
		if skip {
			continue
		}

		if item.value.CStatus != PDH_CSTATUS_VALID_DATA &&
			item.value.CStatus != PDH_CSTATUS_NEW_DATA {
			// Does not necessarily indicate the problem, e.g. the process may have
			// exited by the time the formatting of its counter values happened
			log.Debugf("Counter value not valid for %s[%s]: %#x", counterName, item.instance, item.value.CStatus)
			continue
		}

		fn(item.instance, item.value)
	}

	return nil
}
