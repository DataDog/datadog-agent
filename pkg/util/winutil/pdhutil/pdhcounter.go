// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.
// +build windows

package pdhutil

import (
	"fmt"
	"strconv"
	"syscall"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"golang.org/x/sys/windows"
)

var (
	counterToIndex map[string][]int
)

// CounterInstanceVerify is a callback function called by GetCounterSet for each
// instance of the counter.  Implementation should return true if that instance
// should be included, false otherwise
type CounterInstanceVerify func(string) bool

// PdhCounterSet is the object which represents a pdh counter set.
type PdhCounterSet struct {
	className     string
	counterNames  []string
	query         PDH_HQUERY
	countermap    map[string][]PDH_HCOUNTER // map instance name to counter handle
	singleCounter []PDH_HCOUNTER
}

const singleInstanceKey = "_singleInstance_"

func makeCounterSetIndexes() error {
	counterToIndex = make(map[string][]int)

	bufferIncrement := uint32(1024)
	bufferSize := bufferIncrement
	var counterlist []uint16
	for {
		var regtype uint32
		counterlist = make([]uint16, bufferSize)
		var sz uint32
		sz = bufferSize
		regerr := windows.RegQueryValueEx(syscall.HKEY_PERFORMANCE_DATA,
			syscall.StringToUTF16Ptr("Counter 009"),
			nil, // reserved
			&regtype,
			(*byte)(unsafe.Pointer(&counterlist[0])),
			&sz)
		if regerr == error(syscall.ERROR_MORE_DATA) {
			// buffer's not big enough
			bufferSize += bufferIncrement
			continue
		} else if regerr != nil {
			return regerr
		}
		break
	}
	clist := winutil.ConvertWindowsStringList(counterlist)
	for i := 0; i < len(clist); i += 2 {
		ndx, _ := strconv.Atoi(clist[i])
		counterToIndex[clist[i+1]] = append(counterToIndex[clist[i+1]], ndx)
	}
	return nil
}

