// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package version

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCommand(t *testing.T) {
	tests := []struct {
		name     string
		cliInput []string
		check    func(params core.BundleParams)
	}{
		{
			name:     "version",
			cliInput: []string{"version"},
			check: func(params core.BundleParams) {
				// Verify logger defaults
				require.Equal(t, command.LoggerName, params.LoggerName(), "logger name not matching")
				require.Equal(t, "off", params.LogLevelFn(nil), "log level not matching")
			},
		},
	}

	for _, test := range tests {
		fxutil.TestOneShotSubcommand(t,
			Commands(&command.GlobalParams{}),
			test.cliInput,
			displayVersion,
			test.check,
		)
	}
}
