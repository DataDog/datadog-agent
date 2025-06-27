//go:build windows

package packages

import (
	"context"
)

// ServiceManager interface abstracts all service management operations
type ServiceManager interface {
	GetServiceProcessID(serviceName string) (uint32, error)
	TerminateServiceProcess(ctx context.Context, serviceName string) error
	StopAllAgentServices(ctx context.Context) error
	StartAgentServices(ctx context.Context) error
	RestartAgentServices(ctx context.Context) error
	IsServiceRunning(serviceName string) (bool, error)
	StopService(serviceName string) error
	StartService(serviceName string) error
}

// SystemAPI abstracts system-level API calls
type SystemAPI interface {
	GetServiceProcessID(serviceName string) (uint32, error)
	IsServiceRunning(serviceName string) (bool, error)
	StopService(serviceName string) error
	StartService(serviceName string) error
	OpenProcess(desiredAccess uint32, inheritHandle bool, processID uint32) (ProcessHandle, error)
	TerminateProcess(handle ProcessHandle, exitCode uint32) error
	WaitForSingleObject(handle ProcessHandle, timeoutMs uint32) (uint32, error)
	CloseHandle(handle ProcessHandle) error
}

// ProcessHandle abstracts process handle operations
type ProcessHandle interface {
	// This is just a marker interface - actual handle operations are done through SystemAPI
}