// GetCounterSet returns an initialized PDH counter set.
func GetCounterSet(className string, counterNames []string, instanceName string, verifyfn CounterInstanceVerify) (*PdhCounterSet, error) {
	var p PdhCounterSet
	p.countermap = make(map[string][]PDH_HCOUNTER)
	var err error

	// the counter index list may be > 1, but for class name, only take the first
	// one.  If not present at all, try the english counter name
	ndxlist, err := getCounterIndexList(className)
	if err != nil {
		return nil, err
	}
	if ndxlist == nil || len(ndxlist) == 0 {
		log.Warnf("Didn't find counter index for class %s, attempting english counter", className)
		p.className = className
	} else {
		if len(ndxlist) > 1 {
			log.Warnf("Class %s had multiple (%d) indices, using first", className, len(ndxlist))
		}
		ndx := ndxlist[0]
		p.className, err = pdhLookupPerfNameByIndex(ndx)
		if err != nil {
			return nil, fmt.Errorf("Class name not found: %s", counterNames)
		}
		log.Debugf("Found class name for %s %s", className, p.className)
	}

	winerror := PdhOpenQuery(uintptr(0), uintptr(0), &p.query)
	if ERROR_SUCCESS != winerror {
		err = fmt.Errorf("Failed to open PDH query handle %d", winerror)
		return nil, err
	}
	allcounters, instances, err := pdhEnumObjectItems(p.className)
	if err != nil {
		return nil, err
	}
	if instanceName == "" && len(instances) > 0 {
		// asked for all instances of this class
		for _, inst := range instances {
			if verifyfn != nil {
				if verifyfn(inst) == false {
					// verify function said not interested in this instance, move on
					continue
				}
			}
			paths, err := p.MakeCounterPath("", counterNames, inst, allcounters)
			if err != nil {
				continue
			}
			for _, path := range paths {
				var hc PDH_HCOUNTER
				winerror = PdhAddCounter(p.query, path, uintptr(0), &hc)
				if ERROR_SUCCESS != winerror {
					continue
				}
				if hcs, found := p.countermap[inst]; found {
					p.countermap[inst] = append(hcs, hc)
				} else {
					p.countermap[inst] = []PDH_HCOUNTER{hc}
				}
			}
		}
	} else {
		if instanceName != "" {
			// they asked for specific instance
			if len(instances) <= 0 {
				return nil, fmt.Errorf("Requested instance of sigle instance counter")
			}
			found := false
			for _, inst := range instances {
				if inst == instanceName {
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("Didn't find instance name %s", instanceName)
			}
		}
		paths, err := p.MakeCounterPath("", counterNames, instanceName, allcounters)
		if err != nil {
			return nil, err
		}
		p.singleCounter = make([]PDH_HCOUNTER, 0, len(paths))
		for _, path := range paths {
			var hc PDH_HCOUNTER
			winerror = PdhAddCounter(p.query, path, uintptr(0), &hc)
			if ERROR_SUCCESS != winerror {
				return nil, fmt.Errorf("Failed to add single counter %d", winerror)
			}
			p.singleCounter = append(p.singleCounter, hc)
		}

	}
	// do the initial collect now
	PdhCollectQueryData(p.query)
	return &p, nil
}

// MakeCounterPath creates a counter path from the counter instance and
// counter name.  Tries all available translated counter indexes from
// the english name
func (p *PdhCounterSet) MakeCounterPath(machine string, counterNames []string, instanceName string, counters []string) ([]string, error) {
	/*
	   When handling non english versions, the counters don't work quite as documented.
	   This is because strings like "Bytes Sent/sec" might appear multiple times in the
	   english master, and might not have mappings for each index.

	   Search each index, and make sure the requested counter name actually appears in
	   the list of available counters; that's the counter we'll use.

	   For more information, see README.md.
	*/
	paths := make([]string, 0, len(counterNames))
	for _, counterName := range counterNames {
		idxList, err := getCounterIndexList(counterName)
		if err != nil {
			return paths, err
		}
		var path string
		for _, ndx := range idxList {
			counter, e := pdhLookupPerfNameByIndex(ndx)
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
			path, err = pdhMakeCounterPath(machine, p.className, instanceName, counter)
			if err == nil {
				log.Debugf("Successfully created counter path %s", path)
				p.counterNames = append(p.counterNames, counter)
				paths = append(paths, path)
				break
			}
			// else
			log.Debugf("Unable to create path with %s, trying again", counter)
		}
		if err == nil {
			continue
		}
		// if we get here, was never able to find a counter path or create a valid
		// path.  Return failure.
		log.Warnf("Unable to create counter path for %s %s", counterName, instanceName)
		return paths, fmt.Errorf("Unable to create counter path %s %s", counterName, instanceName)
	}
	return paths, nil
}

// GetAllValues returns the data associated with each instance in a query.
func (p *PdhCounterSet) GetAllValues() (values map[string]map[string]float64, err error) {
	values = make(map[string]map[string]float64)
	err = nil
	PdhCollectQueryData(p.query)
	if len(p.singleCounter) != 0 {
		values[singleInstanceKey] = make(map[string]float64)
		for i, hcounter := range p.singleCounter {
			values[singleInstanceKey][p.counterNames[i]], _ = pdhGetFormattedCounterValueFloat(hcounter)
		}
		return
	}
	for inst, hcounters := range p.countermap {
		values[inst] = make(map[string]float64)
		for i, hcounter := range hcounters {
			values[inst][p.counterNames[i]], err = pdhGetFormattedCounterValueFloat(hcounter)
		}
		if err != nil {
			return
		}
	}
	return
}

// GetSingleValue returns the data associated with a single-value counter
func (p *PdhCounterSet) GetSingleValue() (val map[string]float64, err error) {
	if len(p.singleCounter) == 0 {
		return map[string]float64{}, fmt.Errorf("Not a single-value counter")
	}
	vals, err := p.GetAllValues()
	if err != nil {
		return map[string]float64{}, err
	}
	return vals[singleInstanceKey], nil
}

// Close closes the query handle, freeing the underlying windows resources.
func (p *PdhCounterSet) Close() {
	PdhCloseQuery(p.query)
}

func getCounterIndexList(cname string) ([]int, error) {
	if counterToIndex == nil || len(counterToIndex) == 0 {
		if err := makeCounterSetIndexes(); err != nil {
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
