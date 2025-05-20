// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package fleetstatusimpl

import (
	"bytes"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFleetStatus(t *testing.T) {
	tests := []struct {
		name                string
		remoteUpdatesConfig bool
		expectedStatus      map[string]interface{}
	}{
		{
			name:                "remote updates enabled",
			remoteUpdatesConfig: true,
			expectedStatus: map[string]interface{}{
				"fleetAutomationStatus": map[string]interface{}{
					"remoteManagementEnabled": true,
				},
			},
		},
		{
			name:                "remote updates disabled",
			remoteUpdatesConfig: false,
			expectedStatus: map[string]interface{}{
				"fleetAutomationStatus": map[string]interface{}{
					"remoteManagementEnabled": false,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.NewMock(t)
			cfg.SetWithoutSource("remote_updates", tt.remoteUpdatesConfig)

			provides := NewComponent(Requires{
				Config: cfg,
			})
			statusProvider := provides.Status.Provider

			stats := make(map[string]interface{})
			err := statusProvider.JSON(false, stats)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, stats)

			buffer := new(bytes.Buffer)
			err = statusProvider.Text(false, buffer)
			require.NoError(t, err)
			if tt.remoteUpdatesConfig {
				assert.Contains(t, buffer.String(), "Fleet Management is enabled")
			} else {
				assert.Contains(t, buffer.String(), "Fleet Management is disabled")
			}
			buffer.Reset()

			err = statusProvider.HTML(false, buffer)
			require.NoError(t, err)
			if tt.remoteUpdatesConfig {
				assert.Contains(t, buffer.String(), "Fleet Management is enabled")
			} else {
				assert.Contains(t, buffer.String(), "Fleet Management is disabled")
			}
		})
	}
}
