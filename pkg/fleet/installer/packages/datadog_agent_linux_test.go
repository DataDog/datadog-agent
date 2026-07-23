// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	installerErrors "github.com/DataDog/datadog-agent/pkg/fleet/installer/errors"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestWarnIfLegacyProcmgrRetirementFailsLogsWarning(t *testing.T) {
	logs := setupInstallerLogCapture(t)
	accessDeniedErr := errors.New("Failed to stop datadog-agent-procmgrd.service: Access denied\n\nexit status 4")

	warnIfLegacyProcmgrRetirementFails(testHookContext(), func(HookContext) error {
		return accessDeniedErr
	})

	assert.Contains(t, logs.String(), "failed to retire legacy procmgr units")
	assert.Contains(t, logs.String(), "continuing with Agent service installation")
	assert.Contains(t, logs.String(), "Access denied")
	assert.Contains(t, logs.String(), "exit status 4")
}

func TestPostInstallDatadogAgentContinuesAfterLegacyProcmgrRetirementFailure(t *testing.T) {
	logs := setupInstallerLogCapture(t)
	service := &fakeDatadogAgentServiceManager{}
	deps := testPostInstallDeps(service)
	deps.installFilesystem = func(ctx HookContext) error {
		warnIfLegacyProcmgrRetirementFails(ctx, func(HookContext) error {
			return errors.New("Failed to stop datadog-agent-procmgrd.service: Access denied\n\nexit status 4")
		})
		return nil
	}

	err := postInstallDatadogAgentWithDeps(testHookContext(), deps)

	require.NoError(t, err)
	assert.Equal(t, []string{"WriteStable", "EnableStable", "RestartStable"}, service.calls)
	assert.Contains(t, logs.String(), "failed to retire legacy procmgr units")
	assert.Contains(t, logs.String(), "Access denied")
}

func TestPostInstallDatadogAgentStableServiceFailuresRemainFatal(t *testing.T) {
	tests := []struct {
		name      string
		service   *fakeDatadogAgentServiceManager
		wantCalls []string
		wantErr   string
	}{
		{
			name: "write stable failure",
			service: &fakeDatadogAgentServiceManager{
				writeStableErr: errors.New("failed to write unit"),
			},
			wantCalls: []string{"WriteStable"},
			wantErr:   "failed to write stable units: failed to write unit",
		},
		{
			name: "enable stable failure",
			service: &fakeDatadogAgentServiceManager{
				enableStableErr: errors.New("systemctl enable failed"),
			},
			wantCalls: []string{"WriteStable", "EnableStable"},
			wantErr:   "failed to install stable unit: systemctl enable failed",
		},
		{
			name: "restart stable failure",
			service: &fakeDatadogAgentServiceManager{
				restartStableErr: errors.New("systemctl restart failed"),
			},
			wantCalls: []string{"WriteStable", "EnableStable", "RestartStable"},
			wantErr:   "failed to restart stable unit: systemctl restart failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := testPostInstallDeps(tt.service)

			err := postInstallDatadogAgentWithDeps(testHookContext(), deps)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Equal(t, installerErrors.ErrFilesystemIssue, installerErrors.GetCode(err))
			assert.Contains(t, installerErrors.ToJSON(err), `"code":4`)
			assert.Equal(t, tt.wantCalls, tt.service.calls)
		})
	}
}

func setupInstallerLogCapture(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	logger, err := log.LoggerFromWriterWithMinLevelAndMsgFormat(&logs, log.DebugLvl)
	require.NoError(t, err)
	log.SetupLogger(logger, log.DebugStr)
	t.Cleanup(func() {
		log.SetupLogger(log.Disabled(), log.OffStr)
	})
	return &logs
}

func testHookContext() HookContext {
	return HookContext{
		Context:     context.Background(),
		Package:     agentPackage,
		PackageType: PackageTypeRPM,
		PackagePath: "/opt/datadog-agent",
		Hook:        "postInstall",
		Upgrade:     true,
	}
}

func testPostInstallDeps(service datadogAgentServiceManager) datadogAgentPostInstallDeps {
	return datadogAgentPostInstallDeps{
		installFilesystem: func(HookContext) error {
			return nil
		},
		restoreCustomIntegrations: func(context.Context, string) error {
			return nil
		},
		fixRestoredIntegrationOwnership: func(HookContext) error {
			return nil
		},
		restoreODBCConfig: func(string) error {
			return nil
		},
		getCurrentAgentVersion: func() string {
			return "7.81.1-1"
		},
		setPackage: func(context.Context, string, string, bool) error {
			return nil
		},
		restoreAgentExtensions: func(HookContext, string, bool) error {
			return nil
		},
		installAgentExtensions: func(HookContext, string, bool) error {
			return nil
		},
		writeDDOTProcmgrConfig: func(string) error {
			return nil
		},
		service: service,
	}
}

type fakeDatadogAgentServiceManager struct {
	calls            []string
	writeStableErr   error
	enableStableErr  error
	restartStableErr error
}

func (s *fakeDatadogAgentServiceManager) EnableStable(HookContext) error {
	s.calls = append(s.calls, "EnableStable")
	return s.enableStableErr
}

func (s *fakeDatadogAgentServiceManager) DisableStable(HookContext) error {
	s.calls = append(s.calls, "DisableStable")
	return nil
}

func (s *fakeDatadogAgentServiceManager) RestartStable(HookContext) error {
	s.calls = append(s.calls, "RestartStable")
	return s.restartStableErr
}

func (s *fakeDatadogAgentServiceManager) StopStable(HookContext) error {
	s.calls = append(s.calls, "StopStable")
	return nil
}

func (s *fakeDatadogAgentServiceManager) WriteStable(HookContext) error {
	s.calls = append(s.calls, "WriteStable")
	return s.writeStableErr
}

func (s *fakeDatadogAgentServiceManager) RemoveStable(HookContext) error {
	s.calls = append(s.calls, "RemoveStable")
	return nil
}

func (s *fakeDatadogAgentServiceManager) StartExperiment(HookContext) error {
	s.calls = append(s.calls, "StartExperiment")
	return nil
}

func (s *fakeDatadogAgentServiceManager) StopExperiment(HookContext) error {
	s.calls = append(s.calls, "StopExperiment")
	return nil
}

func (s *fakeDatadogAgentServiceManager) WriteExperiment(HookContext) error {
	s.calls = append(s.calls, "WriteExperiment")
	return nil
}

func (s *fakeDatadogAgentServiceManager) RemoveExperiment(HookContext) error {
	s.calls = append(s.calls, "RemoveExperiment")
	return nil
}
