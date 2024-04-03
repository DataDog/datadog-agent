package aws

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	complog "github.com/DataDog/datadog-agent/comp/core/log"

	"github.com/DataDog/datadog-agent/pkg/agentless/types"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestStartAgentlessScannerAzure(t *testing.T) {
	grp := GroupCommand()

	fxutil.TestOneShotSubcommand(t,
		grp.Commands(),
		[]string{"scan", "plop"},
		awsScanCmd,
		func(params *awsScanParams, log complog.Component, sc *types.ScannerConfig) {
			require.NotNil(t, log)
			require.NotNil(t, sc)

			require.NotNil(t, params)
			require.Equal(t, "plop", params.resourceID)
			require.Equal(t, "unknown", params.targetName)
		})

	fxutil.TestOneShotSubcommand(t,
		grp.Commands(),
		[]string{"snapshot", "plop"},
		awsSnapshotCmd,
		func(params *awsSnapshotParams, log complog.Component, sc *types.ScannerConfig) {
			require.NotNil(t, log)
			require.NotNil(t, sc)

			require.NotNil(t, params)
			require.Equal(t, "plop", params.resourceID)
		})

	fxutil.TestOneShotSubcommand(t,
		grp.Commands(),
		[]string{"offline"},
		awsOfflineCmd,
		func(params *awsOfflineParams, log complog.Component, sc *types.ScannerConfig) {
			require.NotNil(t, log)
			require.NotNil(t, sc)

			require.NotNil(t, params)
			require.NotZero(t, params.workers)
			require.Equal(t, "", params.filters)
			require.Equal(t, "ebs-volume", params.taskType)
			require.Zero(t, params.maxScans)
			require.Equal(t, false, params.printResults)
		})

	fxutil.TestOneShotSubcommand(t,
		grp.Commands(),
		[]string{"attach", "plop"},
		awsAttachCmd,
		func(params *awsAttachParams, log complog.Component, sc *types.ScannerConfig) {
			require.NotNil(t, log)
			require.NotNil(t, sc)

			require.NotNil(t, params)
			require.Equal(t, "plop", params.resourceID)
			require.Equal(t, false, params.noMount)
		})

	fxutil.TestOneShotSubcommand(t,
		grp.Commands(),
		[]string{"cleanup"},
		awsCleanupCmd,
		func(params *awsCleanupParams, log complog.Component, sc *types.ScannerConfig) {
			require.NotNil(t, log)
			require.NotNil(t, sc)

			require.NotNil(t, params)
			require.Equal(t, false, params.dryRun)
			require.Equal(t, time.Duration(0), params.delay)
		})
}
