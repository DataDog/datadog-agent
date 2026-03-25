// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

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
