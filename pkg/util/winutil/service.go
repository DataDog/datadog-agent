// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package winutil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultServiceCommandTimeout = 10
)

var (
	modadvapi32               = windows.NewLazyDLL("advapi32.dll")
	procEnumDependentServices = modadvapi32.NewProc("EnumDependentServicesW")
)

type enumServiceState uint32

func OpenSCManager(desiredAccess uint32) (*mgr.Mgr, error) {
	h, err := windows.OpenSCManager(nil, nil, desiredAccess)
	if h == 0 {
		return nil, err
	}
	return &mgr.Mgr{Handle: h}, err
}

func OpenService(manager *mgr.Mgr, serviceName string, desiredAccess uint32) (*mgr.Service, error) {
	h, err := windows.OpenService(manager.Handle, windows.StringToUTF16Ptr(serviceName), desiredAccess)
	if err != nil {
		return nil, err
	}
	return &mgr.Service{Name: serviceName, Handle: h}, err
}

func StartService(serviceName string) error {

	m, err := OpenSCManager(windows.SC_MANAGER_CONNECT)
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := OpenService(m, serviceName, windows.SERVICE_START)
	if err != nil {
		return fmt.Errorf("could not open service: %v", err)
	}
	defer s.Close()

	err = s.Start()
	if err != nil {
		return fmt.Errorf("could not start service: %v", err)
	}
	return nil
}

func ControlService(serviceName string, command svc.Cmd, to svc.State, desiredAccess uint32, timeout int64) error {

	m, err := OpenSCManager(windows.SC_MANAGER_CONNECT)
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := OpenService(m, serviceName, desiredAccess)
	if err != nil {
		return fmt.Errorf("could not open service: %v", err)
	}
	defer s.Close()

	status, err := s.Control(command)
	if err != nil {
		return fmt.Errorf("could not send control=%d: %v", svc.Stop, err)
	}

	timesup := time.Now().Add(time.Duration(timeout) * time.Second)
	for status.State != to {
		if time.Now().After(timesup) {
			return fmt.Errorf("timeout waiting for service to go to state=%d", to)
		}
		time.Sleep(300 * time.Millisecond)
		status, err = s.Query()
		if err != nil {
			return fmt.Errorf("could not retrieve service status: %v", err)
		}
	}
	return nil
}

func StopService(serviceName string) error {

	deps, err := ListDependentServices(serviceName, windows.SERVICE_ACTIVE)
	if err != nil {
		return fmt.Errorf("could not list dependent services")
	}

	for _, dep := range deps {
		err = StopService(dep.serviceName)
		if err != nil {
			return fmt.Errorf("could not stop service")
		}
	}

	return ControlService(serviceName, svc.Stop, svc.Stopped, windows.SERVICE_STOP|windows.SERVICE_QUERY_STATUS, defaultServiceCommandTimeout)
}

func RestartService(serviceName string) error {
	var err error
	if err = StopService(serviceName); err == nil {
		err = StartService(serviceName)
	}
	return err
}

// when Go has their version, replace ours with the upstream
func ListDependentServices(serviceName string, state enumServiceState) ([]EnumServiceStatus, error) {
	m, err := OpenSCManager(windows.SC_MANAGER_CONNECT)
	if err != nil {
		return nil, err
	}
	defer m.Disconnect()

	s, err := OpenService(m, serviceName, windows.SERVICE_ENUMERATE_DEPENDENTS)
	if err != nil {
		return nil, fmt.Errorf("could not open service: %v", err)
	}
	defer s.Close()

	deps, err := enumDependentServices(s.Handle, state)
	if err != nil {
		return nil, fmt.Errorf("could not enumerate dependent services: %v", err)
	}
	return deps, nil
}

// ServiceStatus reports information pertaining to enumerated services
// only exported so binary.Read works
type ServiceStatus struct {
	DwServiceType             uint32
	DwCurrentState            uint32
	DwControlsAccepted        uint32
	DwWin32ExitCode           uint32
	DwServiceSpecificExitCode uint32
	DwCheckPoint              uint32
	DwWaitHint                uint32
}

// EnumServiceStatus complete enumerated service information
// only exported so binary.Read works
type EnumServiceStatus struct {
	serviceName string
	displayName string
	status      ServiceStatus
}

type internalEnumServiceStatus struct {
	ServiceName uint64 // offset from beginning of buffer
	DisplayName uint64 // offset from beginning of buffer.
	Status      ServiceStatus
	Padding     uint32 // structure is qword aligned.

}

func enumDependentServices(h windows.Handle, state enumServiceState) (services []EnumServiceStatus, err error) {
	services = make([]EnumServiceStatus, 0)
	var bufsz uint32
	var count uint32
	_, _, err = procEnumDependentServices.Call(uintptr(h),
		uintptr(state),
		uintptr(0),
		uintptr(0), // current buffer size is zero
		uintptr(unsafe.Pointer(&bufsz)),
		uintptr(unsafe.Pointer(&count)))

	// success with a 0 buffer means no dependent services
	if err == error(windows.ERROR_SUCCESS) {
		err = nil
		return
	} else {
		if err != error(windows.ERROR_MORE_DATA) {
			log.Warnf("Error getting buffer %v", err)
			return
		}
	}

	servicearray := make([]uint8, bufsz) // TODO convert to uint16 and bufsz/2 to allow windows.UTF16toString call
	ret, _, err := procEnumDependentServices.Call(uintptr(h),
		uintptr(state),
		uintptr(unsafe.Pointer(&servicearray[0])),
		uintptr(bufsz),
		uintptr(unsafe.Pointer(&bufsz)),
		uintptr(unsafe.Pointer(&count)))
	if ret == 0 {
		log.Warnf("Error getting deps %d %v", int(ret), err)
		return
	}
	// now get to parse out the C structure into go.
	var ess internalEnumServiceStatus
	baseptr := uintptr(unsafe.Pointer(&servicearray[0]))
	buf := bytes.NewReader(servicearray)
	for i := uint32(0); i < count; i++ {

		err = binary.Read(buf, binary.LittleEndian, &ess)
		if err != nil {
			break
		}

		ess.ServiceName = ess.ServiceName - uint64(baseptr)
		ess.DisplayName = ess.DisplayName - uint64(baseptr)
		ss := EnumServiceStatus{serviceName: ConvertWindowsString(servicearray[ess.ServiceName:]),
			displayName: ConvertWindowsString(servicearray[ess.DisplayName:]),
			status:      ess.Status}
		services = append(services, ss)
	}
	return
}
