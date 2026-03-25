// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	pkgsettings "github.com/DataDog/datadog-agent/pkg/config/settings"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type mockSettingsClient struct {
	listResult map[string]settings.RuntimeSettingResponse
}

func (m *mockSettingsClient) Get(_ string) (interface{}, error)                          { return nil, nil }
func (m *mockSettingsClient) GetWithSources(_ string) (map[string]interface{}, error)    { return nil, nil }
func (m *mockSettingsClient) Set(_ string, _ string) (bool, error)                       { return false, nil }
func (m *mockSettingsClient) FullConfig() (string, error)                                { return "", nil }
func (m *mockSettingsClient) FullConfigWithoutDefaults() (string, error)                 { return "", nil }
func (m *mockSettingsClient) FullConfigBySource() (string, error)                        { return "", nil }
func (m *mockSettingsClient) List() (map[string]settings.RuntimeSettingResponse, error) {
	return m.listResult, nil
}

func TestConfigCommand(t *testing.T) {
	commands := []*cobra.Command{
		MakeCommand(func() GlobalParams {
			return GlobalParams{}
		}),
	}

	fxutil.TestOneShotSubcommand(t,
		commands,
		[]string{"config"},
		showRuntimeConfiguration,
		func(cliParams *cliParams, _ core.BundleParams) {
			require.Empty(t, cliParams.args)
		})
}

func TestConfigListRuntimeCommand(t *testing.T) {
	commands := []*cobra.Command{
		MakeCommand(func() GlobalParams {
			return GlobalParams{}
		}),
	}

	fxutil.TestOneShotSubcommand(t,
		commands,
		[]string{"config", "list-runtime"},
		listRuntimeConfigurableValue,
		func(cliParams *cliParams, _ core.BundleParams) {
			require.Empty(t, cliParams.args)
		})
}

func TestConfigSetCommand(t *testing.T) {
	commands := []*cobra.Command{
		MakeCommand(func() GlobalParams {
			return GlobalParams{}
		}),
	}

	fxutil.TestOneShotSubcommand(t,
		commands,
		[]string{"config", "set", "foo", "bar"},
		setConfigValue,
		func(cliParams *cliParams, _ core.BundleParams) {
			require.Equal(t, []string{"foo", "bar"}, cliParams.args)
		})
}

func TestConfigSetCommandInvalidArgCount(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"missing value", []string{"log_level"}},
		{"too many args", []string{"dd_url", "too", "many", "args"}},
		{"no args", nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := setConfigValue(nil, nil, &cliParams{args: tc.args})
			assert.ErrorContains(t, err, "exactly two parameters are required: the setting name and its value")
		})
	}
}

func TestListRuntimeConfigurableValueHidesHiddenSettings(t *testing.T) {
	mock := &mockSettingsClient{
		listResult: map[string]settings.RuntimeSettingResponse{
			"log_level":                      {Description: "log level", Hidden: false},
			"dogstatsd_stats":                {Description: "dogstatsd stats", Hidden: false},
			"runtime_mutex_profile_fraction": {Description: "mutex profile fraction", Hidden: true},
			"internal_profiling":             {Description: "internal profiling", Hidden: true},
		},
	}

	params := &cliParams{
		GlobalParams: GlobalParams{
			SettingsBuilder: func(_ ipc.HTTPClient) (pkgsettings.Client, error) {
				return mock, nil
			},
		},
	}

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := listRuntimeConfigurableValue(nil, nil, params)
	require.NoError(t, err)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Visible settings should appear
	assert.Contains(t, output, "log_level")
	assert.Contains(t, output, "dogstatsd_stats")

	// Hidden settings should NOT appear
	assert.NotContains(t, output, "runtime_mutex_profile_fraction")
	assert.NotContains(t, output, "internal_profiling")
}

func TestConfigGetCommandInvalidArgCount(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"no args", nil},
		{"too many args", []string{"too", "many", "args"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := getConfigValue(nil, nil, &cliParams{args: tc.args})
			assert.ErrorContains(t, err, "a single setting name must be specified")
		})
	}
}

func TestConfigGetCommand(t *testing.T) {
	commands := []*cobra.Command{
		MakeCommand(func() GlobalParams {
			return GlobalParams{}
		}),
	}

	fxutil.TestOneShotSubcommand(t,
		commands,
		[]string{"config", "get", "foo"},
		getConfigValue,
		func(cliParams *cliParams, _ core.BundleParams) {
			require.Equal(t, []string{"foo"}, cliParams.args)
		})
}
