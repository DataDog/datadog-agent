// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

package compliance

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/pkg/compliance/cli"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// TestCheckSubcommand ultimately uses the check package, so its dependencies are different from the event subcommand
func TestCheckSubcommand(t *testing.T) {
	tests := []struct {
		name     string
		cliInput []string
		check    func(cliParams *cli.CheckParams, params core.BundleParams)
	}{
		{
			name:     "compliance check",
			cliInput: []string{"compliance", "check"},
			check: func(_ *cli.CheckParams, params core.BundleParams) {
				require.Equal(t, command.LoggerName, params.LoggerName(), "logger name not matching")
				require.Equal(t, "info", params.LogLevelFn(nil), "params.LogLevelFn not matching")
			},
		},
		{
			name:     "compliance check verbose",
			cliInput: []string{"compliance", "check", "--verbose"},
			check: func(_ *cli.CheckParams, params core.BundleParams) {
				require.Equal(t, command.LoggerName, params.LoggerName(), "logger name not matching")
				require.Equal(t, "trace", params.LogLevelFn(nil), "params.LogLevelFn not matching")
			},
		},
	}

	for _, test := range tests {
		fxutil.TestOneShotSubcommand(t,
			Commands(&command.GlobalParams{}),
			test.cliInput,
			cli.RunCheck,
			test.check,
		)
	}
}

func TestLoadSubcommand(t *testing.T) {
	tests := []struct {
		name     string
		cliInput []string
		check    func(cliParams *cli.LoadParams, params core.BundleParams)
	}{
		{
			name:     "compliance load",
			cliInput: []string{"compliance", "load", "k8s"},
			check: func(cliParams *cli.LoadParams, params core.BundleParams) {
				require.Equal(t, command.LoggerName, params.LoggerName(), "logger name not matching")
				require.Equal(t, "info", params.LogLevelFn(nil), "params.LogLevelFn not matching")
				require.Equal(t, "k8s", cliParams.ConfType)
			},
		},
	}

	for _, test := range tests {
		fxutil.TestOneShotSubcommand(t,
			Commands(&command.GlobalParams{}),
			test.cliInput,
			cli.RunLoad,
			test.check,
		)
	}
}
