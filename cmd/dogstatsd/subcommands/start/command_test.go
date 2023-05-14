// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package start

// TODO: (components) This test is does not work with the current error handling
// structure of the config component. Fix this test when config can be constructed
// without a failure due to missing arguments.

// import (
// 	"testing"

// 	"github.com/DataDog/datadog-agent/comp/core/config"
// 	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
// 	"github.com/spf13/cobra"
// 	"github.com/stretchr/testify/require"
// )

// func TestStartCommand(t *testing.T) {
// 	fxutil.TestOneShotSubcommand(t,
// 		[]*cobra.Command{MakeCommand("defaultLogFile")},
// 		[]string{"start", "--cfgpath", "PATH"},
// 		start,
// 		func(cliParams *CLIParams, _ config.Params) {
// 			require.Equal(t, "PATH", cliParams.confPath)
// 		})
// }
