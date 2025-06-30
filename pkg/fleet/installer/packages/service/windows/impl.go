//go:build windows

package windows

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/sys/windows"
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

// terminateServiceProcess terminates a service by killing its process
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
	log.Infof("Waiting for service %s process %d to exit", serviceName, processID)

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
	// In the normal case, this will stop all other services as well
	err = w.api.StopService("datadogagent")
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		log.Infof("Service datadogagent does not exist, skipping stop action")
		return nil
	} else if err != nil {
		log.Warnf("Failed to stop main datadogagent service: %v", err)
	}

	// Terminate any remaining running services
	err = w.terminateServiceProcesses(ctx, allAgentServices)
	if err != nil {
		return err
	}

	log.Infof("All Datadog Agent services have been stopped successfully")
	return nil
}

func (w *WinServiceManager) terminateServiceProcesses(ctx context.Context, serviceNames []string) (err error) {
	var failedServices []error
	for _, serviceName := range serviceNames {
		log.Debugf("Ensuring service %s is stopped", serviceName)

		running, err := w.api.IsServiceRunning(serviceName)
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
		err = w.terminateServiceProcess(ctx, serviceName)
		if err != nil {
			// Check if service is actually stopped despite the termination error
			running, runningErr := w.api.IsServiceRunning(serviceName)
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

	return nil
}

// StartAgentServices starts the agent services
func (w *WinServiceManager) StartAgentServices(ctx context.Context) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "start_agent_service")
	defer func() { span.Finish(err) }()
	log.Infof("Starting datadogagent service")

	err = w.api.StartService("datadogagent")
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
