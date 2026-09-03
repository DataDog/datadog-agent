// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package fleetstatusimpl

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	localapi "github.com/DataDog/datadog-agent/comp/updater/localapi/def"
	localapiclient "github.com/DataDog/datadog-agent/comp/updater/localapiclient/def"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

type testStatusClient struct {
	response localapi.StatusResponse
	err      error
}

func (c testStatusClient) Status() (localapi.StatusResponse, error) {
	return c.response, c.err
}

func newStatusProvider(t *testing.T, remoteUpdates bool, client localapiclient.StatusClient) statusProvider {
	cfg := config.NewMock(t)
	cfg.SetInTest("remote_updates", remoteUpdates)
	provides := NewComponent(Requires{Config: cfg, InstallerAPIClient: client})
	return *(provides.Status.Provider.(*statusProvider))
}

func TestFleetStatusJSONReachable(t *testing.T) {
	client := testStatusClient{response: localapi.StatusResponse{
		SecretsPubKey: "must-not-be-exposed",
		RemoteConfigState: []*pbgo.PackageState{
			{Package: "z-package", StableVersion: "1.0.0"},
			{Package: "a-package", RunningVersion: "2.0.0"},
		},
	}}
	provider := newStatusProvider(t, true, client)

	stats := make(map[string]interface{})
	require.NoError(t, provider.JSON(false, stats))
	got := stats["fleetAutomationStatus"].(fleetAutomationStatus)
	require.True(t, got.RemoteManagementEnabled)
	require.True(t, got.InstallerRunning)
	require.True(t, got.FleetAutomationEnabled)
	require.True(t, got.InstallerStatus.Reachable)
	require.Equal(t, []string{"a-package", "z-package"}, []string{
		got.InstallerStatus.Packages[0].Package,
		got.InstallerStatus.Packages[1].Package,
	})

	raw, err := json.Marshal(stats)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"fleetAutomationStatus": {
			"remoteManagementEnabled": true,
			"installerRunning": true,
			"fleetAutomationEnabled": true,
			"installerStatus": {
				"reachable": true,
				"packages": [
					{"package": "a-package", "running_version": "2.0.0"},
					{"package": "z-package", "stable_version": "1.0.0"}
				]
			}
		}
	}`, string(raw))
	assert.NotContains(t, string(raw), "secrets_pub_key")
	assert.NotContains(t, string(raw), "must-not-be-exposed")
}

func TestFleetStatusJSONDisabledRemoteUpdates(t *testing.T) {
	provider := newStatusProvider(t, false, testStatusClient{})

	stats := make(map[string]interface{})
	require.NoError(t, provider.JSON(false, stats))
	got := stats["fleetAutomationStatus"].(fleetAutomationStatus)
	assert.False(t, got.RemoteManagementEnabled)
	assert.True(t, got.InstallerRunning)
	assert.False(t, got.FleetAutomationEnabled)
	assert.True(t, got.InstallerStatus.Reachable)
	assert.Empty(t, got.InstallerStatus.Packages)
}

func TestFleetStatusUnreachableDoesNotFail(t *testing.T) {
	provider := newStatusProvider(t, true, testStatusClient{err: errors.New("connection refused")})

	stats := make(map[string]interface{})
	require.NoError(t, provider.JSON(false, stats))
	got := stats["fleetAutomationStatus"].(fleetAutomationStatus)
	assert.False(t, got.InstallerRunning)
	assert.False(t, got.FleetAutomationEnabled)
	assert.False(t, got.InstallerStatus.Reachable)
	assert.Empty(t, got.InstallerStatus.Packages)
	assert.Equal(t, "connection refused", got.InstallerStatus.Error)

	raw, err := json.Marshal(stats)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"packages":[]`)

	for _, render := range []func(bool, *bytes.Buffer) error{
		func(verbose bool, buffer *bytes.Buffer) error { return provider.Text(verbose, buffer) },
		func(verbose bool, buffer *bytes.Buffer) error { return provider.HTML(verbose, buffer) },
	} {
		buffer := new(bytes.Buffer)
		require.NoError(t, render(false, buffer))
		assert.Contains(t, buffer.String(), "Unreachable")
		assert.Contains(t, buffer.String(), "connection refused")
	}
}

func TestFleetStatusTextAndHTMLPackagesAndFailedTask(t *testing.T) {
	client := testStatusClient{response: localapi.StatusResponse{RemoteConfigState: []*pbgo.PackageState{
		{
			Package:                 "datadog-agent",
			StableVersion:           "7.99.0",
			ExperimentVersion:       "7.100.0",
			RunningVersion:          "7.100.0",
			StableConfigVersion:     "config-stable",
			ExperimentConfigVersion: "config-experiment",
			RunningConfigVersion:    "config-experiment",
			Completion:              0.75,
			Task: &pbgo.PackageStateTask{
				Id:    "task-123",
				State: pbgo.TaskState_ERROR,
				Error: &pbgo.TaskError{Code: 6, Message: "upgrade failed"},
			},
		},
	}}}
	provider := newStatusProvider(t, true, client)

	for name, render := range map[string]func(bool, *bytes.Buffer) error{
		"text": func(verbose bool, buffer *bytes.Buffer) error { return provider.Text(verbose, buffer) },
		"html": func(verbose bool, buffer *bytes.Buffer) error { return provider.HTML(verbose, buffer) },
	} {
		t.Run(name, func(t *testing.T) {
			buffer := new(bytes.Buffer)
			require.NoError(t, render(false, buffer))
			output := buffer.String()
			for _, expected := range []string{
				"datadog-agent", "7.99.0", "7.100.0", "config-stable", "config-experiment",
				"ERROR", "task-123", "0.75", "6", "upgrade failed",
			} {
				assert.Contains(t, output, expected)
			}
		})
	}
}
