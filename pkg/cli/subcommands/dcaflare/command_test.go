// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

package dcaflare

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestResolveDCALogFile(t *testing.T) {
	t.Run("unconfigured log_file falls back to the cluster-agent's default log file", func(t *testing.T) {
		cfg := configmock.New(t)
		require.Equal(t, defaultpaths.GetDefaultDCALogFile(), resolveDCALogFile(cfg))
	})

	t.Run("explicitly configured log_file is preserved", func(t *testing.T) {
		cfg := configmock.New(t)
		cfg.Set("log_file", "/custom/cluster-agent.log", pkgconfigmodel.SourceAgentRuntime)
		assert.Equal(t, "/custom/cluster-agent.log", resolveDCALogFile(cfg))
	})
}

func TestCommand(t *testing.T) {
	commands := []*cobra.Command{
		MakeCommand(func() GlobalParams {
			return GlobalParams{}
		}),
	}

	fxutil.TestOneShotSubcommand(t,
		commands,
		[]string{"flare"},
		run,
		func() {})
}
