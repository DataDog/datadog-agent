// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package pdhutil

import (
	"fmt"
	"reflect"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"go.uber.org/atomic"
	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	modPdhDll = windows.NewLazyDLL("pdh.dll")

	procPdhEnumObjects              = modPdhDll.NewProc("PdhEnumObjectsW")
	procPdhMakeCounterPath          = modPdhDll.NewProc("PdhMakeCounterPathW")
	procPdhGetFormattedCounterValue = modPdhDll.NewProc("PdhGetFormattedCounterValue")
	procPdhAddEnglishCounterW       = modPdhDll.NewProc("PdhAddEnglishCounterW")
	procPdhCollectQueryData         = modPdhDll.NewProc("PdhCollectQueryData")
	procPdhCloseQuery               = modPdhDll.NewProc("PdhCloseQuery")
	procPdhOpenQuery                = modPdhDll.NewProc("PdhOpenQuery")
	procPdhRemoveCounter            = modPdhDll.NewProc("PdhRemoveCounter")
	procPdhGetFormattedCounterArray = modPdhDll.NewProc("PdhGetFormattedCounterArrayW")
)

const (
	// taken from winperf.h
	PERF_DETAIL_NOVICE   = 100 // The uninformed can understand it
	PERF_DETAIL_ADVANCED = 200 // For the advanced user
	PERF_DETAIL_EXPERT   = 300 // For the expert user
	PERF_DETAIL_WIZARD   = 400 // For the system designer
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
	// than the refresh interval. The refresh interval is controlled by the
	// config option 'windows_counter_refresh_interval'.
	//
	// forceRefresh - If true, ignore the refresh interval and refresh anyway
	//
	// returns didrefresh=true if the refresh operation was successful
	//
	// Background information for 'windows_counter_refresh_interval' config
	// --------------------------------------------------------------------
	// Controls the minimum number of seconds that must have elapsed before the
	// cached performance counters objects list can be refreshed again. Starting
	// in 7.40 it had been used for all Windows Performance Counters based
	// checks (all the time for Python checks and only to initialize Go checks).
	// Starting from 7.42 it is not used anymore for Python Performance Counters
	// based checks due to its side effects (for details see integrations-core PR
	// https://github.com/DataDog/integrations-core/pull/13243). It is still used
	// for Go Performance Counters based checks because it may be useful in rare
	// cases when their initialization fails.
	//
	// A higher value will have lower performance impact and create fewer perflib
	// Windows Event log errors and warnings but will increase the time before the
	// failing counters have a chance to initialize successfully. A lower value
	// will have higher performance impact and create more perflib Windows Event
	// log errors and warnings but will decrease the time before the failing
	// counters have a chance to initialize successfully.

	var len uint32

	refresh_interval := config.Datadog.GetInt("windows_counter_refresh_interval")
	if refresh_interval == 0 {
		// refresh disabled
		return false, nil
	} else if refresh_interval < 0 {
		// invalid value
		e := fmt.Sprintf("windows_counter_refresh_interval cannot be a negative number")
		log.Errorf(e)
		return false, fmt.Errorf(e)
	}

	// Only refresh at most every refresh_interval seconds
	// or when forceRefresh=true
	if !forceRefresh {
		// TODO: use TryLock in golang 1.18
		//       we don't need to block here
		//       worst case the counter is skipped again until next interval.
		lock_lastPdhRefreshTime.Lock()
		defer lock_lastPdhRefreshTime.Unlock()
		timenow := time.Now()
		// time.Time.Sub() uses a monotonic clock
		if int(timenow.Sub(lastPdhRefreshTime.Load()).Seconds()) < refresh_interval {
			// too soon, skip refresh
			return false, nil
		}
	}

	// do the refresh
	// either forceRefresh=true
	// or the interval expired and lock is held

	log.Infof("Refreshing performance counters")
	r, _, _ := procPdhEnumObjects.Call(
		uintptr(0),                    // NULL data source, use computer in szMachineName parameter
		uintptr(0),                    // NULL use local computer
		uintptr(0),                    // NULL don't return output
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
func tryRefreshPdhObjectCache() (didrefresh bool, err error) {
	// Attempt to refresh the Windows internal PDH Object cache
	// may be skipped if cache was refreshed recently.
	// see refreshPdhObjectCache() for details
	return refreshPdhObjectCache(false)
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

// Enum enumerates performance counter values for a wildcard instance counter (e.g. `\Process(*)\% Processor Time`)
//
// Will append '#<INDEX>' to duplicate instance names to ensure their uniqueness.
// Instance uniqueness is normally done by the PDH provider, except in the case of the Process class where it is NOT
// handled in order to maintain backwards compatibility.
// https://learn.microsoft.com/en-us/windows/win32/perfctrs/handling-duplicate-instance-names
func pdhGetFormattedCounterArray(hCounter PDH_HCOUNTER, format uint32) (out_items []PdhCounterValueItem, err error) {
	var buf []uint8
	var bufLen uint32
	var itemCount uint32

	if format == PDH_FMT_DOUBLE {
		format |= PDH_FMT_NOCAP100
	}

	r, _, _ := procPdhGetFormattedCounterArray.Call(
		uintptr(hCounter),
		uintptr(format),
		uintptr(unsafe.Pointer(&bufLen)),
		uintptr(unsafe.Pointer(&itemCount)),
		uintptr(0),
	)

	if r != PDH_MORE_DATA {
		return nil, fmt.Errorf("Failed to get formatted counter array buffer size %#x", r)
	}

	buf = make([]uint8, bufLen)

	r, _, _ = procPdhGetFormattedCounterArray.Call(
		uintptr(hCounter),
		uintptr(format),
		uintptr(unsafe.Pointer(&bufLen)),
		uintptr(unsafe.Pointer(&itemCount)),
		uintptr(unsafe.Pointer(&buf[0])),
	)
	if r != ERROR_SUCCESS {
		return nil, fmt.Errorf("Error getting formatted counter array %#x", r)
	}

	var items []PDH_FMT_COUNTERVALUE_ITEM_DOUBLE
	// Accessing the `SliceHeader` to manipulate the `items` slice
	// In the future we can use unsafe.Slice instead https://pkg.go.dev/unsafe@master#Slice
	hdrItems := (*reflect.SliceHeader)(unsafe.Pointer(&items))
	hdrItems.Data = uintptr(unsafe.Pointer(&buf[0]))
	hdrItems.Len = int(itemCount)
	hdrItems.Cap = int(itemCount)

	var (
		prevName    string
		instanceIdx int
	)

	// Instance names are packed in the buffer following the items structs
	strBufLen := int(bufLen - uint32(unsafe.Sizeof(PDH_FMT_COUNTERVALUE_ITEM_DOUBLE{}))*itemCount)
	for _, item := range items {
		var u []uint16

		// Accessing the `SliceHeader` to manipulate the `u` slice
		hdrU := (*reflect.SliceHeader)(unsafe.Pointer(&u))
		hdrU.Data = uintptr(unsafe.Pointer(item.szName))
		hdrU.Len = strBufLen / 2
		hdrU.Cap = strBufLen / 2

		// Scan for terminating NUL char
		for i, v := range u {
			if v == 0 {
				u = u[:i]
				// subtract from the instance names buffer space
				strBufLen -= (i + 1) * 2 // in bytes including terminating NUL char
				break
			}
		}

		name := windows.UTF16ToString(u)
		if name != prevName {
			instanceIdx = 0
			prevName = name
		} else {
			instanceIdx++
		}

		instance := name
		if instanceIdx != 0 {
			// To match same instance ID as in perfmon on Windows
			instance += "#" + strconv.Itoa(instanceIdx)
		}

		var value PdhCounterValue
		value.CStatus = item.value.CStatus

		switch format {
		case PDH_FMT_DOUBLE:
		case PDH_FMT_DOUBLE | PDH_FMT_NOCAP100:
			value.Double = item.value.DoubleValue
		case PDH_FMT_LONG:
			from := (*PDH_FMT_COUNTERVALUE_ITEM_LONG)(unsafe.Pointer(&item))
			value.Long = from.value.LongValue
		case PDH_FMT_LARGE:
			from := (*PDH_FMT_COUNTERVALUE_ITEM_LARGE)(unsafe.Pointer(&item))
			value.Large = from.value.LargeValue
		}

		value_item := PdhCounterValueItem{
			instance: instance,
			value:    value,
		}
		out_items = append(out_items, value_item)
	}
	return out_items, nil
}
