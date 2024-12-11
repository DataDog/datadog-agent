// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build jmx

package jmx

import (
	"os"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/secrets"

	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCollectCommand(t *testing.T) {
	// this tests several permutations of options that have a complex
	// relationship with the resulting params

	globalParams := newGlobalParamsTest(t)

	t.Run("with no args", func(t *testing.T) {
		fxutil.TestOneShotSubcommand(t,
			Commands(globalParams),
			[]string{"jmx", "collect"},
			runJmxCommandConsole,
			func(cliParams *cliParams, coreParams core.BundleParams, secretParams secrets.Params) {
				require.Equal(t, "collect", cliParams.command)
				require.Equal(t, "debug", cliParams.jmxLogLevel)
				require.Equal(t, "debug", coreParams.LogLevelFn(nil))
				require.Equal(t, "", cliParams.logFile)
				require.Equal(t, "", coreParams.LogFileFn(nil))
				require.Equal(t, "CORE", coreParams.LoggerName())
				require.Equal(t, true, secretParams.Enabled)
			})
	})

	t.Run("with --log-level", func(t *testing.T) {
		fxutil.TestOneShotSubcommand(t,
			Commands(globalParams),
			[]string{"jmx", "collect", "--log-level", "info"},
			runJmxCommandConsole,
			func(cliParams *cliParams, coreParams core.BundleParams, secretParams secrets.Params) {
				require.Equal(t, "collect", cliParams.command)
				require.Equal(t, "info", cliParams.jmxLogLevel)
				require.Equal(t, "info", coreParams.LogLevelFn(nil))
				require.Equal(t, "", cliParams.logFile)
				require.Equal(t, "", coreParams.LogFileFn(nil))
				require.Equal(t, "CORE", coreParams.LoggerName())
				require.Equal(t, true, secretParams.Enabled)
			})
	})

	t.Run("with --flare", func(t *testing.T) {
		fxutil.TestOneShotSubcommand(t,
			Commands(globalParams),
			[]string{"jmx", "collect", "--flare", "--log-level", "info"},
			runJmxCommandConsole,
			func(cliParams *cliParams, coreParams core.BundleParams, secretParams secrets.Params) {
				require.Equal(t, "collect", cliParams.command)
				require.Equal(t, "debug", cliParams.jmxLogLevel)      // overrides --log-level
				require.Equal(t, "debug", coreParams.LogLevelFn(nil)) // overrides --log-level
				require.True(t, strings.HasPrefix(cliParams.logFile, defaultpaths.JMXFlareDirectory))
				require.Equal(t, cliParams.logFile, coreParams.LogFileFn(nil))
				require.Equal(t, "CORE", coreParams.LoggerName())
				require.Equal(t, true, secretParams.Enabled)
			})
	})
}

func TestListEverythingCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(newGlobalParamsTest(t)),
		[]string{"jmx", "list", "everything"},
		runJmxCommandConsole,
		func(cliParams *cliParams, _ core.BundleParams) {
			require.Equal(t, "list_everything", cliParams.command)
		})
}

func TestListMatchingCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(newGlobalParamsTest(t)),
		[]string{"jmx", "list", "matching"},
		runJmxCommandConsole,
		func(cliParams *cliParams, _ core.BundleParams) {
			require.Equal(t, "list_matching_attributes", cliParams.command)
		})
}

func TestListWithRateMetricsCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(newGlobalParamsTest(t)),
		[]string{"jmx", "list", "with-rate-metrics"},
		runJmxCommandConsole,
		func(cliParams *cliParams, _ core.BundleParams) {
			require.Equal(t, "list_with_rate_metrics", cliParams.command)
		})
}

func TestListLimitedCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(newGlobalParamsTest(t)),
		[]string{"jmx", "list", "limited"},
		runJmxCommandConsole,
		func(cliParams *cliParams, _ core.BundleParams) {
			require.Equal(t, "list_limited_attributes", cliParams.command)
		})
}

func TestListCollectedCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(newGlobalParamsTest(t)),
		[]string{"jmx", "list", "collected"},
		runJmxCommandConsole,
		func(cliParams *cliParams, _ core.BundleParams) {
			require.Equal(t, "list_collected_attributes", cliParams.command)
		})
}

func TestListNotMatchingCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(newGlobalParamsTest(t)),
		[]string{"jmx", "list", "not-matching"},
		runJmxCommandConsole,
		func(cliParams *cliParams, _ core.BundleParams) {
			require.Equal(t, "list_not_matching_attributes", cliParams.command)
		})
}

func newGlobalParamsTest(t *testing.T) *command.GlobalParams {
	// Because run uses fx.Invoke, we need to provide a valid config file
	config := path.Join(t.TempDir(), "datadog.yaml")
	err := os.WriteFile(config, []byte("hostname: test"), 0644)
	require.NoError(t, err)

	return &command.GlobalParams{
		ConfFilePath: config,
	}
}
