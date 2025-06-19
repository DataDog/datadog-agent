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
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
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
	// we need to ensure we have a valid auth token
	testDir := t.TempDir()

	configPath := path.Join(testDir, "datadog.yaml")

	// creating in-memory auth artifacts
	ipcmock.New(t)

	err := os.WriteFile(configPath, []byte("hostname: test"), 0644)
	require.NoError(t, err)

	return &command.GlobalParams{
		ConfFilePath: configPath,
	}
}
