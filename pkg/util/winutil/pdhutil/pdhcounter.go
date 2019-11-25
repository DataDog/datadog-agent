// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build windows

package pdhutil

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// For testing
var (
	pfnMakeCounterSetInstances          = makeCounterSetIndexes
	pfnPdhOpenQuery                     = PdhOpenQuery
	pfnPdhAddCounter                    = PdhAddCounter
	pfnPdhCollectQueryData              = PdhCollectQueryData
	pfnPdhEnumObjectItems               = pdhEnumObjectItems
	pfnPdhRemoveCounter                 = PdhRemoveCounter
	pfnPdhLookupPerfNameByIndex         = pdhLookupPerfNameByIndex
	pfnPdhGetFormattedCounterValueFloat = pdhGetFormattedCounterValueFloat
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

	counterName string
}

// PdhSingleInstanceCounterSet is a specialization for single instance counters
type PdhSingleInstanceCounterSet struct {
	PdhCounterSet
	singleCounter PDH_HCOUNTER
}

// PdhMultiInstanceCounterSet is a specialization for a multiple instance counter
type PdhMultiInstanceCounterSet struct {
	PdhCounterSet
	requestedCounterName string
	requestedInstances   map[string]bool
	countermap           map[string]PDH_HCOUNTER // map instance name to counter handle
	verifyfn             CounterInstanceVerify
}

// Initialize initializes a counter set object
func (p *PdhCounterSet) Initialize(className string) error {

	// the counter index list may be > 1, but for class name, only take the first
	// one.  If not present at all, try the english counter name
	ndxlist, err := getCounterIndexList(className)
	if err != nil {
		return err
	}
	if ndxlist == nil || len(ndxlist) == 0 {
		log.Warnf("Didn't find counter index for class %s, attempting english counter", className)
		p.className = className
	} else {
		if len(ndxlist) > 1 {
			log.Warnf("Class %s had multiple (%d) indices, using first", className, len(ndxlist))
		}
		ndx := ndxlist[0]
		p.className, err = pfnPdhLookupPerfNameByIndex(ndx)
		if err != nil {
			return fmt.Errorf("Class name not found: %s", className)
		}
		log.Debugf("Found class name for %s %s", className, p.className)
	}

	winerror := pfnPdhOpenQuery(uintptr(0), uintptr(0), &p.query)
	if ERROR_SUCCESS != winerror {
		err = fmt.Errorf("Failed to open PDH query handle %d", winerror)
		return err
	}
	return nil
}

// GetSingleInstanceCounter returns a single instance counter object for the given counter class
func GetSingleInstanceCounter(className, counterName string) (*PdhSingleInstanceCounterSet, error) {
	var p PdhSingleInstanceCounterSet
	if err := p.Initialize(className); err != nil {
		return nil, err
	}
	// check to make sure this is really a single instance counter
	allcounters, instances, _ := pfnPdhEnumObjectItems(p.className)
	if len(instances) > 0 {
		return nil, fmt.Errorf("Requested counter is not single-instance: %s", p.className)
	}
	path, err := p.MakeCounterPath("", counterName, "", allcounters)
	if err != nil {
		log.Warnf("Failed pdhEnumObjectItems %v", err)
		return nil, err
	}
	winerror := pfnPdhAddCounter(p.query, path, uintptr(0), &p.singleCounter)
	if ERROR_SUCCESS != winerror {
		return nil, fmt.Errorf("Failed to add single counter %d", winerror)
	}

	// do the initial collect now
	pfnPdhCollectQueryData(p.query)
	return &p, nil
}

// GetMultiInstanceCounter returns a multi-instance counter object for the given counter class
func GetMultiInstanceCounter(className, counterName string, requestedInstances *[]string, verifyfn CounterInstanceVerify) (*PdhMultiInstanceCounterSet, error) {
	var p PdhMultiInstanceCounterSet
	if err := p.Initialize(className); err != nil {
		return nil, err
	}
	p.countermap = make(map[string]PDH_HCOUNTER)
	p.verifyfn = verifyfn
	p.requestedCounterName = counterName

	// check to make sure this is really a single instance counter
	_, instances, _ := pfnPdhEnumObjectItems(p.className)
	if len(instances) <= 0 {
		return nil, fmt.Errorf("Requested counter is a single-instance: %s", p.className)
	}
	// save the requested instances
	if requestedInstances != nil && len(*requestedInstances) > 0 {
		p.requestedInstances = make(map[string]bool)
		for _, inst := range *requestedInstances {
			p.requestedInstances[inst] = true
		}
	}
	if err := p.MakeInstanceList(); err != nil {
		return nil, err
	}
	return &p, nil

}

