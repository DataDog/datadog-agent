// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package windows

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
)

// Test functions

func TestWinServiceManager_TerminateServiceProcess(t *testing.T) {
	tests := []struct {
		name          string
		serviceName   string
		expectError   bool
		errorContains string
		setupMocks    func(*mockSystemAPI)
	}{
		{
			name:        "service not running",
			serviceName: "testservice",
			expectError: false,
			setupMocks: func(api *mockSystemAPI) {
				api.On("GetServiceProcessID", "testservice").Return(uint32(0), nil)
			},
		},
		{
			name:        "service does not exist",
			serviceName: "nonexistent",
			expectError: false,
			setupMocks: func(api *mockSystemAPI) {
				api.On("GetServiceProcessID", "nonexistent").Return(uint32(0), windows.ERROR_SERVICE_DOES_NOT_EXIST)
			},
		},
		{
			name:        "successful termination",
			serviceName: "testservice",
			expectError: false,
			setupMocks: func(api *mockSystemAPI) {
				pid := uint32(1234)
				proc := windows.Handle(0x80000000 | pid)
				// First call to get PID
				api.On("GetServiceProcessID", "testservice").Return(pid, nil).Once()
				// Open process
				api.On("OpenProcess", mock.Anything, false, pid).Return(proc, nil)
				api.On("CloseHandle", proc).Return(nil)
				// Second call to verify PID (same PID)
				api.On("GetServiceProcessID", "testservice").Return(pid, nil).Once()
				// Terminate and wait
				api.On("TerminateProcess", proc, uint32(1)).Return(nil)
				api.On("WaitForSingleObject", proc, mock.Anything).Return(uint32(windows.WAIT_OBJECT_0), nil)
			},
		},
		{
			name:          "process termination fails",
			serviceName:   "testservice",
			expectError:   true,
			errorContains: "could not terminate process",
			setupMocks: func(api *mockSystemAPI) {
				pid := uint32(1234)
				proc := windows.Handle(0x80000000 | pid)
				// First call to get PID
				api.On("GetServiceProcessID", "testservice").Return(pid, nil).Once()
				// Open process
				api.On("OpenProcess", mock.Anything, false, pid).Return(proc, nil)
				api.On("CloseHandle", proc).Return(nil)
				// Second call to verify PID (same PID)
				api.On("GetServiceProcessID", "testservice").Return(pid, nil).Once()
				// Terminate fails
				api.On("TerminateProcess", proc, uint32(1)).Return(errors.New("access denied"))
			},
		},
		{
			name:          "service PID changes during termination",
			serviceName:   "testservice",
			expectError:   true,
			errorContains: "process ID for service testservice changed from 1234 to 5678, aborting termination",
			setupMocks: func(api *mockSystemAPI) {
				pid := uint32(1234)
				proc := windows.Handle(0x80000000 | pid)
				// First call to get PID
				api.On("GetServiceProcessID", "testservice").Return(pid, nil).Once()
				// Open process
				api.On("OpenProcess", mock.Anything, false, pid).Return(proc, nil)
				api.On("CloseHandle", proc).Return(nil)
				// Second call to verify PID (PID has changed - race condition)
				api.On("GetServiceProcessID", "testservice").Return(uint32(5678), nil).Once() // new PID
				// Should not proceed with termination since PID changed
			},
		},
		{
			name:        "service stopped during termination",
			serviceName: "testservice",
			expectError: false,
			setupMocks: func(api *mockSystemAPI) {
				pid := uint32(1234)
				proc := windows.Handle(0x80000000 | pid)
				// First call to get PID
				api.On("GetServiceProcessID", "testservice").Return(pid, nil).Once()
				// Open process
				api.On("OpenProcess", mock.Anything, false, pid).Return(proc, nil)
				api.On("CloseHandle", proc).Return(nil)
				// Second call to verify PID (service stopped)
				api.On("GetServiceProcessID", "testservice").Return(uint32(0), nil).Once()
			},
		},
		{
			name:          "process termination succeeds but wait times out",
			serviceName:   "testservice",
			expectError:   true,
			errorContains: "timeout waiting for process 1234 for service testservice to exit",
			setupMocks: func(api *mockSystemAPI) {
				pid := uint32(1234)
				proc := windows.Handle(0x80000000 | pid)
				// First call to get PID
				api.On("GetServiceProcessID", "testservice").Return(pid, nil).Once()
				// Open process
				api.On("OpenProcess", mock.Anything, false, pid).Return(proc, nil)
				api.On("CloseHandle", proc).Return(nil)
				// Second call to verify PID (same PID)
				api.On("GetServiceProcessID", "testservice").Return(pid, nil).Once()
				// Terminate succeeds
				api.On("TerminateProcess", proc, uint32(1)).Return(nil)
				// Wait times out
				api.On("WaitForSingleObject", proc, mock.Anything).Return(uint32(windows.WAIT_TIMEOUT), nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := &mockSystemAPI{}

			tt.setupMocks(mockAPI)

			manager := NewWinServiceManagerWithAPI(mockAPI)

			ctx := context.Background()
			err := manager.terminateServiceProcess(ctx, tt.serviceName)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			mockAPI.AssertExpectations(t)
		})
	}
}

func TestWinServiceManager_StopAllAgentServices(t *testing.T) {
	tests := []struct {
		name          string
		expectError   bool
		errorContains string
		setupMocks    func(*mockSystemAPI)
	}{
		{
			name:        "main service does not exist",
			expectError: false,
			setupMocks: func(api *mockSystemAPI) {
				api.On("StopService", "datadogagent").Return(windows.ERROR_SERVICE_DOES_NOT_EXIST)
			},
		},
		{
			name:        "main service stop fails but continues with termination loop",
			expectError: false,
			setupMocks: func(api *mockSystemAPI) {
				// Initial StopService fails with a generic error (not "does not exist")
				api.On("StopService", "datadogagent").Return(errors.New("access denied"))

				// Function should continue and check all services in termination loop
				serviceNames := []string{
					"datadog-otel-agent",
					"dd-procmgr-service",
					"datadog-trace-agent",
					"datadog-process-agent",
					"datadog-security-agent",
					"datadog-system-probe",
					"datadog-agent-action",
					"Datadog Installer",
					"datadogagent",
				}
				for _, name := range serviceNames {
					api.On("GetServiceState", name).Return(svc.Stopped, nil)
				}
			},
		},
		{
			name:        "successful stop all services",
			expectError: false,
			setupMocks: func(api *mockSystemAPI) {
				api.On("StopService", "datadogagent").Return(nil)

				// All services are not running
				serviceNames := []string{
					"datadog-otel-agent",
					"dd-procmgr-service",
					"datadog-trace-agent",
					"datadog-process-agent",
					"datadog-security-agent",
					"datadog-system-probe",
					"datadog-agent-action",
					"Datadog Installer",
					"datadogagent",
				}
				for _, name := range serviceNames {
					api.On("GetServiceState", name).Return(svc.Stopped, nil)
				}
			},
		},
		{
			name:        "successful termination of running services",
			expectError: false,
			setupMocks: func(api *mockSystemAPI) {
				api.On("StopService", "datadogagent").Return(nil)

				// Two services are running and need termination
				runningServices := []string{"datadog-trace-agent", "datadog-system-probe"}
				for _, serviceName := range runningServices {
					// Service is initially running
					api.On("GetServiceState", serviceName).Return(svc.Running, nil).Once()

					// Successful termination process
					pid := uint32(1234)
					p := windows.Handle(0x80000000 | pid)
					api.On("GetServiceProcessID", serviceName).Return(pid, nil).Once() // First call
					api.On("OpenProcess", mock.Anything, false, pid).Return(p, nil)
					api.On("CloseHandle", p).Return(nil)
					api.On("GetServiceProcessID", serviceName).Return(pid, nil).Once() // Verification call
					api.On("TerminateProcess", p, uint32(1)).Return(nil)
					api.On("WaitForSingleObject", p, mock.Anything).Return(uint32(windows.WAIT_OBJECT_0), nil)
				}

				// Other services are not running
				otherServices := []string{
					"datadog-otel-agent",
					"dd-procmgr-service",
					"datadog-process-agent",
					"datadog-security-agent",
					"datadog-agent-action",
					"Datadog Installer",
					"datadogagent",
				}
				for _, name := range otherServices {
					api.On("GetServiceState", name).Return(svc.Stopped, nil)
				}
			},
		},
		{
			name:        "successful termination of pending services",
			expectError: false,
			setupMocks: func(api *mockSystemAPI) {
				api.On("StopService", "datadogagent").Return(nil)

				// Two services are running and need termination
				runningServices := []string{"datadog-trace-agent", "datadog-system-probe"}
				for _, serviceName := range runningServices {
					// Service is stuck in stop pending state
					api.On("GetServiceState", serviceName).Return(svc.StopPending, nil).Once()

					// Successful termination process
					pid := uint32(1234)
					p := windows.Handle(0x80000000 | pid)
					api.On("GetServiceProcessID", serviceName).Return(pid, nil).Once() // First call
					api.On("OpenProcess", mock.Anything, false, pid).Return(p, nil)
					api.On("CloseHandle", p).Return(nil)
					api.On("GetServiceProcessID", serviceName).Return(pid, nil).Once() // Verification call
					api.On("TerminateProcess", p, uint32(1)).Return(nil)
					api.On("WaitForSingleObject", p, mock.Anything).Return(uint32(windows.WAIT_OBJECT_0), nil)
				}

				// Other services are not running
				otherServices := []string{
					"datadog-otel-agent",
					"dd-procmgr-service",
					"datadog-process-agent",
					"datadog-security-agent",
					"datadog-agent-action",
					"Datadog Installer",
					"datadogagent",
				}
				for _, name := range otherServices {
					api.On("GetServiceState", name).Return(svc.Stopped, nil)
				}
			},
		},
		{
			name:          "some services running but termination fails",
			expectError:   true,
			errorContains: "failed to stop services",
			setupMocks: func(api *mockSystemAPI) {
				api.On("StopService", "datadogagent").Return(nil)

				// First service is running but termination fails
				api.On("GetServiceState", "datadog-trace-agent").Return(svc.Running, nil)
				// Mock the GetServiceProcessID calls within TerminateServiceProcess
				api.On("GetServiceProcessID", "datadog-trace-agent").Return(uint32(1234), errors.New("access denied"))

				// Other services are not running
				serviceNames := []string{
					"datadog-otel-agent",
					"dd-procmgr-service",
					"datadog-process-agent",
					"datadog-security-agent",
					"datadog-system-probe",
					"datadog-agent-action",
					"Datadog Installer",
					"datadogagent",
				}
				for _, name := range serviceNames {
					api.On("GetServiceState", name).Return(svc.Stopped, nil)
				}
				// Still check if first service is running after failed termination
				api.On("GetServiceState", "datadog-trace-agent").Return(svc.Running, nil)
			},
		},
		{
			name:        "some services running but termination fails and services stopped anyway",
			expectError: false,
			setupMocks: func(api *mockSystemAPI) {
				api.On("StopService", "datadogagent").Return(nil)

				// First service is running but termination fails
				api.On("GetServiceState", "datadog-trace-agent").Return(svc.Running, nil).Once()
				// Mock the GetServiceProcessID calls within TerminateServiceProcess
				api.On("GetServiceProcessID", "datadog-trace-agent").Return(uint32(1234), errors.New("access denied"))
				// after failed termination, the service ended up being stopped
				api.On("GetServiceState", "datadog-trace-agent").Return(svc.Stopped, nil).Once()

				// Other services are not running
				serviceNames := []string{
					"datadog-otel-agent",
					"dd-procmgr-service",
					"datadog-process-agent",
					"datadog-security-agent",
					"datadog-system-probe",
					"datadog-agent-action",
					"Datadog Installer",
					"datadogagent",
				}
				for _, name := range serviceNames {
					api.On("GetServiceState", name).Return(svc.Stopped, nil)
				}

			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := &mockSystemAPI{}
			tt.setupMocks(mockAPI)

			manager := NewWinServiceManagerWithAPI(mockAPI)

			ctx := context.Background()
			err := manager.StopAllAgentServices(ctx)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			mockAPI.AssertExpectations(t)
		})
	}
}

func TestWinServiceManager_RestartAgentServices(t *testing.T) {
	t.Run("RestartAgentServices success", func(t *testing.T) {
		mockAPI := &mockSystemAPI{}
		// Stop services
		mockAPI.On("StopService", "datadogagent").Return(nil)
		serviceNames := []string{
			"datadog-otel-agent",
			"dd-procmgr-service",
			"datadog-trace-agent",
			"datadog-process-agent",
			"datadog-security-agent",
			"datadog-system-probe",
			"datadog-agent-action",
			"Datadog Installer",
			"datadogagent",
		}
		for _, name := range serviceNames {
			mockAPI.On("GetServiceState", name).Return(svc.Stopped, nil)
		}
		// Start services
		mockAPI.On("StartService", "datadogagent").Return(nil)

		manager := NewWinServiceManagerWithAPI(mockAPI)

		ctx := context.Background()
		err := manager.RestartAgentServices(ctx)

		assert.NoError(t, err)
		mockAPI.AssertExpectations(t)
	})

	t.Run("RestartAgentServices continues with start even when stop fails", func(t *testing.T) {
		mockAPI := &mockSystemAPI{}

		// Stop services - set up a failure scenario
		mockAPI.On("StopService", "datadogagent").Return(nil)
		// First service is running but termination fails
		mockAPI.On("GetServiceState", "datadog-trace-agent").Return(svc.Running, nil)
		// Mock the GetServiceProcessID to fail, causing termination to fail
		mockAPI.On("GetServiceProcessID", "datadog-trace-agent").Return(uint32(1234), errors.New("access denied"))
		// Check if service is still running after failed termination
		mockAPI.On("GetServiceState", "datadog-trace-agent").Return(svc.Running, nil)

		// Other services are not running
		serviceNames := []string{
			"datadog-otel-agent",
			"dd-procmgr-service",
			"datadog-process-agent",
			"datadog-security-agent",
			"datadog-system-probe",
			"datadog-agent-action",
			"Datadog Installer",
			"datadogagent",
		}
		for _, name := range serviceNames {
			mockAPI.On("GetServiceState", name).Return(svc.Stopped, nil)
		}

		// Start services - this should still be called and succeed
		mockAPI.On("StartService", "datadogagent").Return(nil)

		manager := NewWinServiceManagerWithAPI(mockAPI)

		ctx := context.Background()
		err := manager.RestartAgentServices(ctx)

		// Should succeed overall because start succeeded, even though stop failed
		assert.NoError(t, err, "RestartAgentServices should succeed when start succeeds, even if stop fails")
		mockAPI.AssertExpectations(t)
	})
}

func TestWinServiceManager_DeleteAgentServices(t *testing.T) {
	// Keep this list explicit so a newly retained MSI service needs test coverage.
	services := []string{
		"datadog-trace-agent", "datadog-process-agent", "datadog-security-agent",
		"datadog-system-probe", "datadog-agent-action", "dd-procmgr-service",
		"Datadog Installer", "datadogagent",
	}
	t.Run("repeated purge", func(t *testing.T) {
		api := &mockSystemAPI{}
		for _, name := range services {
			api.On("GetServiceState", name).Return(svc.Stopped, nil).Once()
			api.On("DeleteService", name).Return(nil).Once()
			api.On("GetServiceState", name).Return(svc.Stopped, windows.ERROR_SERVICE_DOES_NOT_EXIST).Once()
		}
		manager := NewWinServiceManagerWithAPI(api)
		assert.NoError(t, manager.DeleteAgentServices(context.Background()))
		assert.NoError(t, manager.DeleteAgentServices(context.Background()))
		api.AssertExpectations(t)
	})

	for _, tt := range []struct {
		name      string
		queryErr  error
		running   bool
		stopErr   error
		deleteErr error
		wantErr   error
	}{
		{name: "retained stopped services skip dependent enumeration"},
		{name: "running service is stopped", running: true},
		{name: "query failure continues with remaining services", queryErr: windows.ERROR_ACCESS_DENIED, wantErr: windows.ERROR_ACCESS_DENIED},
		{name: "service already absent", queryErr: windows.ERROR_SERVICE_DOES_NOT_EXIST},
		{name: "service already marked for deletion", queryErr: windows.ERROR_SERVICE_MARKED_FOR_DELETE},
		{name: "service disappears before deletion", deleteErr: windows.ERROR_SERVICE_DOES_NOT_EXIST},
		{name: "service marked for deletion before deletion", deleteErr: windows.ERROR_SERVICE_MARKED_FOR_DELETE},
		{name: "stop failure continues with remaining services", running: true, stopErr: windows.ERROR_ACCESS_DENIED, wantErr: windows.ERROR_ACCESS_DENIED},
		{name: "delete failure continues with remaining services", deleteErr: windows.ERROR_ACCESS_DENIED, wantErr: windows.ERROR_ACCESS_DENIED},
	} {
		t.Run(tt.name, func(t *testing.T) {
			api := &mockSystemAPI{}
			for i, name := range services {
				state := svc.Stopped
				queryErr, stopErr, deleteErr := error(nil), error(nil), error(nil)
				if i == 0 {
					queryErr, stopErr, deleteErr = tt.queryErr, tt.stopErr, tt.deleteErr
					if tt.running {
						state = svc.Running
					}
				}
				api.On("GetServiceState", name).Return(state, queryErr).Once()
				if queryErr != nil {
					continue
				}
				if state != svc.Stopped {
					api.On("StopService", name).Return(stopErr).Once()
				}
				if stopErr == nil {
					api.On("DeleteService", name).Return(deleteErr).Once()
				}
			}
			err := NewWinServiceManagerWithAPI(api).DeleteAgentServices(context.Background())
			if tt.wantErr == nil {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.ErrorContains(t, err, services[0])
			}
			api.AssertExpectations(t)
		})
	}
}
