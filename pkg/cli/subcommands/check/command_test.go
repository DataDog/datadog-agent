// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCommand(t *testing.T) {
	commands := []*cobra.Command{
		MakeCommand(func() GlobalParams {
			return GlobalParams{}
		}),
	}

	fxutil.TestOneShotSubcommand(t,
		commands,
		// this command has a lot of options, so just test a few
		[]string{"check", "cleopatra", "--delay", "1", "--flare"},
		run,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, []string{"cleopatra"}, cliParams.args)
			require.Equal(t, 1, cliParams.checkDelay)
			require.True(t, cliParams.saveFlare)
			require.Equal(t, true, coreParams.ConfigLoadSecrets())
		})
}

func TestYAMLExistQuery(t *testing.T) {
	exist, err := YAMLExistQuery(integration.Data("{\"ip_address\": \"127.0.0.50\"}"), ".ip_address == \"127.0.0.50\"")
	assert.NoError(t, err)
	assert.True(t, exist)

	exist, err = YAMLExistQuery(integration.Data("{\"ip_address\": \"127.0.0.50\"}"), ".ip_address == \"127.0.0.99\"")
	assert.NoError(t, err)
	assert.False(t, exist)

	exist, err = YAMLExistQuery(integration.Data("{\"ip_address\": \"127.0.0.50\"}"), ".ip_address")
	assert.EqualError(t, err, "filter query must return a boolean: yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `127.0.0.50` into bool")
	assert.False(t, exist)

	exist, err = YAMLExistQuery(integration.Data("{}"), ".ip_address == \"127.0.0.99\"")
	assert.NoError(t, err)
	assert.False(t, exist)
}
