// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package pdhutil

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"golang.org/x/sys/windows"
)

// NOTE TO DEVELOPER
//
// This package uses terminology defined by the following MSDN article
// https://learn.microsoft.com/en-us/windows/win32/perfctrs/about-performance-counters

// For testing
var (
	pfnPdhOpenQuery                     = PdhOpenQuery
	pfnPdhAddEnglishCounter             = PdhAddEnglishCounter
	pfnPdhCollectQueryData              = pdhCollectQueryData
	pfnPdhGetFormattedCounterValueFloat = pdhGetFormattedCounterValueFloat
	pfnPdhGetFormattedCounterArray      = pdhGetFormattedCounterArray
	pfnPdhRemoveCounter                 = PdhRemoveCounter
	pfnPdhCloseQuery                    = PdhCloseQuery
	pfnPdhMakeCounterPath               = pdhMakeCounterPath
)

// CounterInstanceVerify is a callback function called by GetAllValues for each
// instance of the counter.  Implementation should return true if that instance
// should be included, false otherwise
type CounterInstanceVerify func(string) bool

// PdhQuery manages a PDH Query
// https://learn.microsoft.com/en-us/windows/win32/perfctrs/creating-a-query
// https://learn.microsoft.com/en-us/windows/win32/api/pdh/nf-pdh-pdhopenqueryw
type PdhQuery struct {
	Handle   PDH_HQUERY
	counters []PdhCounter
}

// PdhCounter manages behavior common to all types of PDH counters
// https://learn.microsoft.com/en-us/windows/win32/api/pdh/nf-pdh-pdhaddenglishcounterw
type PdhCounter interface {
	// Return true if a query should attempt to initialize this counter.
	// Return false if a query must not attempt to initialize this counter.
	ShouldInit() bool

	// Called during (*PdhQuery).CollectQueryData via AddToQuery() for counters that return true from ShouldInit()
	// Must call the appropriate PdhAddCounter/PdhAddEnglishCounter function to add the
	// counter to the query.
	AddToQuery(*PdhQuery) error

	// Given the result of PdhCounter.AddToQuery, should update initError and initFailCount
	SetInitError(error) error

	// Calls PdhRemoveCounter and updates internal state (handle field)
	Remove() error
}

// PdhSingleInstanceCounter manages a PDH counter with no instance or for a specific instance
// Only a single value is returned.
// https://learn.microsoft.com/en-us/windows/win32/perfctrs/specifying-a-counter-path
type PdhSingleInstanceCounter interface {
	PdhCounter
	// Return the counter value formatted as a float
	GetValue() (float64, error)
}

// PdhMultiInstanceCounter manages a PDH counter that can have multiple instances
// Returns a value for every instance
// https://learn.microsoft.com/en-us/windows/win32/perfctrs/specifying-a-counter-path
type PdhMultiInstanceCounter interface {
	PdhCounter
	// Return a map of instance name -> counter value formatted as a float
	GetAllValues() (map[string]float64, error)
}

// pdhCounter contains info that is common to all counter types
type pdhCounter struct {
	handle PDH_HCOUNTER

	// Parts of PDH counter path
	ObjectName   string // also referred to by Microsoft as a counterset, class, or performance object
	InstanceName string
	CounterName  string

	initError     error
	initFailCount int
}

// pdhEnglishCounter implements AddToQuery for both single and multi instance english counters
type pdhEnglishCounter struct {
	pdhCounter
}

// PdhEnglishSingleInstanceCounter is a specialization for single-instance counters
// https://learn.microsoft.com/en-us/windows/win32/perfctrs/about-performance-counters
type PdhEnglishSingleInstanceCounter struct {
	pdhEnglishCounter
}

// PdhEnglishMultiInstanceCounter is a specialization for multi-instance counters
// https://learn.microsoft.com/en-us/windows/win32/perfctrs/about-performance-counters
type PdhEnglishMultiInstanceCounter struct {
	pdhEnglishCounter
	verifyfn CounterInstanceVerify
}

func (counter *pdhCounter) ShouldInit() bool {
	if counter.handle != PDH_HCOUNTER(0) {
		// already initialized
		return false
	}
	var initFailLimit = config.Datadog.GetInt("windows_counter_init_failure_limit")
	if initFailLimit > 0 && counter.initFailCount >= initFailLimit {
		counter.initError = fmt.Errorf("counter exceeded the maximum number of failed initialization attempts. This error indicates that the Windows performance counter database may need to be rebuilt")
		// attempts exceeded
		return false
	}
	return true
}

func (counter *pdhCounter) SetInitError(err error) error {
	if err == nil {
		counter.initError = nil
		return nil
	}

	counter.initFailCount++
	var initFailLimit = config.Datadog.GetInt("windows_counter_init_failure_limit")
	if initFailLimit > 0 && counter.initFailCount >= initFailLimit {
		err = fmt.Errorf("%v. Counter exceeded the maximum number of failed initialization attempts", err)
	} else if initFailLimit > 0 {
		err = fmt.Errorf("%v (Failure %d/%d)", err, counter.initFailCount, initFailLimit)
	} else {
		err = fmt.Errorf("%v (Failure %d)", err, counter.initFailCount)
	}
	counter.initError = err
	return counter.initError
}

