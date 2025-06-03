// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package fleetstatusimpl

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	daemonchecker "github.com/DataDog/datadog-agent/comp/daemonchecker/def"
	daemoncheckerMock "github.com/DataDog/datadog-agent/comp/daemonchecker/mock"
	"github.com/DataDog/datadog-agent/pkg/util/option"
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
			installerRunning:    true,
		},
		{
			name:                "installer not running",
			remoteUpdatesConfig: true,
			installerRunning:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expectedStatus := map[string]interface{}{
				"fleetAutomationStatus": map[string]interface{}{
					"remoteManagementEnabled": tt.remoteUpdatesConfig,
					"remoteConfigEnabled":     false,
					"installerRunning":        tt.installerRunning,
					"fleetAutomationEnabled":  false,
				},
			}

			cfg := config.NewMock(t)
			cfg.SetWithoutSource("remote_updates", tt.remoteUpdatesConfig)

			var daemonCheckerOption option.Option[daemonchecker.Component]
			if tt.installerRunning {
				daemonCheckerOption = option.New(daemoncheckerMock.Mock(t))
			} else {
				daemonCheckerOption = option.None[daemonchecker.Component]()
			}

			provides := NewComponent(Requires{
				Config:        cfg,
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
			assert.Contains(t, buffer.String(), "Fleet Management is disabled")
			buffer.Reset()

			err = statusProvider.HTML(false, buffer)
			require.NoError(t, err)
			assert.Contains(t, buffer.String(), "Fleet Management is disabled")
		})
	}
}
