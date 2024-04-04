package command

import (
	"testing"

	"github.com/stretchr/testify/require"

	complog "github.com/DataDog/datadog-agent/comp/core/log"

	"github.com/DataDog/datadog-agent/pkg/agentless/types"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestStartAgentlessScannerAzure(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		RootCommand().Commands(),
		[]string{"run"},
		runCmd,
		func(params *runParams, log complog.Component, sc *types.ScannerConfig) {
			require.NotNil(t, log)
			require.NotNil(t, sc)

			require.NotNil(t, params)
			require.Equal(t, "auto", params.cloudProvider)
			require.Equal(t, "", params.pidfilePath)
			require.NotZero(t, params.workers)
			require.NotZero(t, params.scannersMax)
		})

	fxutil.TestOneShotSubcommand(t,
		RootCommand().Commands(),
		[]string{"run-scanner", "--sock", "plop"},
		runScannerCmd,
		func(params *runScannerParams, log complog.Component, sc *types.ScannerConfig) {
			require.NotNil(t, log)
			require.NotNil(t, sc)

			require.NotNil(t, params)
			require.Equal(t, "plop", params.sock)
		})
}
