// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package experimental

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core"
	configcmd "github.com/DataDog/datadog-agent/pkg/cli/subcommands/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestExperimentalCheckConfigCommand(t *testing.T) {
	commands := []*cobra.Command{
		MakeCommand(func() configcmd.GlobalParams {
			return configcmd.GlobalParams{}
		}),
	}

	fxutil.TestOneShotSubcommand(t,
		commands,
		[]string{"experimental", "check-config"},
		runExperimentalCheck,
		func(p *experimentalParams, _ core.BundleParams) {
			require.Empty(t, p.args)
		})
}
