// +build windows

package pdhutil

import (
	"fmt"
	"strings"
)

var activeCounterStrings CounterStrings
var activeAvailableCounters AvailableCounters

func mockmakeCounterSetIndexes() error {
	if !activeCounterStrings.initialized {
		return fmt.Errorf("Counter strings not initialized")
	}
	counterToIndex = make(map[string][]int)
	for k, v := range activeCounterStrings.counterIndex {
		counterToIndex[v] = append(counterToIndex[v], k)
	}
	return nil
}

type mockCounter struct {
	name string
}

type mockQuery struct {
	counters map[int]mockCounter
}

var openQueries = make(map[int]mockQuery)
var openQueriesIndex = 0
var counterIndex = 0 // index of counter into the query must be global, because
// RemoveCounter can be called on just a counter index

var countervalues = make(map[string][]float64)

func mockPdhOpenQuery(szDataSource uintptr, dwUserData uintptr, phQuery *PDH_HQUERY) uint32 {
	var mq mockQuery
	mq.counters = make(map[int]mockCounter)
	openQueriesIndex++

	openQueries[openQueriesIndex] = mq
	*phQuery = PDH_HQUERY(uintptr(openQueriesIndex))
	return 0
}

func mockPdhAddCounter(hQuery PDH_HQUERY, szFullCounterPath string, dwUserData uintptr, phCounter *PDH_HCOUNTER) uint32 {
	ndx := int(hQuery)
	var thisQuery mockQuery
	var ok bool
	if thisQuery, ok = openQueries[ndx]; ok == false {
		return uint32(PDH_INVALID_PATH)
	}
	counterIndex++

	thisQuery.counters[counterIndex] = mockCounter{name: szFullCounterPath}
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

func mockPdhRemoveCounter(hCounter PDH_HCOUNTER) uint32 {
	counterIndex := int(hCounter)

	for _, query := range openQueries {
		if _, ok := query.counters[counterIndex]; ok {
			delete(query.counters, counterIndex)
			return 0
		}
	}
	return PDH_INVALID_HANDLE
}

func mockpdhLookupPerfNameByIndex(ndx int) (string, error) {
	if !activeCounterStrings.initialized {
		return "", fmt.Errorf("Counter strings not initialized")
	}
	if name, ok := activeCounterStrings.counterIndex[ndx]; ok {
		return name, nil
	}
	return "", fmt.Errorf("Index not found")
}

func mockpdhGetFormattedCounterValueFloat(hCounter PDH_HCOUNTER) (val float64, err error) {
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
		return 0, fmt.Errorf("Invalid handle")
	}
	if _, ok = countervalues[ctr.name]; ok {
		if len(countervalues[ctr.name]) > 0 {
			val, countervalues[ctr.name] = countervalues[ctr.name][0], countervalues[ctr.name][1:]
			return val, nil
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

func mockpdhEnumObjectItems(className string) (counters []string, instances []string, err error) {
	counters = activeAvailableCounters.countersByClass[className]
	instances = activeAvailableCounters.instancesByClass[className]
	return
}

// SetupTesting initializes the PDH libarary with the mock functions rather than the real thing
func SetupTesting(counterstringsfile, countersfile string) {
	activeCounterStrings, _ = ReadCounterStrings(counterstringsfile)
	activeAvailableCounters, _ = ReadCounters(countersfile)
	// For testing
	pfnMakeCounterSetInstances = mockmakeCounterSetIndexes
	pfnPdhOpenQuery = mockPdhOpenQuery
	pfnPdhAddCounter = mockPdhAddCounter
	pfnPdhCollectQueryData = mockPdhCollectQueryData
	pfnPdhEnumObjectItems = mockpdhEnumObjectItems
	pfnPdhRemoveCounter = mockPdhRemoveCounter
	pfnPdhLookupPerfNameByIndex = mockpdhLookupPerfNameByIndex
	pfnPdhGetFormattedCounterValueFloat = mockpdhGetFormattedCounterValueFloat
	pfnPdhCloseQuery = mockPdhCloseQuery
	pfnPdhMakeCounterPath = mockpdhMakeCounterPath

}

// SetQueryReturnValue provides an entry point for tests to set expected values for a
// given counter
func SetQueryReturnValue(counter string, val float64) {
	countervalues[counter] = append(countervalues[counter], val)

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