func (counter *pdhCounter) Remove() error {
	if counter.handle == PDH_HCOUNTER(0) {
		return fmt.Errorf("counter is not initialized")
	}

	pdherror := pfnPdhRemoveCounter(counter.handle)
	if windows.ERROR_SUCCESS != windows.Errno(pdherror) {
		return fmt.Errorf("RemoveCounter failed: %#x", pdherror)
	}

	counter.handle = PDH_HCOUNTER(0)
	return nil
}

// Implements PdhCounter.AddToQuery for english counters.
func (counter *pdhEnglishCounter) AddToQuery(query *PdhQuery) error {
	path, err := pfnPdhMakeCounterPath("", counter.ObjectName, counter.InstanceName, counter.CounterName)
	if err != nil {
		return fmt.Errorf("Failed to make counter path (\\%s(%s)\\%s): %v", counter.ObjectName, counter.InstanceName, counter.CounterName, err)
	}
	pdherror := pfnPdhAddEnglishCounter(query.Handle, path, uintptr(0), &counter.handle)
	if windows.ERROR_SUCCESS != windows.Errno(pdherror) {
		return fmt.Errorf("Failed to add english counter (%s): %#x", path, pdherror)
	}
	return nil
}

// AddToQuery calls PdhCounter.AddToQuery and handles initError logic
// Counters should implement PdhCounter.AddToQuery to override init logic
func AddToQuery(query *PdhQuery, counter PdhCounter) error {
	err := counter.AddToQuery(query)
	return counter.SetInitError(err)
}

// AddCounter adds a counter to the list of counters managed by a PdhQuery.
// It does NOT add the Windows counter to the Windows query.
func (query *PdhQuery) AddCounter(counter PdhCounter) {
	query.counters = append(query.counters, counter)
}

// AddEnglishCounterInstance returns a PdhSingleInstanceCounter that will fetch the value of a given instance of the given counter.
// the objectName and counterName must be in English.
// See PdhAddEnglishCounter docs for details
// https://learn.microsoft.com/en-us/windows/win32/api/pdh/nf-pdh-pdhaddenglishcounterw
//
// Implementation detail: This function does not actually call the Windows API PdhAddEnglishCounter. That happens
//
//	when (*PdhQuery).CollectQueryData calls AddToQuery. This function only links our pdhCounter struct
//	to our PdhQuery struct.
//	We do this so we can handle several PDH error cases and their recovery behind the scenes to reduce
//	duplicate/error prone code in the checks that uses this package.
//	For example, see tryRefreshPdhObjectCache().
//	This function cannot fail, if it does then the checks that use it will need to be
//	restructured so that their Configure() call does not fail if the counter is not available
//	right away on host boot (see https://github.com/DataDog/datadog-agent/pull/13101).
//	All errors related to the counter are returned from the GetValue()/GetAllValues() function.
func (query *PdhQuery) AddEnglishCounterInstance(objectName string, counterName string, instanceName string) PdhSingleInstanceCounter {
	var p PdhEnglishSingleInstanceCounter
	p.Initialize(objectName, counterName, instanceName)
	query.AddCounter(&p)
	return &p
}

// AddEnglishSingleInstanceCounter returns a PdhSingleInstanceCounter that will fetch a single instance counter value.
// the objectName and counterName must be in English.
// See PdhAddEnglishCounter docs for details
// https://learn.microsoft.com/en-us/windows/win32/api/pdh/nf-pdh-pdhaddenglishcounterw
//
// Implementation detail: See AddEnglishCounterInstance()
func (query *PdhQuery) AddEnglishSingleInstanceCounter(objectName string, counterName string) PdhSingleInstanceCounter {
	return query.AddEnglishCounterInstance(objectName, counterName, "")
}

// AddEnglishMultiInstanceCounter returns a PdhMultiInstanceCounter that will fetch values for all instances of a counter.
// This uses a '*' wildcard to collect values for all instances of a counter.
// Instances/values can be filtered manually once returned from GetAllValues() or with verifyfn (see CounterInstanceVerify)
//
// Implementation detail: See AddEnglishCounterInstance()
func (query *PdhQuery) AddEnglishMultiInstanceCounter(objectName string, counterName string, verifyfn CounterInstanceVerify) PdhMultiInstanceCounter {
	var p PdhEnglishMultiInstanceCounter
	// Use the * wildcard to collect all instances
	p.Initialize(objectName, counterName, "*")
	p.verifyfn = verifyfn
	query.AddCounter(&p)
	return &p
}

