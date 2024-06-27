// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package winutil

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	defaultServiceCommandTimeout = 30
)

// to support edge case/rase condition testing
type stopServiceCallback interface {
	afterDependentsEnumeration()
	beforeStopService(serviceName string)
}

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
		return nil, fmt.Errorf("could not open service %s: %w", serviceName, err)
	}
	return &mgr.Service{Name: serviceName, Handle: h}, nil
}

// Compound convenience function (two above often called one after the other)
func openManagerService(serviceName string, desiredAccess uint32) (*mgr.Mgr, *mgr.Service, error) {
	manager, err := OpenSCManager(windows.SC_MANAGER_CONNECT)
	if err != nil {
		return nil, nil, fmt.Errorf("could not open SCM for service %s: %w", serviceName, err)
	}

	service, err := OpenService(manager, serviceName, desiredAccess)
	if err != nil {
		manager.Disconnect()
		return nil, nil, err
	}

	return manager, service, nil
}

// Compound convenience function
func closeManagerService(manager *mgr.Mgr, service *mgr.Service) {
	if service != nil {
		service.Close()
	}
	if manager != nil {
		manager.Disconnect()
	}
}

// StartService starts serviceName via SCM.
//
// Does not block until service is started
// https://learn.microsoft.com/en-us/windows/win32/api/winsvc/nf-winsvc-startservicea#remarks
func StartService(serviceName string, serviceArgs ...string) error {
	manager, service, err := openManagerService(serviceName, windows.SERVICE_START)
	if err != nil {
		return err
	}
	defer closeManagerService(manager, service)
	return doStartService(service, serviceName, serviceArgs...)
}

func doStartService(service *mgr.Service, serviceName string, serviceArgs ...string) error {
	if err := service.Start(serviceArgs...); err != nil {
		return fmt.Errorf("could not start service %s: %w", serviceName, err)
	}
	return nil
}

// ControlService sends a control code to a specified service and waits up to
// timeout for the service to transition to the requested state
//
// https://learn.microsoft.com/en-us/windows/win32/api/winsvc/nf-winsvc-controlservice
//
//revive:disable-next-line:var-naming Name is intended to match the Windows API name
func ControlService(serviceName string, command svc.Cmd, to svc.State, desiredAccess uint32, timeout uint64) error {
	manager, service, err := openManagerService(serviceName, desiredAccess)
	if err != nil {
		return err
	}
	defer closeManagerService(manager, service)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	return doControlService(ctx, service, serviceName, command, to)
}

func doControlService(ctx context.Context, service *mgr.Service, serviceName string, command svc.Cmd, to svc.State) error {
	// check if we are already in the desired state (if error
	// skip the check and proceed directly with the control)
	status, err := service.Query()
	if err == nil && status.State == to {
		return nil
	}

	status, err = service.Control(command)
	if err != nil {
		// try to get the status after the control
		statusAfter, errAfter := service.Query()
		if errAfter != nil {
			return fmt.Errorf("could not send control %d to service %s: %w Before control[state:%d, accepts:%d])",
				command, serviceName, err, status.State, status.Accepts)
		}
		return fmt.Errorf("could not send control %d to service %s: %w Before control[state:%d, accepts:%d]), after[state:%d, accepts:%d] ",
			command, serviceName, err, status.State, status.Accepts, statusAfter.State, statusAfter.Accepts)
	}

	return doWaitForState(ctx, service, to)
}

func doStopService(ctx context.Context, service *mgr.Service, serviceName string) error {
	return doControlService(ctx, service, serviceName, svc.Stop, svc.Stopped)
}

// StopService stops a service and any services that depend on it
func StopService(serviceName string) error {
	desiredStopAccess := uint32(windows.SERVICE_STOP | windows.SERVICE_QUERY_STATUS | windows.SERVICE_ENUMERATE_DEPENDENTS)
	manager, service, err := openManagerService(serviceName, desiredStopAccess)
	if err != nil {
		return err
	}
	defer closeManagerService(manager, service)
	return doStopServiceWithDependencies(manager, service, serviceName, svc.AnyActivity, nil)
}

