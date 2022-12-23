// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows
// +build windows

package pdhutil

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// For testing
var (
	pfnPdhOpenQuery                     = PdhOpenQuery
	pfnPdhAddEnglishCounter             = PdhAddEnglishCounter
	pfnPdhCollectQueryData              = PdhCollectQueryData
	pfnPdhRemoveCounter                 = PdhRemoveCounter
	pfnPdhGetFormattedCounterValueFloat = pdhGetFormattedCounterValueFloat
	pfnPdhGetFormattedCounterArray      = pdhGetFormattedCounterArray
	pfnPdhCloseQuery                    = PdhCloseQuery
	pfnPdhMakeCounterPath               = pdhMakeCounterPath
)

// CounterInstanceVerify is a callback function called by GetCounterSet for each
// instance of the counter.  Implementation should return true if that instance
// should be included, false otherwise
type CounterInstanceVerify func(string) bool

// PdhCounterSet is the object which represents a pdh counter set.
type PdhCounterSet struct {
	className string
	query     PDH_HQUERY
	counter PDH_HCOUNTER

	counterName string
}

// PdhSingleInstanceCounterSet is a specialization for single instance counters
type PdhSingleInstanceCounterSet struct {
	PdhCounterSet
}

// PdhMultiInstanceCounterSet is a specialization for a multiple instance counter
type PdhMultiInstanceCounterSet struct {
	PdhCounterSet
	verifyfn             CounterInstanceVerify
}

// Initialize initializes a counter set object
func (p *PdhCounterSet) Initialize(className string, counterName string) error {

	// refresh PDH object cache (refresh will only occur periodically)
	tryRefreshPdhObjectCache()

	p.className = className
	p.counterName = counterName

	pdherror := pfnPdhOpenQuery(uintptr(0), uintptr(0), &p.query)
	if ERROR_SUCCESS != pdherror {
		err := fmt.Errorf("Failed to open PDH query handle %#x", pdherror)
		return err
	}
	return nil
}

// GetEnglishCounterInstance returns a specific instance of the given counter
// the className and counterName must be in English.
// See PdhAddEnglishCounter docs for details
// https://learn.microsoft.com/en-us/windows/win32/api/pdh/nf-pdh-pdhaddenglishcountera
func GetEnglishCounterInstance(className string, counterName string, instance string) (*PdhSingleInstanceCounterSet, error) {

	var p PdhSingleInstanceCounterSet
	if err := p.Initialize(className, counterName); err != nil {
		return nil, err
	}

	path, err := pfnPdhMakeCounterPath("", className, instance, counterName)
	if err != nil {
		return nil, fmt.Errorf("Failed to make counter path %s: %v", counterName, err)
	}
	pdherror := pfnPdhAddEnglishCounter(p.query, path, uintptr(0), &p.counter)
	if ERROR_SUCCESS != pdherror {
		return nil, fmt.Errorf("Failed to add english counter %#x", pdherror)
	}
	pdherror = pfnPdhCollectQueryData(p.query)
	if ERROR_SUCCESS != pdherror {
		return nil, fmt.Errorf("Failed to collect query data %#x", pdherror)
	}
	return &p, nil
}

// GetEnglishSingleInstanceCounter returns a single instance counter object for the given counter class
// the className and counterName must be in English.
// See PdhAddEnglishCounter docs for details
// https://learn.microsoft.com/en-us/windows/win32/api/pdh/nf-pdh-pdhaddenglishcountera
func GetEnglishSingleInstanceCounter(className string, counterName string) (*PdhSingleInstanceCounterSet, error) {
	return GetEnglishCounterInstance(className, counterName, "")
}

// GetMultiInstanceCounter returns a multi-instance counter object for the given counter class
func GetEnglishMultiInstanceCounter(className string, counterName string, verifyfn CounterInstanceVerify) (*PdhMultiInstanceCounterSet, error) {
	var p PdhMultiInstanceCounterSet
	if err := p.Initialize(className, counterName); err != nil {
		return nil, err
	}

	p.verifyfn = verifyfn

	// Use the * wildcard to collect all instances
	path, err := pfnPdhMakeCounterPath("", className, "*", counterName)
	if err != nil {
		return nil, fmt.Errorf("Failed to make counter path %s: %v", counterName, err)
	}
	pdherror := pfnPdhAddEnglishCounter(p.query, path, uintptr(0), &p.counter)
	if ERROR_SUCCESS != pdherror {
		return nil, fmt.Errorf("Failed to add english counter %#x", pdherror)
	}
	pdherror = pfnPdhCollectQueryData(p.query)
	if ERROR_SUCCESS != pdherror {
		return nil, fmt.Errorf("Failed to collect query data %#x", pdherror)
	}

	return &p, nil
}

// GetAllValues returns the data associated with each instance in a query.
// verifyfn is used to filter out instance names that are returned
// instance:value pairs are not returned for items whose CStatus contains an error
func (p *PdhMultiInstanceCounterSet) GetAllValues() (values map[string]float64, err error) {
	// update data
	pfnPdhCollectQueryData(p.query)
	// fetch data
	items, err := pfnPdhGetFormattedCounterArray(p.counter, PDH_FMT_DOUBLE)
	if err != nil {
		return nil, err
	}
	values = make(map[string]float64)
	for _, item := range items {
		if p.verifyfn != nil {
			if p.verifyfn(item.instance) == false {
				// not interested, moving on
				continue
			}
		}
		if item.value.CStatus != PDH_CSTATUS_VALID_DATA &&
			item.value.CStatus != PDH_CSTATUS_NEW_DATA {
			// Does not necessarily indicate the problem, e.g. the process may have
			// exited by the time the formatting of its counter values happened
			log.Debugf("Counter value not valid for %s[%s]: %#x", p.counterName, item.instance, item.value.CStatus)
			continue
		}
		values[item.instance] = item.value.Double
	}
	return values, nil
}

// GetValue returns the data associated with a single-value counter
func (p *PdhSingleInstanceCounterSet) GetValue() (val float64, err error) {
	if p.counter == PDH_HCOUNTER(0) {
		return 0, fmt.Errorf("Not a single-value counter")
	}
	pfnPdhCollectQueryData(p.query)
	return pfnPdhGetFormattedCounterValueFloat(p.counter)

}

// Close closes the query handle, freeing the underlying windows resources.
func (p *PdhCounterSet) Close() {
	PdhCloseQuery(p.query)
}

