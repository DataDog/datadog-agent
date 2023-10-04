// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

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
		check    func(cliParams *cliParams, params core.BundleParams)
	}{
		{
			name:     "flare",
			cliInput: []string{"flare"},
			check: func(cliParams *cliParams, params core.BundleParams) {
				require.Equal(t, command.LoggerName, params.LoggerName(), "logger name not matching")
				require.Equal(t, "off", params.LogLevelFn(nil), "log level not matching")
			},
		},
		{
			name:     "flare 001",
			cliInput: []string{"flare", "001"},
			check: func(cliParams *cliParams, params core.BundleParams) {
				require.Equal(t, command.LoggerName, params.LoggerName(), "logger name not matching")
				require.Equal(t, "off", params.LogLevelFn(nil), "log level not matching")
				require.Equal(t, "001", cliParams.caseID, "case ID not matching")
			},
		},
	}

	for _, test := range tests {
		fxutil.TestOneShotSubcommand(t,
			Commands(&command.GlobalParams{}),
			test.cliInput,
			requestFlare,
			test.check,
		)
	}
}
