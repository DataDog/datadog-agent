// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package fleetstatusimpl

import (
	"bytes"
	"expvar"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	daemonchecker "github.com/DataDog/datadog-agent/comp/daemonchecker/def"
	daemoncheckerMock "github.com/DataDog/datadog-agent/comp/daemonchecker/mock"
	ssistatus "github.com/DataDog/datadog-agent/comp/updater/ssistatus/def"
	ssistatusmock "github.com/DataDog/datadog-agent/comp/updater/ssistatus/mock"
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

			var daemonCheckerOption option.Option[daemonchecker.Component]
			ssisStatusProviderOption := option.None[ssistatus.Component]()
			if tt.installerRunning {
				daemonCheckerOption = option.New(daemoncheckerMock.Mock(t))
			} else {
				daemonCheckerOption = option.None[daemonchecker.Component]()
			}

			provides := NewComponent(Requires{
				Config:            cfg,
				SsiStatusProvider: ssisStatusProviderOption,
				DaemonChecker:     daemonCheckerOption,
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

func TestFleetStatusWithSSI(t *testing.T) {
	rcMapStatus := expvar.NewMap("remoteConfigStatus")
	rcEnabled := expvar.String{}
	rcEnabled.Set("enabled")
	rcMapStatus.Set("enabled", &rcEnabled)

	tests := []struct {
		name                               string
		autoInstrumentationStatusAvailable bool
		autoInstrumentationEnabled         bool
	}{
		{
			name:                               "SSI status unavailable",
			autoInstrumentationStatusAvailable: false,
		},

		{
			name:                               "SSI status available",
			autoInstrumentationStatusAvailable: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			expectedStatus := map[string]interface{}{
				"fleetAutomationStatus": map[string]interface{}{
					"remoteManagementEnabled": true,
					"remoteConfigEnabled":     true,
					"installerRunning":        true,
					"fleetAutomationEnabled":  true,
				},
			}
			if tt.autoInstrumentationStatusAvailable {
				ssiStatus := map[string]bool{
					"autoInstrumentationEnabled": true,
					"hostInstrumented":           true,
					"dockerInstrumented":         false,
				}
				expectedStatus["fleetAutomationStatus"].(map[string]interface{})["ssiStatus"] = ssiStatus
			}

			cfg := config.NewMock(t)
			cfg.SetWithoutSource("remote_updates", true)
			daemonCheckerOption := option.New(daemoncheckerMock.Mock(t))
			var ssisStatusProviderOption option.Option[ssistatus.Component]
			if tt.autoInstrumentationStatusAvailable {
				ssisStatusProviderOption = option.New(ssistatusmock.WithInstrumentationModes(t, []string{"host"}))
			} else {
				ssisStatusProviderOption = option.None[ssistatus.Component]()
			}

			provides := NewComponent(Requires{
				Config:            cfg,
				SsiStatusProvider: ssisStatusProviderOption,
				DaemonChecker:     daemonCheckerOption,
			})
			statusProvider := provides.Status.Provider

			stats := make(map[string]interface{})
			err := statusProvider.JSON(false, stats)
			require.NoError(t, err)
			assert.Equal(t, expectedStatus, stats)

			buffer := new(bytes.Buffer)
			err = statusProvider.Text(false, buffer)
			require.NoError(t, err)
			if tt.autoInstrumentationStatusAvailable {
				assert.Contains(t, buffer.String(), "Host:   Instrumented")
			} else {
				assert.Contains(t, buffer.String(), "APM status not available")
			}
			buffer.Reset()

			err = statusProvider.HTML(false, buffer)
			require.NoError(t, err)
			fmt.Println(buffer.String())
			if tt.autoInstrumentationStatusAvailable {
				assert.Contains(t, buffer.String(), "Host:   Instrumented")
			} else {
				assert.Contains(t, buffer.String(), "APM status not available")
			}
		})
	}
}
