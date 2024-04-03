package azure

import (
	"github.com/stretchr/testify/require"
	"testing"

	complog "github.com/DataDog/datadog-agent/comp/core/log"

	"github.com/DataDog/datadog-agent/pkg/agentless/types"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestStartAgentlessScannerAzure(t *testing.T) {
	grp := GroupCommand()

	fxutil.TestOneShotSubcommand(t,
		grp.Commands(),
		[]string{"attach", "plop"},
		azureAttachCmd,
		func(params *azureAttachParams, log complog.Component, sc *types.ScannerConfig) {
			require.NotNil(t, log)
			require.NotNil(t, sc)

			require.NotNil(t, params)
			require.Equal(t, "plop", params.resourceID)
			require.Equal(t, false, params.noMount)
		})

	fxutil.TestOneShotSubcommand(t,
		grp.Commands(),
		[]string{"scan", "/"},
		azureScanCmd,
		func(params *azureScanParams, log complog.Component, sc *types.ScannerConfig) {
			require.NotNil(t, log)
			require.NotNil(t, sc)

			require.NotNil(t, params)
			require.Equal(t, "/", params.resourceID)
			require.Equal(t, "unknown", params.targetName)
		})

	fxutil.TestOneShotSubcommand(t,
		grp.Commands(),
		[]string{"offline", "--resource-group", "plop"},
		azureOfflineCmd,
		func(params *azureOfflineParams, log complog.Component, sc *types.ScannerConfig) {
			require.NotNil(t, log)
			require.NotNil(t, sc)

			require.NotNil(t, params)
			require.NotZero(t, params.workers)
			require.Equal(t, "plop", params.resourceGroup)
		})
}
