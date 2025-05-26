// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package fleetstatusimpl

import (
	"bytes"
	"context"
	"expvar"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	daemonchecker "github.com/DataDog/datadog-agent/comp/daemonchecker/def"
	daemoncheckerMock "github.com/DataDog/datadog-agent/comp/daemonchecker/mock"
	installerexec "github.com/DataDog/datadog-agent/comp/updater/installerexec/def"
	installerexecmock "github.com/DataDog/datadog-agent/comp/updater/installerexec/mock"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func TestFleetStatus(t *testing.T) {
	rcMapStatus := expvar.NewMap("remoteConfigStatus")
	rcEnabled := expvar.String{}
	rcEnabled.Set("enabled")
	rcMapStatus.Set("enabled", &rcEnabled)

	tests := []struct {
		name                   string
		remoteUpdatesConfig    bool
		installerRunning       bool
		fleetAutomationEnabled bool
	}{
		{
			name:                   "fleet enabled",
			remoteUpdatesConfig:    true,
			installerRunning:       true,
			fleetAutomationEnabled: true,
		},
		{
			name:                   "remote updates disabled",
			remoteUpdatesConfig:    false,
			installerRunning:       true,
			fleetAutomationEnabled: false,
		},
		{
			name:                   "installer not running",
			remoteUpdatesConfig:    true,
			installerRunning:       false,
			fleetAutomationEnabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expectedStatus := map[string]interface{}{
				"fleetAutomationStatus": map[string]interface{}{
					"remoteManagementEnabled": tt.remoteUpdatesConfig,
					"remoteConfigEnabled":     true,
					"installerRunning":        tt.installerRunning,
					"fleetAutomationEnabled":  tt.installerRunning && tt.remoteUpdatesConfig,
				},
			}

			cfg := config.NewMock(t)
			cfg.SetWithoutSource("remote_updates", tt.remoteUpdatesConfig)

			var installerExecOption option.Option[installerexec.Component]
			var daemonCheckerOption option.Option[daemonchecker.Component]
			if tt.installerRunning {
				installerExecOption = option.New(installerexecmock.Mock(t))
				mockDaemonChecker := daemoncheckerMock.Mock(t)
				daemonCheckerOption = option.New(mockDaemonChecker)
				bli, _ := mockDaemonChecker.IsRunning(context.Background())
				fmt.Println("isrunning", bli)
			} else {
				installerExecOption = option.None[installerexec.Component]()
				daemonCheckerOption = option.None[daemonchecker.Component]()
			}

			provides := NewComponent(Requires{
				Config:        cfg,
				InstallerExec: installerExecOption,
				DaemonChecker: daemonCheckerOption,
			})
			statusProvider := provides.Status.Provider

			stats := make(map[string]interface{})
			err := statusProvider.JSON(false, stats)
			require.NoError(t, err)
			assert.Equal(t, expectedStatus, stats)

			buffer := new(bytes.Buffer)
			err = statusProvider.Text(false, buffer)
			require.NoError(t, err)
			if tt.fleetAutomationEnabled {
				assert.Contains(t, buffer.String(), "Fleet Management is enabled")
			} else {
				assert.Contains(t, buffer.String(), "Fleet Management is disabled")
			}
			buffer.Reset()

			err = statusProvider.HTML(false, buffer)
			require.NoError(t, err)
			if tt.fleetAutomationEnabled {
				assert.Contains(t, buffer.String(), "Fleet Management is enabled")
			} else {
				assert.Contains(t, buffer.String(), "Fleet Management is disabled")
			}
		})
	}
}
