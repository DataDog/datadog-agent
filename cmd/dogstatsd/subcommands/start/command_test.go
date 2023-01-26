// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package start

import (
	"fmt"
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestStartCommand(t *testing.T) {

	testDir := t.TempDir()
	cfgPath := fmt.Sprintf("%s/dogstatsd.yaml", testDir)
	logpath := fmt.Sprintf("%s/default.log", testDir)
	os.Create(cfgPath)
	os.Create(logpath)

	fxutil.TestOneShotSubcommand(t,
		[]*cobra.Command{MakeCommand(logpath)},
		[]string{"start", "--cfgpath", cfgPath},
		start,
		func(cliParams *CLIParams, _ config.Params, _ dogstatsdServer.Component) {
			require.Equal(t, cfgPath, cliParams.confPath)
		})
}
