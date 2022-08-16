// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows
// +build windows

package pdhutil

import (
	"fmt"
	"strconv"
	"unsafe"
	"time"
	"sync"

	"golang.org/x/sys/windows"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

var (
	modPdhDll = windows.NewLazyDLL("pdh.dll")

	procPdhLookupPerfNameByIndex    = modPdhDll.NewProc("PdhLookupPerfNameByIndexW")
	procPdhEnumObjects              = modPdhDll.NewProc("PdhEnumObjectsW")
	procPdhEnumObjectItems          = modPdhDll.NewProc("PdhEnumObjectItemsW")
	procPdhMakeCounterPath          = modPdhDll.NewProc("PdhMakeCounterPathW")
	procPdhGetFormattedCounterValue = modPdhDll.NewProc("PdhGetFormattedCounterValue")
	procPdhAddCounterW              = modPdhDll.NewProc("PdhAddCounterW")
	procPdhAddEnglishCounterW       = modPdhDll.NewProc("PdhAddEnglishCounterW")
	procPdhCollectQueryData         = modPdhDll.NewProc("PdhCollectQueryData")
	procPdhCloseQuery               = modPdhDll.NewProc("PdhCloseQuery")
	procPdhOpenQuery                = modPdhDll.NewProc("PdhOpenQuery")
	procPdhRemoveCounter            = modPdhDll.NewProc("PdhRemoveCounter")
	procPdhGetFormattedCounterArray = modPdhDll.NewProc("PdhGetFormattedCounterArrayW")
)

var (
	counterToIndex map[string][]int
)

const (
	// taken from winperf.h
	PERF_DETAIL_NOVICE   = 100 // The uninformed can understand it
	PERF_DETAIL_ADVANCED = 200 // For the advanced user
	PERF_DETAIL_EXPERT   = 300 // For the expert user
	PERF_DETAIL_WIZARD   = 400 // For the system designer
)

func pdhLookupPerfNameByIndex(ndx int) (string, error) {
	var len uint32
	var name string
	r, _, _ := procPdhLookupPerfNameByIndex.Call(uintptr(0), // machine name, for now always local
		uintptr(ndx),
		uintptr(0),
		uintptr(unsafe.Pointer(&len)))

	if r != PDH_MORE_DATA {
		log.Errorf("Failed to look up Windows performance counter (looking for index %d)", ndx)
		log.Errorf("This error indicates that the Windows performance counter database may need to be rebuilt")
		return name, fmt.Errorf("Failed to get buffer size (%#x)", r)
	}
	buf := make([]uint16, len)
	r, _, _ = procPdhLookupPerfNameByIndex.Call(uintptr(0), // machine name, for now always local
		uintptr(ndx),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&len)))

	if r != ERROR_SUCCESS {
		return name, fmt.Errorf("Error getting perf name for index %d (%#x)", ndx, r)
	}
	name = windows.UTF16ToString(buf)
	return name, nil
}