// CollectQueryData updates the counter values managed by the query.
//
// Adds any Windows counters not yet added to the Windows query.
//
// Must be called before GetValue/GetAllValues to make new counter values available.
// https://learn.microsoft.com/en-us/windows/win32/api/pdh/nf-pdh-pdhcollectquerydata
func (query *PdhQuery) CollectQueryData() error {
	// iterate each of the counters and try to add them to the query
	var addedNewCounter = false
	for _, counter := range query.counters {
		if counter.ShouldInit() {
			// refresh PDH object cache (refresh will only occur periodically)
			// This will update Windows PDH internals and make any newly
			// initialized (during host boot) or newly enabled counters available to us.
			// https://learn.microsoft.com/en-us/previous-versions/windows/it-pro/windows-server-2003/cc784382(v=ws.10)
			_, _ = tryRefreshPdhObjectCache()

			err := AddToQuery(query, counter)
			if err == nil {
				addedNewCounter = true
			} else {
				log.Warnf("Failed to add counter to query: %v. This error indicates that the Windows performance counter database may need to be rebuilt.", err)
			}
		}
	}
	if addedNewCounter {
		// if we added a new counter then we need an additional call to
		// PdhCollectQuery data because some counters require two datapoints
		// before they can return a value.
		err := PdhCollectQueryData(query.Handle)
		if err != nil {
			return fmt.Errorf("%v. This error indicates that the Windows performance counter database may need to be rebuilt", err)
		}
	}

	// Update the counters
	err := PdhCollectQueryData(query.Handle)
	if err != nil {
		return fmt.Errorf("%v. This error indicates that the Windows performance counter database may need to be rebuilt", err)
	}
	return nil
}

// Initialize initializes a pdhCounter object
func (counter *pdhCounter) Initialize(objectName string, counterName string, instanceName string) {
	counter.ObjectName = objectName
	counter.CounterName = counterName
	counter.InstanceName = instanceName
}

// GetAllValues returns the data associated with each instance in a counter.
// verifyfn is used to filter out instance names that are returned
// instance:value pairs are not returned for items whose CStatus contains an error
func (counter *PdhEnglishMultiInstanceCounter) GetAllValues() (values map[string]float64, err error) {
	if counter.handle == PDH_HCOUNTER(0) {
		// If there was an error initializing this counter, return it here
		if counter.initError != nil {
			return nil, counter.initError
		}
		return nil, fmt.Errorf("counter is not initialized")
	}
	// fetch data
	items, err := pfnPdhGetFormattedCounterArray(counter.handle, PDH_FMT_DOUBLE)
	if err != nil {
		return nil, err
	}
	values = make(map[string]float64)
	for _, item := range items {
		if counter.verifyfn != nil {
			if !counter.verifyfn(item.instance) {
				// not interested, moving on
				continue
			}
		}
		if item.value.CStatus != PDH_CSTATUS_VALID_DATA &&
			item.value.CStatus != PDH_CSTATUS_NEW_DATA {
			// Does not necessarily indicate the problem, e.g. the process may have
			// exited by the time the formatting of its counter values happened
			log.Debugf("Counter value not valid for %s[%s]: %#x", counter.CounterName, item.instance, item.value.CStatus)
			continue
		}
		values[item.instance] = item.value.Double
	}
	return values, nil
}

// GetValue returns the data associated with a single-value counter
func (counter *PdhEnglishSingleInstanceCounter) GetValue() (float64, error) {
	if counter.handle == PDH_HCOUNTER(0) {
		// If there was an error initializing this counter, return it here
		if counter.initError != nil {
			return 0, counter.initError
		}
		return 0, fmt.Errorf("counter is not initialized")
	}
	// fetch data
	return pfnPdhGetFormattedCounterValueFloat(counter.handle)
}

// CreatePdhQuery creates a query that can have counters added to it
//
// https://learn.microsoft.com/en-us/windows/win32/api/pdh/nf-pdh-pdhopenqueryw
func CreatePdhQuery() (*PdhQuery, error) {
	var q PdhQuery

	pdherror := pfnPdhOpenQuery(uintptr(0), uintptr(0), &q.Handle)
	if windows.ERROR_SUCCESS != windows.Errno(pdherror) {
		err := fmt.Errorf("failed to open PDH query handle %#x", pdherror)
		return nil, err
	}
	return &q, nil
}

// Close closes the query handle, freeing the underlying windows resources.
// It is not necessary to remove the counters from the query before calling this function.
// PdhCloseQuery closes all counter handles associated with the query.
// https://learn.microsoft.com/en-us/windows/win32/perfctrs/creating-a-query
func (query *PdhQuery) Close() {
	if query.Handle != PDH_HQUERY(0) {
		pfnPdhCloseQuery(query.Handle)
		query.Handle = PDH_HQUERY(0)
	}
}

// PdhCollectQueryData Windows API
//
// https://learn.microsoft.com/en-us/windows/win32/api/pdh/nf-pdh-pdhcollectquerydata
func PdhCollectQueryData(hQuery PDH_HQUERY) error {
	if hQuery == PDH_HQUERY(0) {
		return fmt.Errorf("invalid query handle")
	}
	pdherror := pfnPdhCollectQueryData(hQuery)
	if windows.ERROR_SUCCESS != windows.Errno(pdherror) {
		return fmt.Errorf("failed to collect query data %#x", pdherror)
	}
	return nil
}
