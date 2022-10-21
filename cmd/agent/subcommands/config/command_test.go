// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestConfigCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"config"},
		showRuntimeConfiguration,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, []string{}, cliParams.args)
			require.Equal(t, false, coreParams.ConfigLoadSecrets)
		})
}

func TestConfigListRuntimeCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"config", "list-runtime"},
		listRuntimeConfigurableValue,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, []string{}, cliParams.args)
			require.Equal(t, false, coreParams.ConfigLoadSecrets)
		})
}

func TestConfigSetCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"config", "set", "foo", "bar"},
		setConfigValue,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, []string{"foo", "bar"}, cliParams.args)
			require.Equal(t, false, coreParams.ConfigLoadSecrets)
		})
}

func TestConfigGetCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"config", "get", "foo"},
		getConfigValue,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, []string{"foo"}, cliParams.args)
			require.Equal(t, false, coreParams.ConfigLoadSecrets)
		})
}