// We need to get all dependent services (windows.SERVICE_STATE_ALL) to attempt to stop them,
// including those that are not RUNNING yet (stopped). It may appears to be strange, but it
// better handles some edge cases (which more likely during installation or upgrade if
// Agent configuration is updated immediately after). It may happen if the core agent
// (datadogagent) service is starting, which in turn will "manually" start dependent services
// and at the same time, externally, a configuration script would restart agent to make sure
// that new configuration is applied.
//
// In this case, attempting to stop ALL dependent services should handle cases when a
// dependent service started moments after stop command get list of dependent services and
// and if it collects only already running dependent services it would miss a chance to get
// and stop just started dependent services. In this case, as result, restart would fail,
// the core agent service will not be stopped, and some dependent services may be left
// running (and some may be stopped), which may lead to unexpected behavior. Certanly, new
// configuration will not be applied.
//
//		For illustration purposes here is an example of one specific race condition:
//		1. The core agent (datadogagent) service is starting
//		2. Starting core agent starts trace-agent dependent service
//		3. Agent's restart-service CLI command started
//		4. Agent get list all dependent services (e.g., trace-agent, process-agent, system-probe
//		5. Agent's restart-service CLI command enumerate all dependent services and if they are
//	    running it will stop them.
//		6. Just before restart-service CLI command will stop core agent service, the service
//	    is still starting and it may start trace-agent dependent service.
//		7. Attempt to stop core agent service will fail because a dependent service is "still" running
//
// Callback is invoked to help unit tests to setup race condition in deterministic way
func doStopServiceWithDependencies(manager *mgr.Mgr, service *mgr.Service, serviceName string,
	depenStatus svc.ActivityStatus, callback stopServiceCallback) error {

	ctx, cancel := context.WithTimeout(context.Background(), defaultServiceCommandTimeout*time.Second)
	defer cancel()

	// open dependent services
	depServiceNames, err := service.ListDependentServices(depenStatus)
	if err != nil {
		return fmt.Errorf("could not list dependent services for %s: %v", serviceName, err)
	}
	if callback != nil {
		callback.afterDependentsEnumeration()
	}
	var depServices []*mgr.Service
	for _, depServiceName := range depServiceNames {
		depService, err := OpenService(manager, depServiceName, windows.SERVICE_STOP|windows.SERVICE_QUERY_STATUS)
		if err != nil {
			return fmt.Errorf("could open service %s: %w", depServiceName, err)
		}
		depServices = append(depServices, depService)
		defer depService.Close()
	}

	for {
		select {
		case <-ctx.Done():
			// timed out
			return fmt.Errorf("could not stop service %s: %w", serviceName, ctx.Err())
		default:
			// try to stop dependent and then primary target
			for i, depService := range depServices {
				if callback != nil {
					callback.beforeStopService(depServiceNames[i])
				}
				err = doStopService(ctx, depService, depServiceNames[i])
				if err != nil {
					return fmt.Errorf("could not stop service %s: %w", depServiceNames[i], err)
				}
			}

			if callback != nil {
				callback.beforeStopService(serviceName)
			}
			err = doStopService(ctx, service, serviceName)
			if err == nil {
				return nil
			}
			if errors.Is(errors.Unwrap(err), windows.ERROR_DEPENDENT_SERVICES_RUNNING) {
				// If we are here, it means that some dependent service is still running, try
				// again hopefully race condition will be resolved in the next iteration.
				continue
			}

			return fmt.Errorf("could not stop service %s: %w", serviceName, err)
		}
	}
}

// WaitForState waits for the service to become the desired state. A timeout can be specified
// with a context. Returns nil if/when the service becomes the desired state.
func WaitForState(ctx context.Context, serviceName string, desiredState svc.State) error {
	manager, service, err := openManagerService(serviceName, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return err
	}
	defer closeManagerService(manager, service)

	return doWaitForState(ctx, service, desiredState)
}

// WaitForState waits for the service to become the desired state. A timeout can be specified
// with a context. Returns nil if/when the service becomes the desired state.
func doWaitForState(ctx context.Context, service *mgr.Service, desiredState svc.State) error {
	// check if state matches desiredState
	status, err := service.Query()
	if err != nil {
		return fmt.Errorf("could not retrieve service status: %w", err)
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
				return fmt.Errorf("could not retrieve service status: %w", err)
			}
			if status.State == desiredState {
				return nil
			}
		case <-ctx.Done():
			status, err := service.Query()
			if err != nil {
				return fmt.Errorf("could not retrieve service status: %w", err)
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
	restartFlags := uint32(windows.SERVICE_ENUMERATE_DEPENDENTS | windows.SERVICE_START |
		windows.SERVICE_STOP | windows.SERVICE_QUERY_STATUS)
	manager, service, err := openManagerService(serviceName, restartFlags)
	if err != nil {
		return err
	}
	defer closeManagerService(manager, service)

	if err = doStopServiceWithDependencies(manager, service, serviceName, svc.AnyActivity, nil); err == nil {
		err = doStartService(service, serviceName)
	}
	return err
}

// IsServiceDisabled returns true if serviceName is disabled
func IsServiceDisabled(serviceName string) (enabled bool, err error) {
	enabled = false

	manager, service, err := openManagerService(serviceName, windows.SERVICE_QUERY_CONFIG)
	if err != nil {
		return enabled, err
	}
	defer closeManagerService(manager, service)

	serviceConfig, err := service.Config()
	if err != nil {
		return enabled, fmt.Errorf("could not retrieve config for %s: %w", serviceName, err)
	}
	return (serviceConfig.StartType == windows.SERVICE_DISABLED), nil
}

// IsServiceRunning returns true if serviceName's state is SERVICE_RUNNING
func IsServiceRunning(serviceName string) (running bool, err error) {
	running = false

	manager, service, err := openManagerService(serviceName, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return running, err
	}
	defer closeManagerService(manager, service)

	status, err := service.Query()
	if err != nil {
		return running, fmt.Errorf("could not retrieve status for %s: %w", serviceName, err)
	}
	return (status.State == windows.SERVICE_RUNNING), nil
}
