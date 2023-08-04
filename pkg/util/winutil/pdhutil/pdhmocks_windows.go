// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package pdhutil

import (
	"fmt"
	"regexp"
	"strings"
)

var activeAvailableCounters AvailableCounters

type mockCounter struct {
	path     string
	machine  string
	class    string
	instance string
	counter  string
}

type mockQuery struct {
	counters map[int]mockCounter
}

var openQueries = make(map[int]mockQuery)
var openQueriesIndex = 0
var counterIndex = 0 // index of counter into the query must be global, because
// RemoveCounter can be called on just a counter index

// class -> counter -> instance -> values
var counterValues = make(map[string]map[string]map[string][]float64)

func mockCounterFromString(path string) mockCounter {
	// Example: \\.\LogicalDisk(HarddiskVolume2)\Current Disk Queue Length
	// Example: \\.\Memory\Available Bytes
	r := regexp.MustCompile(`\\\\([^\\]+)\\([^\\\(]+)(?:\(([^\\\)]+)\))?\\(.+)`)
	res := r.FindStringSubmatch(path)
	return mockCounter{
		path:     path,
		machine:  res[1],
		class:    res[2],
		instance: res[3],
		counter:  res[4],
	}
}

func mockPdhOpenQuery(szDataSource uintptr, dwUserData uintptr, phQuery *PDH_HQUERY) uint32 {
	var mq mockQuery
	mq.counters = make(map[int]mockCounter)
	openQueriesIndex++

	openQueries[openQueriesIndex] = mq
	*phQuery = PDH_HQUERY(uintptr(openQueriesIndex))
	return 0
}

func mockPdhAddEnglishCounter(hQuery PDH_HQUERY, szFullCounterPath string, dwUserData uintptr, phCounter *PDH_HCOUNTER) uint32 {
	ndx := int(hQuery)
	var thisQuery mockQuery
	var ok bool
	if thisQuery, ok = openQueries[ndx]; ok == false {
		return uint32(PDH_INVALID_PATH)
	}
	counterIndex++

	mc := mockCounterFromString(szFullCounterPath)
	thisQuery.counters[counterIndex] = mc
	*phCounter = PDH_HCOUNTER(uintptr(counterIndex))

	return 0
}

func mockPdhCollectQueryData(hQuery PDH_HQUERY) uint32 {
	return 0
}

func mockPdhCloseQuery(hQuery PDH_HQUERY) uint32 {
	iQuery := int(hQuery)
	if _, ok := openQueries[iQuery]; !ok {
		return PDH_INVALID_HANDLE
	}
	delete(openQueries, iQuery)
	return 0
}

func mockCounterFromHandle(hCounter PDH_HCOUNTER) (mockCounter, error) {
	// check to see that it's a valid counter
	ndx := int(hCounter)
	var ctr mockCounter
	var ok bool
	for _, query := range openQueries {
		if ctr, ok = query.counters[ndx]; ok {
			break
		}
	}
	if !ok {
		return ctr, fmt.Errorf("Invalid handle")
	}
	return ctr, nil

}
func mockPdhGetFormattedCounterArray(hCounter PDH_HCOUNTER, format uint32) (out_items []PdhCounterValueItem, err error) {
	ctr, err := mockCounterFromHandle(hCounter)
	if err != nil {
		return nil, err
	}
	if classMap, ok := counterValues[ctr.class]; ok {
		if instMap, ok := classMap[ctr.counter]; ok {
			for inst, vals := range instMap {
				if len(vals) > 0 {
					out_items = append(out_items,
						PdhCounterValueItem{
							instance: inst,
							value: PdhCounterValue{
								CStatus: PDH_CSTATUS_NEW_DATA,
								Double:  vals[0],
							},
						},
					)
					instMap[inst] = vals[1:]
				}
			}
			return out_items, nil
		}
	}
	return nil, NewErrPdhInvalidInstance("Invalid counter instance")
}

func mockpdhGetFormattedCounterValueFloat(hCounter PDH_HCOUNTER) (val float64, err error) {
	ctr, err := mockCounterFromHandle(hCounter)
	if err != nil {
		return 0, err
	}
	if classMap, ok := counterValues[ctr.class]; ok {
		if instMap, ok := classMap[ctr.counter]; ok {
			if vals, ok := instMap[ctr.instance]; ok {
				if len(vals) > 0 {
					val, instMap[ctr.instance] = vals[0], vals[1:]
					return val, nil
				}
			}
		}
	}
	return 0, NewErrPdhInvalidInstance("Invalid counter instance")
}

func mockpdhMakeCounterPath(machine string, object string, instance string, counter string) (path string, err error) {
	var inst string
	if len(instance) != 0 {
		inst = fmt.Sprintf("(%s)", instance)
	}
	if len(machine) == 0 {
		machine = "."
	}
	path = fmt.Sprintf("\\\\%s\\%s%s\\%s", machine, object, inst, counter)
	return
}

// SetupTesting initializes the PDH libarary with the mock functions rather than the real thing
func SetupTesting(counterstringsfile, countersfile string) {
	activeAvailableCounters, _ = ReadCounters(countersfile)
	// For testing
	pfnPdhOpenQuery = mockPdhOpenQuery
	pfnPdhAddEnglishCounter = mockPdhAddEnglishCounter
	pfnPdhCollectQueryData = mockPdhCollectQueryData
	pfnPdhGetFormattedCounterValueFloat = mockpdhGetFormattedCounterValueFloat
	pfnPdhGetFormattedCounterArray = mockPdhGetFormattedCounterArray
	pfnPdhCloseQuery = mockPdhCloseQuery
	pfnPdhMakeCounterPath = mockpdhMakeCounterPath

}

// SetQueryReturnValue provides an entry point for tests to set expected values for a
// given counter
func SetQueryReturnValue(counter string, val float64) {
	mc := mockCounterFromString(counter)
	// class -> counter
	counterMap, ok := counterValues[mc.class]
	if !ok {
		counterMap = make(map[string]map[string][]float64)
		counterValues[mc.class] = counterMap
	}
	// counter -> instance
	instMap, ok := counterMap[mc.counter]
	if !ok {
		instMap = make(map[string][]float64)
		counterMap[mc.counter] = instMap
	}
	// instance -> value list
	instMap[mc.instance] = append(instMap[mc.instance], val)
}

// RemoveCounterInstance removes a specific instance from the table of available instances
func RemoveCounterInstance(clss, inst string) {
	for idx, val := range activeAvailableCounters.instancesByClass[clss] {
		if strings.EqualFold(inst, val) {
			activeAvailableCounters.instancesByClass[clss] =
				append(activeAvailableCounters.instancesByClass[clss][:idx],
					activeAvailableCounters.instancesByClass[clss][idx+1:]...)
			return
		}
	}
}

// AddCounterInstance adds a specific instance to the table of available instances
func AddCounterInstance(clss, inst string) {
	activeAvailableCounters.instancesByClass[clss] =
		append(activeAvailableCounters.instancesByClass[clss], inst)
	return
}
