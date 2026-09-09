// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && clusterchecks

package run

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent-cloudfoundry/command"
	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func TestCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(newGlobalParamsTest(t)),
		[]string{"run"},
		run,
		func() {})
}

// TestAgentTelemetryWired confirms the fx graph provides a real
// agenttelemetry.Component (via agenttelemetryfx.Module()) instead of the
// option.None it used to be hardcoded to, so the errortracking submitter
// installed by that module (see comp/core/agenttelemetry/fx.Module) actually
// runs for this binary.
func TestAgentTelemetryWired(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(newGlobalParamsTest(t)),
		[]string{"run"},
		run,
		func(at option.Option[agenttelemetry.Component]) {
			_, isSet := at.Get()
			require.True(t, isSet, "agenttelemetry.Component must be provided, not option.None")
		})
}

func newGlobalParamsTest(t *testing.T) *command.GlobalParams {
	// Because run uses fx.Invoke, demultiplexer, and workloadmeta component are built
	// which lead to build:
	//   - config.Component which requires a valid datadog.yaml
	config := path.Join(t.TempDir(), "datadog.yaml")
	err := os.WriteFile(config, []byte("hostname: test"), 0644)
	require.NoError(t, err)

	return &command.GlobalParams{
		ConfFilePath: config,
	}
}
