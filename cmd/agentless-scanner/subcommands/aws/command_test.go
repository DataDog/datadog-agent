package aws

import (
	"os"
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/common"
	"github.com/DataDog/datadog-agent/pkg/agentless/types"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestStartAgentlessScannerAWS(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(newGlobalParamsTest(t)),
		[]string{"aws", "scan", "--target-id", "plop"},
		awsScanCmd,
		func(params *awsScanParams, sc *types.ScannerConfig) {
			require.NotNil(t, sc)

			require.NotNil(t, params)
			require.Equal(t, "plop", params.targetID)
			require.Equal(t, "unknown", params.targetName)
		})

	fxutil.TestOneShotSubcommand(t,
		Commands(newGlobalParamsTest(t)),
		[]string{"aws", "snapshot", "--target-id", "plop"},
		awsSnapshotCmd,
		func(params *awsSnapshotParams, sc *types.ScannerConfig) {
			require.NotNil(t, sc)

			require.NotNil(t, params)
			require.Equal(t, "plop", params.targetID)
		})

	fxutil.TestOneShotSubcommand(t,
		Commands(newGlobalParamsTest(t)),
		[]string{"aws", "offline"},
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
		Commands(newGlobalParamsTest(t)),
		[]string{"aws", "attach", "--target-id", "plop"},
		awsAttachCmd,
		func(params *awsAttachParams, sc *types.ScannerConfig) {
			require.NotNil(t, sc)

			require.NotNil(t, params)
			require.Equal(t, "plop", params.targetID)
			require.Equal(t, false, params.noMount)
		})

	fxutil.TestOneShotSubcommand(t,
		Commands(newGlobalParamsTest(t)),
		[]string{"aws", "cleanup"},
		awsCleanupCmd,
		func(params *awsCleanupParams, sc *types.ScannerConfig) {
			require.NotNil(t, sc)

			require.NotNil(t, params)
			require.Equal(t, false, params.dryRun)
			require.Equal(t, time.Duration(0), params.delay)
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
