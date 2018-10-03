// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.
// +build windows

package pdhutil

import (
	"fmt"
	"syscall"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

var (
	modPdhDll = syscall.NewLazyDLL("pdh.dll")

	procPdhLookupPerfNameByIndex    = modPdhDll.NewProc("PdhLookupPerfNameByIndexW")
	procPdhEnumObjectItems          = modPdhDll.NewProc("PdhEnumObjectItemsW")
	procPdhMakeCounterPath          = modPdhDll.NewProc("PdhMakeCounterPathW")
	procPdhGetFormattedCounterValue = modPdhDll.NewProc("PdhGetFormattedCounterValue")
	procPdhAddCounterW              = modPdhDll.NewProc("PdhAddCounterW")
	procPdhCollectQueryData         = modPdhDll.NewProc("PdhCollectQueryData")
	procPdhCloseQuery               = modPdhDll.NewProc("PdhCloseQuery")
	procPdhOpenQuery                = modPdhDll.NewProc("PdhOpenQuery")
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
		return name, fmt.Errorf("Failed to get buffer size %v", r)
	}
	buf := make([]uint16, len)
	r, _, _ = procPdhLookupPerfNameByIndex.Call(uintptr(0), // machine name, for now always local
		uintptr(ndx),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&len)))

	if r != ERROR_SUCCESS {
		return name, fmt.Errorf("Error getting perf name for index %d %v", ndx, r)
	}
	name = syscall.UTF16ToString(buf)
	return name, nil
}

func pdhEnumObjectItems(className string) (counters []string, instances []string, err error) {
	var counterlen uint32
	var instancelen uint32
	r, _, _ := procPdhEnumObjectItems.Call(
		uintptr(0), // NULL data source, use computer in computername parameter
		uintptr(0), // local computer
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(className))),
		uintptr(0), // empty list, for now
		uintptr(unsafe.Pointer(&counterlen)),
		uintptr(0), // empty instance list
		uintptr(unsafe.Pointer(&instancelen)),
		uintptr(PERF_DETAIL_WIZARD),
		uintptr(0))
	if r != PDH_MORE_DATA {
		log.Errorf("Failed to enumerage windows performance counters (class %s)", className)
		log.Errorf("This error indicates that the Windows performance counter database may need to be rebuilt")
		return nil, nil, fmt.Errorf("Failed to get buffer size %v", r)
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
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(className))),
		uintptr(unsafe.Pointer(&counterbuf[0])),
		uintptr(unsafe.Pointer(&counterlen)),
		instanceptr,
		uintptr(unsafe.Pointer(&instancelen)),
		uintptr(PERF_DETAIL_WIZARD),
		uintptr(0))
	if r != ERROR_SUCCESS {
		err = fmt.Errorf("Error getting counter items %v", r)
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

func pdhMakeCounterPath(machine string, object string, instance string, counter string) (path string, err error) {
	var elems pdh_counter_path_elements

	if machine != "" {
		elems.ptrmachineString = uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(machine)))
	}
	if object != "" {
		elems.ptrobjectString = uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(object)))
	}
	if instance != "" {
		elems.ptrinstanceString = uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(instance)))
	}
	if counter != "" {
		elems.countername = uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(counter)))
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
		err = fmt.Errorf("Failed to get buffer size %v", r)
		return
	}
	buf := make([]uint16, len)
	r, _, _ = procPdhMakeCounterPath.Call(
		uintptr(unsafe.Pointer(&elems)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&len)),
		uintptr(0))
	if r != ERROR_SUCCESS {
		err = fmt.Errorf("Failed to get path %v", r)
		return
	}
	path = syscall.UTF16ToString(buf)
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
		return 0, fmt.Errorf("Error retrieving large value %v", ret)
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
		return 0, fmt.Errorf("Error retrieving large value %v", ret)
	}

	return pValue.DoubleValue, nil
}
