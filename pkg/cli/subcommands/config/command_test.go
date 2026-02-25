// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func getCmdByUse(cmd *cobra.Command, use string) *cobra.Command {
	for _, c := range cmd.Commands() {
		if c.Use == use {
			return c
		}
	}
	return nil
}

func TestConfigCommandPresent(t *testing.T) {
	root := MakeCommand(func() GlobalParams { return GlobalParams{} })
	require.Equal(t, "config", root.Use)
}

func TestConfigListRuntimeSubcommandPresent(t *testing.T) {
	root := MakeCommand(func() GlobalParams { return GlobalParams{} })
	require.NotNil(t, getCmdByUse(root, "list-runtime"))
}

func TestConfigSetSubcommandPresent(t *testing.T) {
	root := MakeCommand(func() GlobalParams { return GlobalParams{} })
	require.NotNil(t, getCmdByUse(root, "set [setting] [value]"))
}

func TestConfigSystemProbeCommand(t *testing.T) {
	root := MakeCommand(func() GlobalParams { return GlobalParams{} })
	require.NotNil(t, getCmdByUse(root, "system-probe"))
}

func TestConfigOneShotInvoked(t *testing.T) {
	root := MakeCommand(func() GlobalParams { return GlobalParams{} })
	fxutil.TestOneShotSubcommand(
		t,
		[]*cobra.Command{root},
		[]string{"config"},
		showRuntimeConfiguration,
		func(p *cliParams) {
			// Should be invoked with a cliParams instance; no args for top-level command
			require.Empty(t, p.args)
		},
	)
}
