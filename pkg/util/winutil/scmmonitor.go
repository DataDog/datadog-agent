// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package winutil

import (
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ServiceInfo contains name information for each service identified by PID
type ServiceInfo struct {
	ServiceName []string
	DisplayName []string
}

// ServiceList is the return value from a query by pid.
type ServiceList struct {
	essp      []*windows.ENUM_SERVICE_STATUS_PROCESS
	startTime uint64
}

// SCMMonitor is an object that allows the caller to monitor Windows services.
// The object will maintain a table of active services indexed by PID
type SCMMonitor struct {
	mux              sync.Mutex
	pidToService     map[uint64]*ServiceList
	nonServicePid    map[uint64]uint64 // start time of this pid
	lastMapTime      uint64            // ns since jan1 1970
	serviceRefreshes uint64
}

// GetServiceMonitor returns a service monitor object
func GetServiceMonitor() *SCMMonitor {
	return &SCMMonitor{
		pidToService:  make(map[uint64]*ServiceList),
		nonServicePid: make(map[uint64]uint64),
	}
}

type startTimeFunc func(uint64) (uint64, error)

var pGetProcessStartTimeAsNs = startTimeFunc(getProcessStartTimeAsNs)

// GetRefreshCount returns the number of times we've actually queried
// the SCM database.  used for logging stats.
func (scm *SCMMonitor) GetRefreshCount() uint64 {
	return scm.serviceRefreshes
}
func (scm *SCMMonitor) refreshCache() error {

	// EnumServiceStatusEx requires only SC_MANAGER_ENUM_SERVICE.  Switch to
	// new library to use least privilege
	h, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_ENUMERATE_SERVICE)
	if err != nil {
		log.Warnf("Failed to connect to scm %v", err)
		return fmt.Errorf("Failed to open SCM %v", err)
	}
	m := &mgr.Mgr{Handle: h}
	defer m.Disconnect()

	var bytesNeeded, servicesReturned uint32
	var buf []byte
	for {
		var p *byte
		if len(buf) > 0 {
			p = &buf[0]
		}
		err = windows.EnumServicesStatusEx(m.Handle, windows.SC_ENUM_PROCESS_INFO,
			windows.SERVICE_WIN32, windows.SERVICE_STATE_ALL,
			p, uint32(len(buf)), &bytesNeeded, &servicesReturned, nil, nil)
		if err == nil {
			break
		}
		if err != windows.ERROR_MORE_DATA {
			return fmt.Errorf("Failed to enum services %v", err)
		}
		if bytesNeeded <= uint32(len(buf)) {
			return err
		}
		buf = make([]byte, bytesNeeded)
	}
	if servicesReturned == 0 {
		return nil
	}

	services := unsafe.Slice((*windows.ENUM_SERVICE_STATUS_PROCESS)(unsafe.Pointer(&buf[0])), servicesReturned)

	newmap := make(map[uint64]*ServiceList)
	for idx, svc := range services {
		var thissvc *ServiceList
		var ok bool
		if thissvc, ok = newmap[uint64(svc.ServiceStatusProcess.ProcessId)]; !ok {
			thissvc = &ServiceList{}
			newmap[uint64(svc.ServiceStatusProcess.ProcessId)] = thissvc
		}
		thissvc.essp = append(thissvc.essp, &services[idx])
	}
	var current windows.Filetime
	windows.GetSystemTimeAsFileTime(&current)
	scm.lastMapTime = uint64(current.Nanoseconds())
	scm.pidToService = newmap
	scm.serviceRefreshes++
	return nil
}

func (s *ServiceList) toServiceInfo() *ServiceInfo {
	var si ServiceInfo
	for _, inf := range s.essp {
		si.DisplayName = append(si.DisplayName, windows.UTF16PtrToString(inf.DisplayName))
		si.ServiceName = append(si.ServiceName, windows.UTF16PtrToString(inf.ServiceName))
	}
	return &si

}

func (scm *SCMMonitor) getServiceFromCache(pid, pidstart uint64) (*ServiceInfo, error) {
	var err error
	if val, ok := scm.pidToService[pid]; ok {
		// it was in there.  see if it's the right one
		if val.startTime == 0 {
			// we don't prepopulate the cache with pid start times
			val.startTime, err = pGetProcessStartTimeAsNs(pid)
			if err != nil {
				return nil, err
			}
		}
		if val.startTime == pidstart {
			// it's the same one.
			return val.toServiceInfo(), nil
		}
	}
	return nil, nil
}

// GetServiceInfo gets the service name and display name if the process identified
// by the pid is in the SCM.  A process which is not an SCM controlled service will
// return nil with no error
func (scm *SCMMonitor) GetServiceInfo(pid uint64) (*ServiceInfo, error) {
	// get the process start time of the pid being checked
	pidstart, err := pGetProcessStartTimeAsNs(pid)
	if err != nil {
		return nil, err
	}
	scm.mux.Lock()
	defer scm.mux.Unlock()
	// check to see if the pid is in the cache of known, not service pids
	if val, ok := scm.nonServicePid[pid]; ok {
		// it's a known non service pid.  Make sure it's not recycled.
		if val == pidstart {
			// it's the same process.  We know this isn't a service
			return nil, nil
		}
		// it was in there but the times didn't match, which means
		// it's a different process.  Clean it out.
		delete(scm.nonServicePid, pid)
	}
	// if we get here it either wasn't in the map, or it was but the
	// start time didn't match

	if pidstart <= scm.lastMapTime {
		// so it's been around longer than our last check of the service
		// table.  so if it's in there it's probably good.
		si, err := scm.getServiceFromCache(pid, pidstart)
		if err != nil {
			return nil, err
		}
		if si != nil {
			return si, nil
		}
		// else
		scm.nonServicePid[pid] = pidstart
		return nil, nil

	}

	// if we get here, the process
	// is newer than the service map.
	if err = scm.refreshCache(); err != nil {
		return nil, err
	}
	// now check the service map
	si, err := scm.getServiceFromCache(pid, pidstart)
	if err != nil {
		return nil, err
	}
	if si != nil {
		return si, nil
	}
	// otherwise put this pid as a known, non service
	scm.nonServicePid[pid] = pidstart
	return nil, nil
}
