// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package flare

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	scManagerAccess = windows.SC_MANAGER_CONNECT | windows.SC_MANAGER_ENUMERATE_SERVICE
)

type serviceInfo struct {
	ServiceName           string
	ServiceState          string
	ProcessID             uint32
	Config                queryServiceConfig
	DependentServices     []string
	TriggersCount         uint32
	ServiceFailureActions serviceFailureActions
}

type queryServiceConfig struct {
	ServiceType      string
	StartType        string
	ErrorControl     string
	BinaryPathName   string // fully qualified path to the service binary file, can also include arguments for an auto-start service
	LoadOrderGroup   string
	TagID            uint32
	Dependencies     []string
	ServiceStartName string // name of the account under which the service should run
	DisplayName      string
	Password         string
	Description      string
	SidType          uint32 // one of SERVICE_SID_TYPE, the type of sid to use for the service
	DelayedAutoStart bool   // the service is started after other auto-start services are started plus a short delay
}

type recoveryActionsUpdated struct {
	Type  string
	Delay time.Duration
}

type serviceFailureActions struct {
	ResetPeriod                      uint32
	RebootMessage                    string
	RecoveryCommand                  string
	FailureActionsOnNonCrashFailures bool
	RecoveryActions                  []recoveryActionsUpdated
}

func getFailureActions(s *mgr.Service) (serviceFailureActions, error) {
	sfa := serviceFailureActions{}

	var err error
	var err1 error

	sfa.ResetPeriod, err1 = s.ResetPeriod()
	if err1 != nil {
		err = fmt.Errorf("%v, ResetPeriod: %v", err, err1)
	}

	sfa.RebootMessage, err1 = s.RebootMessage()
	if err1 != nil {
		err = fmt.Errorf("%v, RebootMessage: %v", err, err1)
	}

	sfa.RecoveryCommand, err1 = s.RecoveryCommand()
	if err1 != nil {
		err = fmt.Errorf("%v, RecoveryCommand: %v", err, err1)
	}

	sfa.FailureActionsOnNonCrashFailures, err1 = s.RecoveryActionsOnNonCrashFailures()
	if err1 != nil {
		err = fmt.Errorf("%v, FailureActionsOnNonCrashFailure: %v", err, err1)
	}

	recoveryActionSlice, err1 := s.RecoveryActions()
	if err1 != nil {
		err = fmt.Errorf("%v, RecoveryActions: %v", err, err1)
	}

	for _, act := range recoveryActionSlice {
		rau := recoveryActionsUpdated{
			Type:  actionTypeToString(act.Type),
			Delay: act.Delay / time.Millisecond,
		}
		sfa.RecoveryActions = append(sfa.RecoveryActions, rau)
	}

	return sfa, err
}

func serviceTypeToString(serviceType uint32) string {
	switch serviceType {
	case windows.SERVICE_KERNEL_DRIVER:
		return "KernelDriver"
	case windows.SERVICE_FILE_SYSTEM_DRIVER:
		return "FileSystemDriver"
	case windows.SERVICE_ADAPTER:
		return "Adapter"
	case windows.SERVICE_RECOGNIZER_DRIVER:
		return "RecognizerDriver"
	case windows.SERVICE_WIN32_OWN_PROCESS:
		return "Win32OwnProcess"
	case windows.SERVICE_WIN32_SHARE_PROCESS:
		return "Win32ShareProcess"
	case windows.SERVICE_INTERACTIVE_PROCESS:
		return "InteractiveProcess"
	default:
		return "Unknown"
	}
}

func startTypeToString(startType uint32) string {
	switch startType {
	case windows.SERVICE_AUTO_START:
		return "Automatic"
	case windows.SERVICE_BOOT_START:
		return "Boot"
	case windows.SERVICE_DISABLED:
		return "Disabled"
	case windows.SERVICE_DEMAND_START:
		return "Manual"
	case windows.SERVICE_SYSTEM_START:
		return "System"
	default:
		return "Unknown"
	}
}

func errorControlToString(errorCode uint32) string {
	switch errorCode {
	case windows.SERVICE_ERROR_IGNORE:
		return "Error Ignore"
	case windows.SERVICE_ERROR_NORMAL:
		return "Error Normal"
	case windows.SERVICE_ERROR_SEVERE:
		return "Error Severe"
	case windows.SERVICE_ERROR_CRITICAL:
		return "Error Critical"
	default:
		return "Unknown"
	}
}