// MakeInstanceList walks the list of available instances, and adds new
// instances that have appeared since the last check run
func (p *PdhMultiInstanceCounterSet) MakeInstanceList() error {
	allcounters, instances, err := pfnPdhEnumObjectItems(p.className)
	if err != nil {
		return err
	}
	var instToMake []string
	for _, actualInstance := range instances {
		// if we have a list of requested instances, walk it, and make sure
		// they're here.  If not, add them to the list of instances to make
		if p.requestedInstances != nil {
			// if it's not in the requestedInstances, don't bother
			if !p.requestedInstances[actualInstance] {
				continue
			}
			// ok.  it was requested.  If it's not in our map
			// of counters, we have to add it
			if p.countermap[actualInstance] == PDH_HCOUNTER(0) {
				log.Debugf("Adding requested instance %s", actualInstance)
				instToMake = append(instToMake, actualInstance)
			}
		} else {
			// wanted all the instances.  Make sure all of the instances
			// are present
			if p.countermap[actualInstance] == PDH_HCOUNTER(0) {
				instToMake = append(instToMake, actualInstance)
			}
		}
	}
	added := false
	for _, inst := range instToMake {
		if p.verifyfn != nil {
			if p.verifyfn(inst) == false {
				// not interested, moving on
				continue
			}
		}
		path, err := p.MakeCounterPath("", p.requestedCounterName, inst, allcounters)
		if err != nil {
			log.Debugf("Failed tomake counter path %s %s", p.counterName, inst)
			continue
		}
		var hc PDH_HCOUNTER
		winerror := pfnPdhAddCounter(p.query, path, uintptr(0), &hc)
		if ERROR_SUCCESS != winerror {
			log.Debugf("Failed to add counter path %s", path)
			continue
		}
		log.Debugf("Adding missing counter instance %s", inst)
		p.countermap[inst] = hc
		added = true
	}
	if added {
		// do the initial collect now
		pfnPdhCollectQueryData(p.query)
	}
	return nil
}

//RemoveInvalidInstance removes an instance from the counter that is no longer valid
func (p *PdhMultiInstanceCounterSet) RemoveInvalidInstance(badInstance string) {
	hc := p.countermap[badInstance]
	if hc != PDH_HCOUNTER(0) {
		log.Debugf("Removing non-existent counter instance %s", badInstance)
		pfnPdhRemoveCounter(hc)
		delete(p.countermap, badInstance)
	} else {
		log.Debugf("Instance handle not found")
	}
}

// MakeCounterPath creates a counter path from the counter instance and
// counter name.  Tries all available translated counter indexes from
// the english name
func (p *PdhCounterSet) MakeCounterPath(machine, counterName, instanceName string, counters []string) (string, error) {
	/*
	   When handling non english versions, the counters don't work quite as documented.
	   This is because strings like "Bytes Sent/sec" might appear multiple times in the
	   english master, and might not have mappings for each index.

	   Search each index, and make sure the requested counter name actually appears in
	   the list of available counters; that's the counter we'll use.

	   For more information, see README.md.
	*/
	idxList, err := getCounterIndexList(counterName)
	if err != nil {
		return "", err
	}
	for _, ndx := range idxList {
		counter, e := pfnPdhLookupPerfNameByIndex(ndx)
		if e != nil {
			log.Debugf("Counter index %d not found, skipping", ndx)
			continue
		}
		// see if the counter we got back is in the list of counters
		if !stringInSlice(counter, counters) {
			log.Debugf("counter %s not in counter list", counter)
			continue
		}
		// check to see if we can create the counter
		path, err := pfnPdhMakeCounterPath(machine, p.className, instanceName, counter)
		if err == nil {
			log.Debugf("Successfully created counter path %s", path)
			p.counterName = counter
			return path, nil
		}
		// else
		log.Debugf("Unable to create path with %s, trying again", counter)
	}
	// if we get here, was never able to find a counter path or create a valid
	// path.  Return failure.
	log.Warnf("Unable to create counter path for %s %s", counterName, instanceName)
	return "", fmt.Errorf("Unable to create counter path %s %s", counterName, instanceName)
}

// GetAllValues returns the data associated with each instance in a query.
func (p *PdhMultiInstanceCounterSet) GetAllValues() (values map[string]float64, err error) {
	values = make(map[string]float64)
	err = nil
	var removeList []string
	pfnPdhCollectQueryData(p.query)
	for inst, hcounter := range p.countermap {
		var retval float64
		retval, err = pfnPdhGetFormattedCounterValueFloat(hcounter)
		if err != nil {
			switch err.(type) {
			case *ErrPdhInvalidInstance:
				removeList = append(removeList, inst)
				log.Debugf("Got invalid instance for %s %s", p.requestedCounterName, inst)
				err = nil
				continue
			default:
				log.Debugf("Other Error getting all values %s %s %v", p.requestedCounterName, inst, err)
				return
			}
		}
		values[inst] = retval
	}
	for _, inst := range removeList {
		p.RemoveInvalidInstance(inst)
	}
	// check for newly found instances
	p.MakeInstanceList()
	return
}

// GetValue returns the data associated with a single-value counter
func (p *PdhSingleInstanceCounterSet) GetValue() (val float64, err error) {
	if p.singleCounter == PDH_HCOUNTER(0) {
		return 0, fmt.Errorf("Not a single-value counter")
	}
	pfnPdhCollectQueryData(p.query)
	return pfnPdhGetFormattedCounterValueFloat(p.singleCounter)

}

// Close closes the query handle, freeing the underlying windows resources.
func (p *PdhCounterSet) Close() {
	PdhCloseQuery(p.query)
}

func getCounterIndexList(cname string) ([]int, error) {
	if counterToIndex == nil || len(counterToIndex) == 0 {
		if err := pfnMakeCounterSetInstances(); err != nil {
			return []int{}, err
		}
	}

	ndxlist, found := counterToIndex[cname]
	if !found {
		return []int{}, nil
	}
	return ndxlist, nil
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}
