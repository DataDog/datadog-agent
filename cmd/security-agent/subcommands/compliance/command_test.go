// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows || !kubeapiserver

package compliance

import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
)

// This test suite requires the opposite build flags of the check package
// in order to test the compliance command in environments that cannot have the check subcommand.

func TestEventCommands(t *testing.T) {
	tests := []struct {
		name     string
		cliInput []string
		check    func(cliParams *eventCliParams, params core.BundleParams)
	}{
		{
			name:     "compliance event tags",
			cliInput: []string{"compliance", "event", "--tags", "test:tag"},
			check: func(cliParams *eventCliParams, params core.BundleParams) {
				require.Equal(t, command.LoggerName, params.LoggerName(), "logger name not matching")
				require.Equal(t, "info", params.LogLevelFn(nil), "log level not matching")
				require.Equal(t, []string{"test:tag"}, cliParams.event.Tags, "tags arg input not matching")
			},
		},
	}

	for _, test := range tests {
		rootCommand := Commands(&command.GlobalParams{})[0]

		var subcommandNames []string
		for _, subcommand := range rootCommand.Commands() {
			subcommandNames = append(subcommandNames, subcommand.Use)
		}

		require.Equal(t, []string{"event", "load <conf-type>"}, subcommandNames, "subcommand missing")

		fxutil.TestOneShotSubcommand(t,
			Commands(&command.GlobalParams{}),
			test.cliInput,
			eventRun,
			test.check,
		)
	}
}

func TestLoadSubcommand(t *testing.T) {
	tests := []struct {
		name     string
		cliInput []string
		check    func(cliParams *loadCliParams, params core.BundleParams)
	}{
		{
			name:     "compliance load",
			cliInput: []string{"compliance", "load", "k8s"},
			check: func(cliParams *loadCliParams, params core.BundleParams) {
				require.Equal(t, command.LoggerName, params.LoggerName(), "logger name not matching")
				require.Equal(t, "info", params.LogLevelFn(nil), "params.LogLevelFn not matching")
				require.Equal(t, "k8s", cliParams.confType)
			},
		},
	}

	for _, test := range tests {
		fxutil.TestOneShotSubcommand(t,
			Commands(&command.GlobalParams{}),
			test.cliInput,
			loadRun,
			test.check,
		)
	}
}
