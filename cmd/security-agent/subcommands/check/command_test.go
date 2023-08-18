// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

package check

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCommands(t *testing.T) {
	tests := []struct {
		name     string
		cliInput []string
		check    func(cliParams *CliParams, params core.BundleParams)
	}{
		{
			name:     "check",
			cliInput: []string{"check"},
			check: func(cliParams *CliParams, params core.BundleParams) {
				require.Equal(t, command.LoggerName, params.LoggerName(), "logger name not matching")
				require.Equal(t, "info", params.LogLevelFn(nil), "params.LogLevelFn not matching")
			},
		},
		{
			name:     "verbose",
			cliInput: []string{"check", "--verbose"},
			check: func(cliParams *CliParams, params core.BundleParams) {
				require.Equal(t, command.LoggerName, params.LoggerName(), "logger name not matching")
				require.Equal(t, "trace", params.LogLevelFn(nil), "params.LogLevelFn not matching")
			},
		},
	}

	for _, test := range tests {
		fxutil.TestOneShotSubcommand(t,
			SecurityAgentCommands(&command.GlobalParams{}),
			test.cliInput,
			RunCheck,
			test.check,
		)

		// TODO:
		//fxutil.TestOneShotSubcommand(t,
		//	ClusterAgentCommands(&core.BundleParams{}),
		//	test.cliInput,
		//	RunCheck,
		//	test.check,
		//)
	}
}
