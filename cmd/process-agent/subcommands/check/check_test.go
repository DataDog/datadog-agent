// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"context"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/process-agent/command"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
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
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("auth_token_file_path", path.Join(testDir, "auth_token"))

	_, err := security.FetchOrCreateAuthToken(context.Background(), mockConfig)
	require.NoError(t, err)

	err = os.WriteFile(configPath, []byte("hostname: test"), 0644)
	require.NoError(t, err)

	return &command.GlobalParams{
		ConfFilePath: configPath,
	}
}
