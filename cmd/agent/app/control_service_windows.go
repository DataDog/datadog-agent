// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package app

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"github.com/spf13/cobra"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() {
	AgentCmd.AddCommand(startsvcCommand)
	AgentCmd.AddCommand(stopsvcCommand)
	AgentCmd.AddCommand(restartsvcCommand)
}

var startsvcCommand = &cobra.Command{
	Use:   "start-service",
	Short: "starts the agent within the service control manager",
	Long:  ``,
	RunE:  StartService,
}

var stopsvcCommand = &cobra.Command{
	Use:   "stopservice",
	Short: "stops the agent within the service control manager",
	Long:  ``,
	RunE:  stopService,
}

var restartsvcCommand = &cobra.Command{
	Use:   "restart-service",
	Short: "restarts the agent within the service control manager",
	Long:  ``,
	RunE:  restartService,
}

var (
	modadvapi32 = windows.NewLazyDLL("advapi32.dll")

	procEnumDependentServices = modadvapi32.NewProc("EnumDependentServicesW")
)

type enumServiceState uint32

const (
	enumServiceActive = enumServiceState(0x1) // START_PENDING, STOP_PENDING, RUNNING
	// continue_pending, pause_pending, paused
	enumServiceInactive = enumServiceState(0x02) // STOPPED
	enumServiceAll      = enumServiceState(0x03) // all of the above
)

// StartService starts the agent service via the Service Control Manager
func StartService(cmd *cobra.Command, args []string) error {
	/*
	 * default go impolementations of mgr.Connect and mgr.OpenService use way too
	 * open permissions by default.  Use those structures so the other methods
	 * work properly, but initialize them here using restrictive enough permissions
	 * that we can actually open/start the service when running as non-root.
	 */
	h, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		log.Warnf("Failed to connect to scm %v", err)
		return err
	}
	m := &mgr.Mgr{Handle: h}
	defer m.Disconnect()

	hSvc, err := windows.OpenService(m.Handle, windows.StringToUTF16Ptr(config.ServiceName),
		windows.SERVICE_START|windows.SERVICE_STOP)
	if err != nil {
		log.Warnf("Failed to open service %v", err)
		return fmt.Errorf("could not access service: %v", err)
	}
	scm := &mgr.Service{Name: config.ServiceName, Handle: hSvc}
	defer scm.Close()
	err = scm.Start("is", "manual-started")
	if err != nil {
		log.Warnf("Failed to start service %v", err)
		return fmt.Errorf("could not start service: %v", err)
	}
	return nil
}

func stopService(cmd *cobra.Command, args []string) error {
	return StopService(config.ServiceName, true)
}

func restartService(cmd *cobra.Command, args []string) error {
	var err error
	if err = StopService(config.ServiceName, true); err == nil {
		err = StartService(cmd, args)
	}
	return err
}

// StopService stops the agent service via the Service Control Manager
func StopService(serviceName string, withDeps bool) error {
	/*
	 * default go impolementations of mgr.Connect and mgr.OpenService use way too
	 * open permissions by default.  Use those structures so the other methods
	 * work properly, but initialize them here using restrictive enough permissions
	 * that we can actually open/start the service when running as non-root.
	 */
	h, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		log.Warnf("Failed to connect to scm %v", err)
		return err
	}
	m := &mgr.Mgr{Handle: h}
	defer m.Disconnect()

	hSvc, err := windows.OpenService(m.Handle, windows.StringToUTF16Ptr(serviceName),
		windows.SERVICE_START|windows.SERVICE_STOP|windows.SERVICE_QUERY_STATUS|windows.SERVICE_ENUMERATE_DEPENDENTS)
	if err != nil {
		log.Warnf("Failed to open service %v", err)
		return fmt.Errorf("could not access service: %v", err)
	}
	s := &mgr.Service{Name: serviceName, Handle: hSvc}
	defer s.Close()
	if withDeps {
		deps, err := enumDependentServices(s.Handle, enumServiceActive)
		if err != nil {
			log.Warnf("Failed to enumerate dependencies; skipping %v", err)
		} else {
			for _, dep := range deps {
				log.Debugf("Stopping service %s", dep.serviceName)
				StopService(dep.serviceName, false)
			}
		}
	}
	status, err := s.Control(svc.Stop)
	if err != nil {
		return fmt.Errorf("could not send control=%d: %v", svc.Stop, err)
	}
	timeout := time.Now().Add(10 * time.Second)
	for status.State != svc.Stopped {
		if timeout.Before(time.Now()) {
			return fmt.Errorf("timeout waiting for service to go to state=%d", svc.Stopped)
		}
		time.Sleep(300 * time.Millisecond)
		status, err = s.Query()
		if err != nil {
			return fmt.Errorf("could not retrieve service status: %v", err)
		}
	}
	return nil
}

// ControlService sets the service state via the Service Control Manager
func ControlService(serviceName string, c svc.Cmd, to svc.State) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("could not access service: %v", err)
	}
	defer s.Close()
	status, err := s.Control(c)
	if err != nil {
		return fmt.Errorf("could not send control=%d: %v", c, err)
	}
	timeout := time.Now().Add(10 * time.Second)
	for status.State != to {
		if timeout.Before(time.Now()) {
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
		ss := EnumServiceStatus{serviceName: winutil.ConvertWindowsString(servicearray[ess.ServiceName:]),
			displayName: winutil.ConvertWindowsString(servicearray[ess.DisplayName:]),
			status:      ess.Status}
		services = append(services, ss)
	}
	return
}
