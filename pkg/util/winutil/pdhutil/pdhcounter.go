// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.
// +build windows

package pdhutil

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
	"strconv"
	"syscall"
	"unsafe"
)

var (
	counterToIndex map[string]int
)

// CounterInstanceVerify is a callback function called by GetCounterSet for each
// instance of the counter.  Implementation should return true if that instance
// should be included, false otherwise
type CounterInstanceVerify func(string) bool

// PdhCounterSet is the object which represents a pdh counter set.
type PdhCounterSet struct {
	className     string
	counterName   string
	query         win.PDH_HQUERY
	countermap    map[string]win.PDH_HCOUNTER // map instance name to counter handle
	singleCounter win.PDH_HCOUNTER
}

const singleInstanceKey = "_singleInstance_"

func init() {
	counterToIndex = make(map[string]int)

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
		}
		break
	}
	clist := winutil.ConvertWindowsStringList(counterlist)
	for i := 0; i < len(clist); i += 2 {
		ndx, _ := strconv.Atoi(clist[i])
		counterToIndex[clist[i+1]] = ndx
	}

}

// GetCounterSet returns an initialized PDH counter set.
func GetCounterSet(className string, counterName string, instanceName string, verifyfn CounterInstanceVerify) (*PdhCounterSet, error) {
	var p PdhCounterSet
	p.countermap = make(map[string]win.PDH_HCOUNTER)
	var ndx int
	var err error
	if ndx = getCounterIndex(className); ndx == -1 {
		return nil, fmt.Errorf("Class name not found: %s", className)
	}
	p.className, err = pdhLookupPerfNameByIndex(ndx)
	if err != nil {
		return nil, err
	}
	if ndx = getCounterIndex(counterName); ndx == -1 {
		return nil, fmt.Errorf("Class name not found: %s", counterName)
	}
	p.counterName, err = pdhLookupPerfNameByIndex(ndx)
	if err != nil {
		return nil, err
	}
	winerror := win.PdhOpenQuery(uintptr(0), uintptr(0), &p.query)
	if win.ERROR_SUCCESS != winerror {
		err = fmt.Errorf("Failed to open PDH query handle %d", winerror)
		return nil, err
	}
	_, instances, err := pdhEnumObjectItems(p.className)
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
			path, err := pdhMakeCounterPath("", p.className, inst, p.counterName)
			if err != nil {
				continue
			}
			var hc win.PDH_HCOUNTER
			winerror = win.PdhAddCounter(p.query, path, uintptr(0), &hc)
			if win.ERROR_SUCCESS != winerror {
				continue
			}
			p.countermap[inst] = hc
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
		path, err := pdhMakeCounterPath("", p.className, instanceName, p.counterName)
		if err != nil {
			return nil, err
		}
		winerror = win.PdhAddCounter(p.query, path, uintptr(0), &p.singleCounter)
		if win.ERROR_SUCCESS != winerror {
			return nil, fmt.Errorf("Failed to add single counter %d", winerror)
		}
	}
	// do the initial collect now
	win.PdhCollectQueryData(p.query)
	return &p, nil
}

// GetAllValues returns the data associated with each instance in a query.
func (p *PdhCounterSet) GetAllValues() (values map[string]float64, err error) {
	values = make(map[string]float64)
	err = nil
	win.PdhCollectQueryData(p.query)
	if p.singleCounter != win.PDH_HCOUNTER(0) {
		values[singleInstanceKey], _ = pdhGetFormattedCounterValueFloat(p.singleCounter)
		return
	}
	for inst, hcounter := range p.countermap {
		values[inst], err = pdhGetFormattedCounterValueFloat(hcounter)
		if err != nil {
			return
		}
	}
	return
}

// Close closes the query handle, freeing the underlying windows resources.
func (p *PdhCounterSet) Close() {
	win.PdhCloseQuery(p.query)
}

func getCounterIndex(cname string) int {
	ndx, found := counterToIndex[cname]
	if !found {
		return -1
	}
	return ndx
}
