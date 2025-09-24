// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package fleetstatusimpl

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	daemoncheckerMock "github.com/DataDog/datadog-agent/comp/updater/daemonchecker/mock"
)

func TestFleetStatus(t *testing.T) {
	tests := []struct {
		name                   string
		remoteUpdatesConfig    bool
		installerRunning       bool
		fleetAutomationEnabled bool
	}{
		{
			name:                "remote updates disabled",
			remoteUpdatesConfig: false,
		},
		{
			name:                "remote updates enabled",
			remoteUpdatesConfig: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expectedStatus := map[string]interface{}{
				"fleetAutomationStatus": map[string]interface{}{
					"remoteManagementEnabled": tt.remoteUpdatesConfig,
					"installerRunning":        true,
					"fleetAutomationEnabled":  tt.remoteUpdatesConfig,
				},
			}

			cfg := config.NewMock(t)
			cfg.SetWithoutSource("remote_updates", tt.remoteUpdatesConfig)

			daemonChecker := daemoncheckerMock.Mock(t)

			provides := NewComponent(Requires{
				Config:        cfg,
				DaemonChecker: daemonChecker,
			})
			statusProvider := provides.Status.Provider

			stats := make(map[string]interface{})
			err := statusProvider.JSON(false, stats)
			require.NoError(t, err)
			assert.Equal(t, expectedStatus, stats)

			buffer := new(bytes.Buffer)
			err = statusProvider.Text(false, buffer)
			fmt.Println(buffer.String())
			require.NoError(t, err)
			assert.NotEmpty(t, buffer.String())
			buffer.Reset()

			err = statusProvider.HTML(false, buffer)
			require.NoError(t, err)
			assert.NotEmpty(t, buffer.String())
		})
	}
}
