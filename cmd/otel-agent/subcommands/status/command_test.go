// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package status

import (
	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"os"
	"path"
	"testing"
)

func newGlobalParamsTest(t *testing.T) *subcommands.GlobalParams {
	// Because getSharedFxOption uses fx.Invoke, demultiplexer component is built
	// which lead to build:
	//   - config.Component which requires a valid datadog.yaml
	//   - hostname.Component which requires a valid hostname
	config := path.Join(t.TempDir(), "datadog.yaml")
	err := os.WriteFile(config, []byte("hostname: test"), 0644)
	require.NoError(t, err)

	return &subcommands.GlobalParams{
		CoreConfPath: config,
		ConfPaths:    []string{"test_config.yaml"},
	}
}

func TestStatusCommand(t *testing.T) {
	globalConfGetter := func() *subcommands.GlobalParams {
		return newGlobalParamsTest(t)
	}
	fxutil.TestOneShotSubcommand(t,
		[]*cobra.Command{MakeCommand(globalConfGetter)},
		[]string{"status"},
		runStatus,
		func() {},
	)
}
