// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package windows

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
)

// WinServiceManager implements ServiceManager using the SystemAPI interface
type WinServiceManager struct {
	api systemAPI
}

// NewWinServiceManager creates a new WinServiceManager with real implementations
func NewWinServiceManager() *WinServiceManager {
	return &WinServiceManager{
		api: &winSystemAPI{},
	}
}

// NewWinServiceManagerWithAPI creates a new WinServiceManager with a custom SystemAPI (for testing)
func NewWinServiceManagerWithAPI(api systemAPI) *WinServiceManager {
	return &WinServiceManager{
		api: api,
	}
}

// terminateServiceProcess terminates a service by killing its process.
// Returns nil if the service is not running or does not exist.
func (w *WinServiceManager) terminateServiceProcess(ctx context.Context, serviceName string) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "terminate_service_process")
	defer func() { span.Finish(err) }()
	span.SetTag("service_name", serviceName)

	// Get the process ID for the service
	processID, err := w.api.GetServiceProcessID(serviceName)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return nil
	} else if err != nil {
		return fmt.Errorf("could not get process ID for service %s: %w", serviceName, err)
	}

	if processID == 0 {
		return nil // Service is not running
	}

	span.SetTag("pid", fmt.Sprintf("%d", processID))

	// Open the process with termination rights
	handle, err := w.api.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, processID)
	if err != nil {
		return fmt.Errorf("could not open process %d for service %s: %w", processID, serviceName, err)
	}
	defer w.api.CloseHandle(handle) //nolint:errcheck

	// Verify we still have the correct process by checking the service PID again
	currentProcessID, err := w.api.GetServiceProcessID(serviceName)
	if err != nil {
		return fmt.Errorf("could not verify process ID for service %s: %w", serviceName, err)
	}
	if currentProcessID == 0 {
		// Service stopped between PID lookup and process termination
		return nil
	}

	if currentProcessID != processID {
		return fmt.Errorf("process ID for service %s changed from %d to %d, aborting termination",
			serviceName, processID, currentProcessID)
	}

	// Terminate the process
	err = w.api.TerminateProcess(handle, 1)
	if err != nil {
		return fmt.Errorf("could not terminate process %d for service %s: %w", processID, serviceName, err)
	}

	// Wait for the process to exit
	processWaitTimeout := 30 * time.Second
	waitResult, err := w.api.WaitForSingleObject(handle, uint32(processWaitTimeout.Milliseconds()))
	if err != nil {
		return fmt.Errorf("error waiting for process %d to exit: %w", processID, err)
	}

	if waitResult == uint32(windows.WAIT_TIMEOUT) {
		return fmt.Errorf("timeout waiting for process %d for service %s to exit", processID, serviceName)
	}

	return nil
}

// StopAllAgentServices stops the Agent service, expecting it to stop all other services as well.
// After attempting to stop the Agent service, it will force stop any remaining services.
//
// Returns nil if the Agent service does not exist.
func (w *WinServiceManager) StopAllAgentServices(ctx context.Context) (err error) {
	allAgentServices := []string{
		"datadog-trace-agent",
		"datadog-process-agent",
		"datadog-security-agent",
		"datadog-system-probe",
		"Datadog Installer",
		"datadogagent",
	}

	span, _ := telemetry.StartSpanFromContext(ctx, "stop_all_agent_services")
	defer func() { span.Finish(err) }()
	span.SetTag("agent-services", allAgentServices)

	// First, try to stop the main datadogagent service
	// In the normal case, this will stop all other services as well
	err = w.api.StopService("datadogagent")
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return nil
	}
	// ignore error (if any) and continue to force stop any remaining services

	// Terminate any remaining running services
	err = w.terminateServiceProcesses(ctx, allAgentServices)
	if err != nil {
		return err
	}

	return nil
}

// terminateServiceProcesses terminates the processes of the given services.
//
// Returns an error if any of the services failed to terminate.
// Returns nil if all services were terminated successfully, were not running, or do not exist.
func (w *WinServiceManager) terminateServiceProcesses(ctx context.Context, serviceNames []string) (err error) {
	var failedServices []error
	for _, serviceName := range serviceNames {

		state, err := w.api.GetServiceState(serviceName)
		if err != nil {
			if !errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
				failedServices = append(failedServices, fmt.Errorf("could not get service state for %s: %w", serviceName, err))
			}
			continue
		} else if state == svc.Stopped {
			continue
		}

		// Service is running or we failed to check, terminate its process
		err = w.terminateServiceProcess(ctx, serviceName)
		if err != nil {
			// Check if service is actually stopped despite the termination error
			state, stateErr := w.api.GetServiceState(serviceName)
			if stateErr != nil {
				failedServices = append(failedServices, fmt.Errorf("%s: termination failed (%w) and state verification failed (%w)", serviceName, err, stateErr))
			} else if state != svc.Stopped {
				failedServices = append(failedServices, fmt.Errorf("%s: termination failed and service still running: %w", serviceName, err))
			}
		}
	}

	if len(failedServices) > 0 {
		return fmt.Errorf("failed to stop services: %w", errors.Join(failedServices...))
	}

	return nil
}

// StartAgentServices starts the Agent service, expecting it to start all other services as well.
func (w *WinServiceManager) StartAgentServices(ctx context.Context) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "start_agent_service")
	defer func() { span.Finish(err) }()

	err = w.api.StartService("datadogagent")
	if err != nil {
		return fmt.Errorf("failed to start datadogagent service: %w", err)
	}

	return nil
}

// RestartAgentServices combines StopAllAgentServices and StartAgentServices.
//
// Ignores errors from StopAllAgentServices and will always attempt to start the services again.
// Returns an error if any of the services failed to start.
func (w *WinServiceManager) RestartAgentServices(ctx context.Context) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "restart_agent_services")
	defer func() { span.Finish(err) }()

	// Attempt to stop all agent services first
	_ = w.StopAllAgentServices(ctx)
	// ignore stop error, we always want to try to start the services again.
	// a stop error is unlikely since StopAllAgentServices force stops processes, too.

	// Always attempt to restart the services.
	err = w.StartAgentServices(ctx)
	if err != nil {
		return fmt.Errorf("failed to start agent services: %w", err)
	}

	return nil
}