// TODO: configurable?
const (
	PDH_REFRESH_INTERVAL = 60 // in seconds
)
// Lock enforces no more than once forceRefresh=false
// is running concurrently
var lock_lastPdhRefreshTime sync.Mutex
// tracks last time a refresh was successful
// initialize with process init time as that is when
// the PDH object cache is implicitly created/refreshed.
var lastPdhRefreshTime = atomic.NewTime(time.Now())
func refreshPdhObjectCache(forceRefresh bool) (didrefresh bool, err error) {
	// Refresh the Windows internal PDH Object cache
	//
	// When forceRefresh=false, the cache is refreshed no more frequently
	// than the PDH_REFRESH_INTERVAL.
	//
	// forceRefresh - If true, ignore PDH_REFRESH_INTERVAL and refresh anyway
	//
	// returns didrefresh=true if the refresh operation was successful
	//

	var len uint32

	// Only refresh at most every PDH_REFRESH_INTERVAL seconds
	// or when forceRefresh=true
	if !forceRefresh {
		// TODO: use TryLock in golang 1.18
		//       we don't need to block here
		//       worst case the counter is skipped again until next interval.
		lock_lastPdhRefreshTime.Lock()
		defer lock_lastPdhRefreshTime.Unlock()
		timenow := time.Now()
		// time.Time.Sub() uses a monotonic clock
		if timenow.Sub(lastPdhRefreshTime.Load()).Seconds() < PDH_REFRESH_INTERVAL {
			// too soon, skip refresh
			return false, nil
		}
	}

	// do the refresh
	// either forceRefresh=true
	// or the interval expired and lock is held

	log.Infof("Refreshing performance counters")
	r, _, _ := procPdhEnumObjects.Call(
		uintptr(0), // NULL data source, use computer in szMachineName parameter
		uintptr(0), // NULL use local computer
		uintptr(0), // NULL don't return output
		uintptr(unsafe.Pointer(&len)), // output size
		uintptr(PERF_DETAIL_WIZARD),
		uintptr(1)) // do refresh
	if r != PDH_MORE_DATA {
		e := fmt.Sprintf("Failed to refresh performance counters (%#x)", r)
		log.Errorf(e)
		return false, fmt.Errorf(e)
	}

	// refresh successful
	log.Infof("Successfully refreshed performance counters!")
	// update time
	lastPdhRefreshTime.Store(time.Now())
	return true, nil
}
func forceRefreshPdhObjectCache() (didrefresh bool, err error) {
	// Refresh the Windows internal PDH Object cache
	// see refreshPdhObjectCache() for details
	return refreshPdhObjectCache(true)
}
func tryRefreshPdhObjectCache() (didrefresh bool, err error) {
	// Attempt to refresh the Windows internal PDH Object cache
	// may be skipped if cache was refreshed recently.
	// see refreshPdhObjectCache() for details
	return refreshPdhObjectCache(false)
}

func pdhEnumObjectItems(className string) (counters []string, instances []string, err error) {
	var counterlen uint32
	var instancelen uint32

	if counterlen != 0 || instancelen != 0 {
		log.Errorf("invalid parameter %v %v", counterlen, instancelen)
		counterlen = 0
		instancelen = 0
	}
	r, _, _ := procPdhEnumObjectItems.Call(
		uintptr(0), // NULL data source, use computer in computername parameter
		uintptr(0), // local computer
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(className))),
		uintptr(0), // empty list, for now
		uintptr(unsafe.Pointer(&counterlen)),
		uintptr(0), // empty instance list
		uintptr(unsafe.Pointer(&instancelen)),
		uintptr(PERF_DETAIL_WIZARD),
		uintptr(0))
	if r != PDH_MORE_DATA {
		log.Errorf("Failed to enumerate windows performance counters (%#x) (class %s)", r, className)
		log.Errorf("This error indicates that the Windows performance counter database may need to be rebuilt")
		if r == PDH_CSTATUS_NO_OBJECT {
			return nil, nil, fmt.Errorf("Object not found (%#x) (class %v)", r, className)
		} else {
			return nil, nil, fmt.Errorf("Failed to get buffer size (%#x)", r)
		}
	}
	counterbuf := make([]uint16, counterlen)
	var instanceptr uintptr
	var instancebuf []uint16

	if instancelen != 0 {
		instancebuf = make([]uint16, instancelen)
		instanceptr = uintptr(unsafe.Pointer(&instancebuf[0]))
	}
	r, _, _ = procPdhEnumObjectItems.Call(
		uintptr(0), // NULL data source, use computer in computername parameter
		uintptr(0), // local computer
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(className))),
		uintptr(unsafe.Pointer(&counterbuf[0])),
		uintptr(unsafe.Pointer(&counterlen)),
		instanceptr,
		uintptr(unsafe.Pointer(&instancelen)),
		uintptr(PERF_DETAIL_WIZARD),
		uintptr(0))
	if r != ERROR_SUCCESS {
		err = fmt.Errorf("Error getting counter items (%#x)", r)
		return
	}
	counters = winutil.ConvertWindowsStringList(counterbuf)
	instances = winutil.ConvertWindowsStringList(instancebuf)
	err = nil
	return

}

type pdh_counter_path_elements struct {
	ptrmachineString  uintptr
	ptrobjectString   uintptr
	ptrinstanceString uintptr
	ptrparentString   uintptr
	instanceIndex     uint32
	countername       uintptr
}

type ErrPdhInvalidInstance struct {
	message string
}

func NewErrPdhInvalidInstance(message string) *ErrPdhInvalidInstance {
	return &ErrPdhInvalidInstance{
		message: message,
	}
}

