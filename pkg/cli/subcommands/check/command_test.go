// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"os"
	"path"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCommand(t *testing.T) {
	commands := []*cobra.Command{
		MakeCommand(func() GlobalParams {
			// the config needs an existing config file when initializing
			config := path.Join(t.TempDir(), "datadog.yaml")
			err := os.WriteFile(config, []byte("hostname: test"), 0644)
			require.NoError(t, err)

			return GlobalParams{
				ConfFilePath: config,
			}
		}),
	}

	fxutil.TestOneShotSubcommand(t,
		commands,
		// this command has a lot of options, so just test a few
		[]string{"check", "cleopatra", "--delay", "1", "--flare"},
		run,
		func(cliParams *cliParams, _ core.BundleParams, secretParams secrets.Params) {
			require.Equal(t, []string{"cleopatra"}, cliParams.args)
			require.Equal(t, 1, cliParams.checkDelay)
			require.True(t, cliParams.saveFlare)
			require.Equal(t, true, secretParams.Enabled)
		})
}
