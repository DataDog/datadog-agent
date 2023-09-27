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
	SC_MANAGER_ACCESS = windows.SC_MANAGER_CONNECT | windows.SC_MANAGER_ENUMERATE_SERVICE
	SERVICE_ACCESS    = windows.SERVICE_ENUMERATE_DEPENDENTS | windows.SERVICE_QUERY_CONFIG | windows.SERVICE_QUERY_STATUS
)

type ServiceInfo struct {
	ServiceName           string
	ServiceState          string
	ProcessId             uint32
	Config                QueryServiceConfig
	DependentServices     []string
	TriggersCount         uint32
	ServiceFailureActions ServiceFailureActions
}

type QueryServiceConfig struct {
	ServiceType      string
	StartType        string
	ErrorControl     string
	BinaryPathName   string // fully qualified path to the service binary file, can also include arguments for an auto-start service
	LoadOrderGroup   string
	TagId            uint32
	Dependencies     []string
	ServiceStartName string // name of the account under which the service should run
	DisplayName      string
	Password         string
	Description      string
	SidType          uint32 // one of SERVICE_SID_TYPE, the type of sid to use for the service
	DelayedAutoStart bool   // the service is started after other auto-start services are started plus a short delay
}

type RecoveryActionsUpdated struct {
	Type  string
	Delay time.Duration
}

type ServiceFailureActions struct {
	ResetPeriod                      uint32
	RebootMessage                    string
	RecoveryCommand                  string
	FailureActionsOnNonCrashFailures bool
	RecoveryActions                  []RecoveryActionsUpdated
}

func GetFailureActions(s *mgr.Service) (ServiceFailureActions, error) {
	sfa := ServiceFailureActions{}

	var err error
	var err1 error

	sfa.ResetPeriod, err1 = s.ResetPeriod()
	if err1 != nil {
		err = fmt.Errorf("%v, ResetPeriod: %v", err, err1)
	}

	sfa.RebootMessage, err = s.RebootMessage()
	if err1 != nil {
		err = fmt.Errorf("%v, RebootMessage: %v", err, err1)
	}

	sfa.RecoveryCommand, err = s.RecoveryCommand()
	if err1 != nil {
		err = fmt.Errorf("%v, RecoveryCommand: %v", err, err1)
	}

	sfa.FailureActionsOnNonCrashFailures, err = s.RecoveryActionsOnNonCrashFailures()
	if err1 != nil {
		err = fmt.Errorf("%v, FailureActionsOnNonCrashFailure: %v", err, err1)
	}

	recovery_action_slice, err1 := s.RecoveryActions()
	if err1 != nil {
		err = fmt.Errorf("%v, RecoveryActions: %v", err, err1)
	}

	for _, act := range recovery_action_slice {
		rau := RecoveryActionsUpdated{
			Type:  ActionTypeToString(act.Type),
			Delay: act.Delay / time.Millisecond,
		}
		sfa.RecoveryActions = append(sfa.RecoveryActions, rau)
	}

	return sfa, err
}

func ServiceTypeToString(serviceType uint32) string {
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

func StartTypeToString(startType uint32) string {
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
		return "Unkown"
	}
}

func ErrorControlToString(errorCode uint32) string {
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

func ServiceStatusToString(svc_state svc.State) string {
	switch svc_state {
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

func ConvertConfigToQueryServiceConfig(config mgr.Config) QueryServiceConfig {
	return QueryServiceConfig{
		ServiceType:      ServiceTypeToString(config.ServiceType),
		StartType:        StartTypeToString(config.StartType),
		ErrorControl:     ErrorControlToString(config.ErrorControl),
		BinaryPathName:   config.BinaryPathName,
		LoadOrderGroup:   config.LoadOrderGroup,
		TagId:            config.TagId,
		Dependencies:     config.Dependencies,
		ServiceStartName: config.ServiceStartName,
		DisplayName:      config.DisplayName,
		Password:         config.Password,
		Description:      config.Description,
		SidType:          config.SidType,
		DelayedAutoStart: config.DelayedAutoStart,
	}
}

func GetServiceInfo(s *mgr.Service) (ServiceInfo, error) {

	srvinfo := ServiceInfo{}
	srvinfo.ServiceName = s.Name

	var err error
	var err1 error

	conf, err1 := s.Config()
	if err1 != nil {
		err = fmt.Errorf("%v, Config: %v", err, err1)
	}
	srvinfo.Config = ConvertConfigToQueryServiceConfig(conf)

	srvinfo.TriggersCount, err1 = ServiceTriggerCount(s)
	if err1 != nil {
		err = fmt.Errorf("%v, ServiceTriggerCount: %v", err, err1)
	}

	srvinfo.ServiceFailureActions, err1 = GetFailureActions(s)
	if err1 != nil {
		err = fmt.Errorf("%v, GetFailureActions: %v", err, err1)
	}

	srvc_status, err1 := s.Query()
	if err1 != nil {
		err = fmt.Errorf("%v, Query: %v", err, err1)
	}
	srvinfo.ServiceState = ServiceStatusToString(srvc_status.State)

	srvinfo.DependentServices, err1 = s.ListDependentServices(svc.AnyActivity)
	if err1 != nil {
		err = fmt.Errorf("%v, ListDependentServices: %v", err, err1)
	}

	return srvinfo, err
}

func ActionTypeToString(i int) string {
	switch i {
	case windows.SC_ACTION_NONE:
		return "No Action."
	case windows.SC_ACTION_REBOOT:
		return "Reboot the computer."
	case windows.SC_ACTION_RESTART:
		return "Restart the service."
	case windows.SC_ACTION_RUN_COMMAND:
		return "Run a command."
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
type ServiceTriggerInfo struct {
	TriggersCount uint32
	Triggers      uintptr
	Reserved      *uint8
}

func ServiceTriggerCount(s *mgr.Service) (uint32, error) {
	b, err := queryServiceConfig2Local(s, windows.SERVICE_CONFIG_TRIGGER_INFO)
	if err != nil {
		return 0, err
	}
	p := (*ServiceTriggerInfo)(unsafe.Pointer(&b[0]))
	/*if p.TriggersCount == nil {
		return nil, err
	}*/

	return p.TriggersCount, nil
}
