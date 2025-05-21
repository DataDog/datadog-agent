// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package fleetstatusimpl

import (
	"bytes"
	"expvar"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	installerexec "github.com/DataDog/datadog-agent/comp/updater/installerexec/def"
	installerexecmock "github.com/DataDog/datadog-agent/comp/updater/installerexec/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFleetStatus(t *testing.T) {
	rcMapStatus := expvar.NewMap("remoteConfigStatus")
	rcEnabled    := expvar.String{}
	rcEnabled.Set("enabled")
	rcMapStatus.Set("enabled", &rcEnabled)

	tests := []struct {
		name                string
		remoteUpdatesConfig bool
		installerRunning    bool
		fleetAutomationEnabled bool
	}{
		{
			name:                "fleet enabled",
			remoteUpdatesConfig: true,
			installerRunning:    true,
			fleetAutomationEnabled: true,
		},
		{
			name:                "remote updates disabled",
			remoteUpdatesConfig: false,
			installerRunning:    true,
			fleetAutomationEnabled: false,
		},
		{
			name:                "instaler not running",
			remoteUpdatesConfig: true,
			installerRunning:    false,
			fleetAutomationEnabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expectedStatus:= map[string]interface{}{
				"fleetAutomationStatus": map[string]interface{}{
					"remoteManagementEnabled": tt.remoteUpdatesConfig,
					"remoteConfigEnabled":     true,
					"installerRunning":        tt.installerRunning,
					"fleetAutomationEnabled": tt.installerRunning && tt.remoteUpdatesConfig,
				},
			}

			cfg := config.NewMock(t)
			cfg.SetWithoutSource("remote_updates", tt.remoteUpdatesConfig)

			var installerExec installerexec.Component
			if tt.installerRunning {
				installerExec = installerexecmock.Mock(t)
			}

			provides := NewComponent(Requires{
				Config: cfg,
				InstallerExec: installerExec,
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
