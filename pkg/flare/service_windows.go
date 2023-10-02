// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package flare

import (
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
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
	TriggersCount         *uint32
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

	sfa.ResetPeriod, err = s.ResetPeriod()
	if err != nil {
		log.Warnf("Error getting ResetPeriod for %s: %v", s.Name, err)
	}

	sfa.RebootMessage, err = s.RebootMessage()
	if err != nil {
		log.Warnf("Error getting RebootMessage for %s: %v", s.Name, err)
	}

	sfa.RecoveryCommand, err = s.RecoveryCommand()
	if err != nil {
		log.Warnf("Error getting RecoveryCommandfor %s: %v", s.Name, err)
	}

	sfa.FailureActionsOnNonCrashFailures, err = s.RecoveryActionsOnNonCrashFailures()
	if err != nil {
		log.Warnf("Error getting FailureActionsOnNonCrashFailuresfor %s: %v", s.Name, err)
	}

	recoveryActionSlice, err := s.RecoveryActions()
	if err != nil {
		log.Warnf("Error getting RecoveryActionsfor %s: %v", s.Name, err)
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

// Converting returned enums to readable strings
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

	conf, err := s.Config()
	if err != nil {
		log.Warnf("Error getting Config for %s: %v", s.Name, err)
	} else {
		srvinfo.Config = convertConfigToQueryServiceConfig(conf)
	}

	srvinfo.TriggersCount, err = serviceTriggerCount(s)
	if err != nil {
		log.Warnf("Error getting TriggersCount for %s: %v", s.Name, err)
	}

	srvinfo.ServiceFailureActions, err = getFailureActions(s)
	if err != nil {
		log.Warnf("Error getting ServiceFailureActions for %s: %v", s.Name, err)
	}

	srvcStatus, err := s.Query()
	if err != nil {
		log.Warnf("Error getting Service Status for %s: %v", s.Name, err)
	} else {
		srvinfo.ServiceState = serviceStatusToString(srvcStatus.State)
		srvinfo.ProcessID = srvcStatus.ProcessId
	}

	srvinfo.DependentServices, err = s.ListDependentServices(svc.AnyActivity)
	if err != nil {
		log.Warnf("Error getting DependentServices for %s: %v", s.Name, err)
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

func serviceTriggerCount(s *mgr.Service) (*uint32, error) {
	b, err := queryServiceConfig2Local(s, windows.SERVICE_CONFIG_TRIGGER_INFO)
	if err != nil {
		return nil, err
	}
	p := (*serviceTriggerInfo)(unsafe.Pointer(unsafe.SliceData(b)))

	return &p.TriggersCount, nil
}

func getDDServices(manager *mgr.Mgr) ([]serviceInfo, error) {
	ddServices := []serviceInfo{}

	// Returns a string slice with all running services. Does not return Kernel services only SERVICE_WIN32
	list, err := manager.ListServices()
	if err != nil {
		log.Warnf("Error getting list of running services %v", err)
		return nil, err
	}

	for _, serviceName := range list {
		if strings.HasPrefix(serviceName, "datadog") {
			srvc, err := winutil.OpenService(manager, serviceName, windows.GENERIC_READ)
			if err != nil {
				log.Warnf("Error Opening Service %s: %v", serviceName, err)
			} else {
				conf2, err := getServiceInfo(srvc)
				if err != nil {
					log.Warnf("Error getting info for %s: %v", serviceName, err)
				}
				ddServices = append(ddServices, conf2)
			}
		}
	}

	// Getting ddnpm service info separately
	ddnpm, err := winutil.OpenService(manager, "ddnpm", windows.GENERIC_READ)
	if err != nil {
		log.Warnf("Error Opening Service ddnpm %v", err)
	} else {
		ddnpmConf, err := getServiceInfo(ddnpm)
		if err != nil {
			log.Warnf("Error getting info for ddnpm: %v", err)
		}
		ddServices = append(ddServices, ddnpmConf)
	}

	return ddServices, nil
}
