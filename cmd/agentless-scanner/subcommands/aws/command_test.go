package aws

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/agentless/types"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestStartAgentlessScannerAWS(t *testing.T) {
	grp := GroupCommand()

	fxutil.TestOneShotSubcommand(t,
		grp.Commands(),
		[]string{"scan", "plop"},
		awsScanCmd,
		func(params *awsScanParams, sc *types.ScannerConfig) {
			require.NotNil(t, sc)

			require.NotNil(t, params)
			require.Equal(t, "plop", params.resourceID)
			require.Equal(t, "unknown", params.targetName)
		})

	fxutil.TestOneShotSubcommand(t,
		grp.Commands(),
		[]string{"snapshot", "plop"},
		awsSnapshotCmd,
		func(params *awsSnapshotParams, sc *types.ScannerConfig) {
			require.NotNil(t, sc)

			require.NotNil(t, params)
			require.Equal(t, "plop", params.resourceID)
		})

	fxutil.TestOneShotSubcommand(t,
		grp.Commands(),
		[]string{"offline"},
		awsOfflineCmd,
		func(params *awsOfflineParams, sc *types.ScannerConfig) {
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
		func(params *awsAttachParams, sc *types.ScannerConfig) {
			require.NotNil(t, sc)

			require.NotNil(t, params)
			require.Equal(t, "plop", params.resourceID)
			require.Equal(t, false, params.noMount)
		})

	fxutil.TestOneShotSubcommand(t,
		grp.Commands(),
		[]string{"cleanup"},
		awsCleanupCmd,
		func(params *awsCleanupParams, sc *types.ScannerConfig) {
			require.NotNil(t, sc)

			require.NotNil(t, params)
			require.Equal(t, false, params.dryRun)
			require.Equal(t, time.Duration(0), params.delay)
		})
}
