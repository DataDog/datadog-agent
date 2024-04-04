package command

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/common"
	complog "github.com/DataDog/datadog-agent/comp/core/log"

	"github.com/DataDog/datadog-agent/pkg/agentless/types"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestStartAgentlessScannerAzure(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(newGlobalParamsTest(t)),
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
		Commands(newGlobalParamsTest(t)),
		[]string{"run-scanner", "--sock", "plop"},
		runScannerCmd,
		func(params *runScannerParams, log complog.Component, sc *types.ScannerConfig) {
			require.NotNil(t, log)
			require.NotNil(t, sc)

			require.NotNil(t, params)
			require.Equal(t, "plop", params.sock)
		})
}

func newGlobalParamsTest(t *testing.T) *common.GlobalParams {
	// config.Component which requires a valid datadog.yaml
	config := path.Join(t.TempDir(), "datadog.yaml")
	err := os.WriteFile(config, []byte("hostname: test"), 0644)
	require.NoError(t, err)

	return &common.GlobalParams{
		ConfigFilePath: config,
	}
}
