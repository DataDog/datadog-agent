package azure

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/common"
	"github.com/DataDog/datadog-agent/pkg/agentless/types"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestStartAgentlessScannerAzure(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(newGlobalParamsTest(t)),
		[]string{"azure", "attach", "plop"},
		azureAttachCmd,
		func(params *azureAttachParams, sc *types.ScannerConfig) {
			require.NotNil(t, sc)

			require.NotNil(t, params)
			require.Equal(t, "plop", params.resourceID)
			require.Equal(t, false, params.noMount)
		})

	fxutil.TestOneShotSubcommand(t,
		Commands(newGlobalParamsTest(t)),
		[]string{"azure", "scan", "/"},
		azureScanCmd,
		func(params *azureScanParams, sc *types.ScannerConfig) {
			require.NotNil(t, sc)

			require.NotNil(t, params)
			require.Equal(t, "/", params.resourceID)
			require.Equal(t, "unknown", params.targetName)
		})

	fxutil.TestOneShotSubcommand(t,
		Commands(newGlobalParamsTest(t)),
		[]string{"azure", "offline", "--resource-group", "plop"},
		azureOfflineCmd,
		func(params *azureOfflineParams, sc *types.ScannerConfig) {
			require.NotNil(t, sc)

			require.NotNil(t, params)
			require.NotZero(t, params.workers)
			require.Equal(t, "plop", params.resourceGroup)
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
