//go:build windows

package packages

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
)

// Internal types for SCM operations (not exported)

// winManagerHandle wraps mgr.Mgr for internal use
type winManagerHandle struct {
	mgr *mgr.Mgr
}

func (h *winManagerHandle) Disconnect() error {
	return h.mgr.Disconnect()
}

// winServiceHandle wraps mgr.Service for internal use
type winServiceHandle struct {
	service *mgr.Service
}

func (h *winServiceHandle) Query() (winServiceStatus, error) {
	status, err := h.service.Query()
	if err != nil {
		return winServiceStatus{}, err
	}
	return winServiceStatus{ProcessId: status.ProcessId}, nil
}

func (h *winServiceHandle) Close() error {
	return h.service.Close()
}

// winServiceStatus represents service status information for internal use
type winServiceStatus struct {
	ProcessId uint32
}

// Real implementations of the interfaces

// WinSystemAPI implements SystemAPI using winutil and Windows API
type WinSystemAPI struct{}

func (api *WinSystemAPI) GetServiceProcessID(serviceName string) (uint32, error) {
	manager, err := winutil.OpenSCManager(windows.SC_MANAGER_CONNECT)
	if err != nil {
		return 0, fmt.Errorf("could not open SCM for service %s: %w", serviceName, err)
	}
	defer manager.Disconnect()

	service, err := winutil.OpenService(manager, serviceName, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return 0, fmt.Errorf("could not open service %s: %w", serviceName, err)
	}
	defer service.Close()

	status, err := service.Query()
	if err != nil {
		return 0, fmt.Errorf("could not query service %s: %w", serviceName, err)
	}

	return status.ProcessId, nil
}

func (api *WinSystemAPI) IsServiceRunning(serviceName string) (bool, error) {
	return winutil.IsServiceRunning(serviceName)
}

func (api *WinSystemAPI) StopService(serviceName string) error {
	return winutil.StopService(serviceName)
}

func (api *WinSystemAPI) StartService(serviceName string) error {
	return winutil.StartService(serviceName)
}

func (api *WinSystemAPI) OpenProcess(desiredAccess uint32, inheritHandle bool, processID uint32) (ProcessHandle, error) {
	handle, err := windows.OpenProcess(desiredAccess, inheritHandle, processID)
	if err != nil {
		return nil, err
	}
	return &WinProcessHandle{handle: handle}, nil
}

func (api *WinSystemAPI) TerminateProcess(handle ProcessHandle, exitCode uint32) error {
	winHandle := handle.(*WinProcessHandle)
	return windows.TerminateProcess(winHandle.handle, exitCode)
}

func (api *WinSystemAPI) WaitForSingleObject(handle ProcessHandle, timeoutMs uint32) (uint32, error) {
	winHandle := handle.(*WinProcessHandle)
	return windows.WaitForSingleObject(winHandle.handle, timeoutMs)
}

func (api *WinSystemAPI) CloseHandle(handle ProcessHandle) error {
	winHandle := handle.(*WinProcessHandle)
	return windows.CloseHandle(winHandle.handle)
}

// WinProcessHandle implements ProcessHandle
type WinProcessHandle struct {
	handle windows.Handle
}

// WinServiceManager implements ServiceManager using the SystemAPI interface
type WinServiceManager struct {
	api SystemAPI
}

// NewWinServiceManager creates a new WinServiceManager with real implementations
func NewWinServiceManager() *WinServiceManager {
	return &WinServiceManager{
		api: &WinSystemAPI{},
	}
}

// NewWinServiceManagerWithAPI creates a new WinServiceManager with a custom SystemAPI (for testing)
func NewWinServiceManagerWithAPI(api SystemAPI) *WinServiceManager {
	return &WinServiceManager{
		api: api,
	}
}

// Basic service operations
func (w *WinServiceManager) IsServiceRunning(serviceName string) (bool, error) {
	return w.api.IsServiceRunning(serviceName)
}

func (w *WinServiceManager) StopService(serviceName string) error {
	return w.api.StopService(serviceName)
}

func (w *WinServiceManager) StartService(serviceName string) error {
	return w.api.StartService(serviceName)
}

// GetServiceProcessID retrieves the process ID for a given service name
func (w *WinServiceManager) GetServiceProcessID(serviceName string) (uint32, error) {
	return w.api.GetServiceProcessID(serviceName)
}

