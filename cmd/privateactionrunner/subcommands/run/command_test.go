// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package run

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	privateactionrunner "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestPrivateActionRunnerRunCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(newGlobalParamsTest(t)),
		[]string{"run"},
		run,
		func(_ log.Component, _ config.Component, _ privateactionrunner.Component) {})
}

func newGlobalParamsTest(t *testing.T) *command.GlobalParams {
	// Create minimal config for private action runner testing
	config := path.Join(t.TempDir(), "datadog.yaml")
	err := os.WriteFile(config, []byte(`
hostname: test
privateactionrunner:
  enabled: false
  private_key: test_private_key
  urn: test_urn
api_key: test_key
`), 0644)
	require.NoError(t, err)

	return &command.GlobalParams{
		ConfFilePath: config,
	}
}