func serviceStatusToString(svcState svc.State) string {
	switch svcState {
	case svc.Stopped:
		return "Stopped"
	case svc.StartPending:
		return "Start Pending"
	case svc.StopPending:
		return "Stop Pending"
	case svc.Running:
		return "Running"
	case svc.ContinuePending:
		return "Continue Pending"
	case svc.PausePending:
		return "Pause Pending"
	case svc.Paused:
		return "Pause"
	default:
		return "Unknown"
	}
}

func convertConfigToQueryServiceConfig(config mgr.Config) queryServiceConfig {
	return queryServiceConfig{
		ServiceType:      serviceTypeToString(config.ServiceType),
		StartType:        startTypeToString(config.StartType),
		ErrorControl:     errorControlToString(config.ErrorControl),
		BinaryPathName:   config.BinaryPathName,
		LoadOrderGroup:   config.LoadOrderGroup,
		TagID:            config.TagId,
		Dependencies:     config.Dependencies,
		ServiceStartName: config.ServiceStartName,
		DisplayName:      config.DisplayName,
		Password:         config.Password,
		Description:      config.Description,
		SidType:          config.SidType,
		DelayedAutoStart: config.DelayedAutoStart,
	}
}

func getServiceInfo(s *mgr.Service) (serviceInfo, error) {

	srvinfo := serviceInfo{}
	srvinfo.ServiceName = s.Name

	var err error
	var err1 error

	conf, err1 := s.Config()
	if err1 != nil {
		err = fmt.Errorf("%v, Config: %v", err, err1)
	} else {
		srvinfo.Config = convertConfigToQueryServiceConfig(conf)
	}

	srvinfo.TriggersCount, err1 = serviceTriggerCount(s)
	if err1 != nil {
		err = fmt.Errorf("%v, ServiceTriggerCount: %v", err, err1)
	}

	srvinfo.ServiceFailureActions, err1 = getFailureActions(s)
	if err1 != nil {
		err = fmt.Errorf("%v, GetFailureActions: %v", err, err1)
	}

	srvcStatus, err1 := s.Query()
	if err1 != nil {
		err = fmt.Errorf("%v, Query: %v", err, err1)
	} else {
		srvinfo.ServiceState = serviceStatusToString(srvcStatus.State)
		srvinfo.ProcessID = srvcStatus.ProcessId
	}

	srvinfo.DependentServices, err1 = s.ListDependentServices(svc.AnyActivity)
	if err1 != nil {
		err = fmt.Errorf("%v, ListDependentServices: %v", err, err1)
	}

	return srvinfo, err
}

func actionTypeToString(i int) string {
	switch i {
	case windows.SC_ACTION_NONE:
		return "No Action"
	case windows.SC_ACTION_REBOOT:
		return "Reboot the computer"
	case windows.SC_ACTION_RESTART:
		return "Restart the service"
	case windows.SC_ACTION_RUN_COMMAND:
		return "Run a command"
	default:
		return "Unknown"
	}
}

func queryServiceConfig2Local(s *mgr.Service, infoLevel uint32) ([]byte, error) {
	n := uint32(1024)
	for {
		b := make([]byte, n)
		err := windows.QueryServiceConfig2(s.Handle, infoLevel, &b[0], n, &n)
		if err == nil {
			return b, nil
		}
		if err.(syscall.Errno) != syscall.ERROR_INSUFFICIENT_BUFFER {
			return nil, err
		}
		if n <= uint32(len(b)) {
			return nil, err
		}
	}
}

// Based off of https://cs.opensource.google/go/x/sys/+/refs/tags/v0.12.0:windows/service.go;l=213-228
type serviceTriggerInfo struct {
	TriggersCount uint32
	Triggers      uintptr
	Reserved      *uint8
}

func serviceTriggerCount(s *mgr.Service) (uint32, error) {
	b, err := queryServiceConfig2Local(s, windows.SERVICE_CONFIG_TRIGGER_INFO)
	if err != nil {
		return 0, err
	}
	//p := (*ServiceTriggerInfo)(unsafe.Pointer(&b[0]))
	p := (*serviceTriggerInfo)(unsafe.Pointer(unsafe.SliceData(b)))

	return p.TriggersCount, nil
}
