// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package run

import (
	"testing"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestStartCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		[]*cobra.Command{MakeCommand(func() *subcommands.GlobalParams {
			return &subcommands.GlobalParams{
				ConfPath: "PATH",
			}

		})},
		[]string{"run", "--cpu-profile", "/foo", "--mem-profile", "/bar", "--pidfile", "/var/run/quz.pid"},
		Start,
		func(cliParams *RunParams) {
			require.Equal(t, "PATH", cliParams.ConfPath)
			require.Equal(t, "/foo", cliParams.CPUProfile)
			require.Equal(t, "/bar", cliParams.MemProfile)
			require.Equal(t, "/var/run/quz.pid", cliParams.PIDFilePath)
		})
}
