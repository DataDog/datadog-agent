// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package run

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestPrivateActionRunnerRunCommand(t *testing.T) {
	fxutil.TestRun(t, func() error {
		commands := Commands(newGlobalParamsTest(t))
		return commands[0].RunE(nil, []string{"run"})
	})
}

func newGlobalParamsTest(t *testing.T) *command.GlobalParams {
	// Create minimal config for private action runner testing
	configPath := path.Join(t.TempDir(), "datadog.yaml")
	err := os.WriteFile(configPath, []byte(`
hostname: test
privateactionrunner:
  enabled: false
  private_key: test_private_key
  urn: test_urn
api_key: test_key
`), 0644)
	require.NoError(t, err)

	return &command.GlobalParams{
		ConfFilePath: configPath,
	}
}
