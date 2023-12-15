// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package winutil

import (
	"bytes"
	"context"
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

// OpenSCManager connects to SCM
//
// https://learn.microsoft.com/en-us/windows/win32/api/winsvc/nf-winsvc-openscmanagerw
func OpenSCManager(desiredAccess uint32) (*mgr.Mgr, error) {
	h, err := windows.OpenSCManager(nil, nil, desiredAccess)
	if err != nil {
		return nil, err
	}
	return &mgr.Mgr{Handle: h}, nil
}

// OpenService opens a handle for serviceName
//
// https://learn.microsoft.com/en-us/windows/win32/api/winsvc/nf-winsvc-openservicew
func OpenService(manager *mgr.Mgr, serviceName string, desiredAccess uint32) (*mgr.Service, error) {
	h, err := windows.OpenService(manager.Handle, windows.StringToUTF16Ptr(serviceName), desiredAccess)
	if err != nil {
		return nil, err
	}
	return &mgr.Service{Name: serviceName, Handle: h}, nil
}

// StartService starts serviceName via SCM.
//
// Does not block until service is started
// https://learn.microsoft.com/en-us/windows/win32/api/winsvc/nf-winsvc-startservicea#remarks
func StartService(serviceName string, serviceArgs ...string) error {

	manager, err := OpenSCManager(windows.SC_MANAGER_CONNECT)
	if err != nil {
		return fmt.Errorf("could not open SCM: %v", err)
	}
	defer manager.Disconnect()

	service, err := OpenService(manager, serviceName, windows.SERVICE_START)
	if err != nil {
		return fmt.Errorf("could not open service %s: %v", serviceName, err)
	}
	defer service.Close()

	err = service.Start(serviceArgs...)
	if err != nil {
		return fmt.Errorf("could not start service %s: %v", serviceName, err)
	}
	return nil
}

// ControlService sends a control code to a specified service and waits up to
// timeout for the service to transition to the requested state
//
// https://learn.microsoft.com/en-us/windows/win32/api/winsvc/nf-winsvc-controlservice
//
//revive:disable-next-line:var-naming Name is intended to match the Windows API name
func ControlService(serviceName string, command svc.Cmd, to svc.State, desiredAccess uint32, timeout int64) error {

	manager, err := OpenSCManager(windows.SC_MANAGER_CONNECT)
	if err != nil {
		return fmt.Errorf("could not open SCM: %v", err)
	}
	defer manager.Disconnect()

	service, err := OpenService(manager, serviceName, desiredAccess)
	if err != nil {
		return fmt.Errorf("could not open service %s: %v", serviceName, err)
	}
	defer service.Close()

	status, err := service.Control(command)
	if err != nil {
		return fmt.Errorf("could not send control %d: %v", command, err)
	}

	timesup := time.Now().Add(time.Duration(timeout) * time.Second)
	for status.State != to {
		if time.Now().After(timesup) {
			return fmt.Errorf("timeout waiting for service %s to go to state %d; current state: %d", serviceName, to, status.State)
		}
		time.Sleep(300 * time.Millisecond)
		status, err = service.Query()
		if err != nil {
			return fmt.Errorf("could not retrieve status for %s: %v", serviceName, err)
		}
	}
	return nil
}

func doStopService(serviceName string) error {
	return ControlService(serviceName, svc.Stop, svc.Stopped, windows.SERVICE_STOP|windows.SERVICE_QUERY_STATUS, defaultServiceCommandTimeout)
}

// StopService stops a service and any services that depend on it
func StopService(serviceName string) error {

	deps, err := ListDependentServices(serviceName, windows.SERVICE_ACTIVE)
	if err != nil {
		return fmt.Errorf("could not list dependent services for %s: %v", serviceName, err)
	}

	for _, dep := range deps {
		err = doStopService(dep.serviceName)
		if err != nil {
			return fmt.Errorf("could not stop service %s: %v", dep.serviceName, err)
		}
	}
	return doStopService(serviceName)
}

// WaitForState waits for the service to become the desired state. A timeout can be specified
// with a context. Returns nil if/when the service becomes the desired state.
func WaitForState(ctx context.Context, serviceName string, desiredState svc.State) error {
	// open handle to service
	manager, err := OpenSCManager(windows.SC_MANAGER_CONNECT)
	if err != nil {
		return fmt.Errorf("could not open SCM: %v", err)
	}
	defer manager.Disconnect()

	service, err := OpenService(manager, serviceName, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return fmt.Errorf("could not open service %s: %v", serviceName, err)
	}
	defer service.Close()

	// check if state matches desiredState
	status, err := service.Query()
	if err != nil {
		return fmt.Errorf("could not retrieve service status: %v", err)
	}
	if status.State == desiredState {
		return nil
	}

	// Wait for timeout or state to match desiredState
	for {
		select {
		case <-time.After(300 * time.Millisecond):
			status, err := service.Query()
			if err != nil {
				return fmt.Errorf("could not retrieve service status: %v", err)
			}
			if status.State == desiredState {
				return nil
			}
		case <-ctx.Done():
			status, err := service.Query()
			if err != nil {
				return fmt.Errorf("could not retrieve service status: %v", err)
			}
			if status.State == desiredState {
				return nil
			}
			return ctx.Err()
		}
	}
}

// RestartService stops a service and thenif the stop was successful starts it again
func RestartService(serviceName string) error {
	var err error
	if err = StopService(serviceName); err == nil {
		err = StartService(serviceName)
	}
	return err
}

// ListDependentServices returns the services that depend on serviceName
//
// https://learn.microsoft.com/en-us/windows/win32/api/winsvc/nf-winsvc-enumdependentservicesw
//
// when Go has their version, replace ours with the upstream
// https://github.com/golang/go/issues/56766
func ListDependentServices(serviceName string, state enumServiceState) ([]EnumServiceStatus, error) {
	manager, err := OpenSCManager(windows.SC_MANAGER_CONNECT)
	if err != nil {
		return nil, err
	}
	defer manager.Disconnect()

	service, err := OpenService(manager, serviceName, windows.SERVICE_ENUMERATE_DEPENDENTS)
	if err != nil {
		return nil, fmt.Errorf("could not open service %s: %v", serviceName, err)
	}
	defer service.Close()

	deps, err := enumDependentServices(service.Handle, state)
	if err != nil {
		return nil, fmt.Errorf("could not enumerate dependent services for %s: %v", serviceName, err)
	}
	return deps, nil
}

// IsServiceDisabled returns true if serviceName is disabled
func IsServiceDisabled(serviceName string) (enabled bool, err error) {
	enabled = false

	manager, err := OpenSCManager(windows.SC_MANAGER_CONNECT)
	if err != nil {
		return
	}
	defer manager.Disconnect()

	service, err := OpenService(manager, serviceName, windows.SERVICE_QUERY_CONFIG)
	if err != nil {
		return enabled, fmt.Errorf("could not open service %s: %v", serviceName, err)
	}
	defer service.Close()

	serviceConfig, err := service.Config()
	if err != nil {
		return enabled, fmt.Errorf("could not retrieve config for %s: %v", serviceName, err)
	}
	return (serviceConfig.StartType == windows.SERVICE_DISABLED), nil
}

// IsServiceRunning returns true if serviceName's state is SERVICE_RUNNING
func IsServiceRunning(serviceName string) (running bool, err error) {
	running = false

	manager, err := OpenSCManager(windows.SC_MANAGER_CONNECT)
	if err != nil {
		return
	}
	defer manager.Disconnect()

	service, err := OpenService(manager, serviceName, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return running, fmt.Errorf("could not open service %s: %v", serviceName, err)
	}
	defer service.Close()

	serviceStatus, err := service.Query()
	if err != nil {
		return running, fmt.Errorf("could not retrieve status for %s: %v", serviceName, err)
	}
	return (serviceStatus.State == windows.SERVICE_RUNNING), nil
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
	}

	// since the initial buffer sent is 0 bytes, we expect the return code to
	// always be ERROR_MORE_DATA, unless something went wrong
	if err != error(windows.ERROR_MORE_DATA) {
		log.Warnf("Error getting buffer %v", err)
		return
	}

	servicearray := make([]uint8, bufsz)
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
