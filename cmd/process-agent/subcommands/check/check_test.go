// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/process-agent/command"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestRunCheckCmdCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(newGlobalParamsTest(t)),
		[]string{"check", "process"},
		RunCheckCmd,
		func(_ *CliParams) {},
	)
}

func newGlobalParamsTest(t *testing.T) *command.GlobalParams {
	// Because we uses fx.Invoke some components are built
	// Since process agent could use the remote tagger we should disable here just in case
	config := path.Join(t.TempDir(), "datadog.yaml")
	configYaml := `hostname: tests
process_config:
  remote_tagger: false`

	err := os.WriteFile(config, []byte(configYaml), 0644)
	require.NoError(t, err)

	return &command.GlobalParams{
		ConfFilePath: config,
	}
}