// TerminateServiceProcess terminates a service by killing its process
func (w *WinServiceManager) TerminateServiceProcess(ctx context.Context, serviceName string) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "terminate_service_process")
	defer func() { span.Finish(err) }()
	span.SetTag("service_name", serviceName)

	// Get the process ID for the service
	processID, err := w.GetServiceProcessID(serviceName)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return nil
	} else if err != nil {
		return fmt.Errorf("could not get process ID for service %s: %w", serviceName, err)
	}

	if processID == 0 {
		return nil // Service is not running
	}

	span.SetTag("pid", fmt.Sprintf("%d", processID))
	log.Infof("Waiting for service %s process %d to exit", serviceName, processID)

	// Open the process with termination rights
	handle, err := w.api.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, processID)
	if err != nil {
		return fmt.Errorf("could not open process %d for service %s: %w", processID, serviceName, err)
	}
	defer w.api.CloseHandle(handle)

	// Verify we still have the correct process by checking the service PID again
	currentProcessID, err := w.GetServiceProcessID(serviceName)
	if err != nil {
		return fmt.Errorf("could not verify process ID for service %s: %w", serviceName, err)
	}
	if currentProcessID == 0 {
		return nil // Service stopped between PID lookup and process termination
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

	log.Infof("Service %s process %d terminated successfully", serviceName, processID)
	return nil
}

// StopAllAgentServices stops all agent services
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
	log.Infof("Stopping all Datadog Agent services")

	// First, try to stop the main datadogagent service
	err = w.StopService("datadogagent")
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		log.Infof("Service datadogagent does not exist, skipping stop action")
		return nil
	} else if err != nil {
		log.Warnf("Failed to stop main datadogagent service: %v", err)
	}

	// Terminate any remaining running services
	var failedServices []error
	for _, serviceName := range allAgentServices {
		log.Debugf("Ensuring service %s is stopped", serviceName)

		running, err := w.IsServiceRunning(serviceName)
		if err != nil {
			if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
				log.Debugf("Service %s does not exist, skipping", serviceName)
				continue
			}
			log.Warnf("Could not check if service %s is running: %v", serviceName, err)
		} else if !running {
			log.Debugf("Service %s is already stopped", serviceName)
			continue
		}

		// Service is running, terminate its process
		err = w.TerminateServiceProcess(ctx, serviceName)
		if err != nil {
			// Check if service is actually stopped despite the termination error
			running, runningErr := w.IsServiceRunning(serviceName)
			if runningErr != nil {
				log.Errorf("Termination failed for service %s (%v) and could not verify service state (%v)", serviceName, err, runningErr)
				failedServices = append(failedServices, fmt.Errorf("%s: termination failed (%w) and state verification failed (%w)", serviceName, err, runningErr))
			} else if running {
				log.Errorf("Termination failed for service %s (%v) and service is still running", serviceName, err)
				failedServices = append(failedServices, fmt.Errorf("%s: termination failed and service still running: %w", serviceName, err))
			} else {
				log.Infof("Service %s is stopped", serviceName)
			}
		} else {
			log.Infof("Successfully terminated service %s", serviceName)
		}
	}

	if len(failedServices) > 0 {
		return fmt.Errorf("failed to stop services: %w", errors.Join(failedServices...))
	}

	log.Infof("All Datadog Agent services have been stopped successfully")
	return nil
}

// StartAgentServices starts the agent services
func (w *WinServiceManager) StartAgentServices(ctx context.Context) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "start_agent_service")
	defer func() { span.Finish(err) }()
	log.Infof("Starting datadogagent service")

	err = w.StartService("datadogagent")
	if err != nil {
		return fmt.Errorf("failed to start datadogagent service: %w", err)
	}

	log.Infof("Successfully started agent services")
	return nil
}

// RestartAgentServices restarts the agent services
func (w *WinServiceManager) RestartAgentServices(ctx context.Context) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "restart_agent_services")
	defer func() { span.Finish(err) }()

	// Attempt to stop all agent services first
	err = w.StopAllAgentServices(ctx)
	if err != nil {
		log.Warnf("Failed to stop agent services: %v. Continuing with start attempt to ensure daemon stays running", err)
	}

	// Always attempt to restart the services
	err = w.StartAgentServices(ctx)
	if err != nil {
		return fmt.Errorf("failed to start agent services: %w", err)
	}

	log.Infof("Successfully restarted agent services")
	return nil
}