func (e *ErrPdhInvalidInstance) Error() string {
	return e.message
}
func pdhMakeCounterPath(machine string, object string, instance string, counter string) (path string, err error) {
	var elems pdh_counter_path_elements

	if machine != "" {
		elems.ptrmachineString = uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(machine)))
	}
	if object != "" {
		elems.ptrobjectString = uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(object)))
	}
	if instance != "" {
		elems.ptrinstanceString = uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(instance)))
	}
	if counter != "" {
		elems.countername = uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(counter)))
	}
	var len uint32
	r, _, _ := procPdhMakeCounterPath.Call(
		uintptr(unsafe.Pointer(&elems)),
		uintptr(0),
		uintptr(unsafe.Pointer(&len)),
		uintptr(0))
	if r != PDH_MORE_DATA {
		log.Errorf("Failed to make Windows performance counter (%s %s %s %s)", machine, object, instance, counter)
		log.Errorf("This error indicates that the Windows performance counter database may need to be rebuilt")
		err = fmt.Errorf("Failed to get buffer size (%#x)", r)
		return
	}
	buf := make([]uint16, len)
	r, _, _ = procPdhMakeCounterPath.Call(
		uintptr(unsafe.Pointer(&elems)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&len)),
		uintptr(0))
	if r != ERROR_SUCCESS {
		err = fmt.Errorf("Failed to get path (%#x)", r)
		return
	}
	path = windows.UTF16ToString(buf)
	return

}

func pdhGetFormattedCounterValueLarge(hCounter PDH_HCOUNTER) (val int64, err error) {
	var lpdwType uint32
	var pValue PDH_FMT_COUNTERVALUE_LARGE

	ret, _, _ := procPdhGetFormattedCounterValue.Call(
		uintptr(hCounter),
		uintptr(PDH_FMT_LARGE),
		uintptr(unsafe.Pointer(&lpdwType)),
		uintptr(unsafe.Pointer(&pValue)))
	if ERROR_SUCCESS != ret {
		if ret == PDH_INVALID_DATA && pValue.CStatus == PDH_CSTATUS_NO_INSTANCE {
			return 0, NewErrPdhInvalidInstance("Invalid counter instance")
		}
		return 0, fmt.Errorf("Error retrieving large value %#x %#x", ret, pValue.CStatus)
	}

	return pValue.LargeValue, nil
}

func pdhGetFormattedCounterValueFloat(hCounter PDH_HCOUNTER) (val float64, err error) {
	var lpdwType uint32
	var pValue PDH_FMT_COUNTERVALUE_DOUBLE

	ret, _, _ := procPdhGetFormattedCounterValue.Call(
		uintptr(hCounter),
		uintptr(PDH_FMT_DOUBLE),
		uintptr(unsafe.Pointer(&lpdwType)),
		uintptr(unsafe.Pointer(&pValue)))
	if ERROR_SUCCESS != ret {
		if ret == PDH_INVALID_DATA && pValue.CStatus == PDH_CSTATUS_NO_INSTANCE {
			return 0, NewErrPdhInvalidInstance("Invalid counter instance")
		}
		return 0, fmt.Errorf("Error retrieving float value %#x %#x", ret, pValue.CStatus)
	}

	return pValue.DoubleValue, nil
}

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
		regerr := windows.RegQueryValueEx(windows.HKEY_PERFORMANCE_DATA,
			windows.StringToUTF16Ptr("Counter 009"),
			nil, // reserved
			&regtype,
			(*byte)(unsafe.Pointer(&counterlist[0])),
			&sz)
		if regerr == error(windows.ERROR_MORE_DATA) {
			// buffer's not big enough
			bufferSize += bufferIncrement
			continue
		} else if regerr != nil {
			return regerr
		}
		// must set the length of the slice to the actual amount of data
		// sz is in bytes, but it's a slice of uint16s, so divide the returned
		// buffer size by two.

		counterlist = counterlist[:(sz / 2)]
		break
	}
	clist := winutil.ConvertWindowsStringList(counterlist)
	for i := 0; (i + 1) < len(clist); i += 2 {
		ndx, _ := strconv.Atoi(clist[i])
		counterToIndex[clist[i+1]] = append(counterToIndex[clist[i+1]], ndx)
	}
	return nil
}
