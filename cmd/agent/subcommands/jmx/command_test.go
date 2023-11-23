// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build jmx

package jmx

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/common/path"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCollectCommand(t *testing.T) {
	// this tests several permutations of options that have a complex
	// relationship with the resulting params
	t.Run("with no args", func(t *testing.T) {
		fxutil.TestOneShotSubcommand(t,
			Commands(&command.GlobalParams{}),
			[]string{"jmx", "collect"},
			runJmxCommandConsole,
			func(cliParams *cliParams, coreParams core.BundleParams) {
				require.Equal(t, "collect", cliParams.command)
				require.Equal(t, "debug", cliParams.jmxLogLevel)
				require.Equal(t, "debug", coreParams.LogLevelFn(nil))
				require.Equal(t, "", cliParams.logFile)
				require.Equal(t, "", coreParams.LogFileFn(nil))
				require.Equal(t, "CORE", coreParams.LoggerName())
				require.Equal(t, true, coreParams.ConfigLoadSecrets())
			})
	})

	t.Run("with --log-level", func(t *testing.T) {
		fxutil.TestOneShotSubcommand(t,
			Commands(&command.GlobalParams{}),
			[]string{"jmx", "collect", "--log-level", "info"},
			runJmxCommandConsole,
			func(cliParams *cliParams, coreParams core.BundleParams) {
				require.Equal(t, "collect", cliParams.command)
				require.Equal(t, "info", cliParams.jmxLogLevel)
				require.Equal(t, "info", coreParams.LogLevelFn(nil))
				require.Equal(t, "", cliParams.logFile)
				require.Equal(t, "", coreParams.LogFileFn(nil))
				require.Equal(t, "CORE", coreParams.LoggerName())
				require.Equal(t, true, coreParams.ConfigLoadSecrets())
			})
	})

	t.Run("with --flare", func(t *testing.T) {
		fxutil.TestOneShotSubcommand(t,
			Commands(&command.GlobalParams{}),
			[]string{"jmx", "collect", "--flare", "--log-level", "info"},
			runJmxCommandConsole,
			func(cliParams *cliParams, coreParams core.BundleParams) {
				require.Equal(t, "collect", cliParams.command)
				require.Equal(t, "debug", cliParams.jmxLogLevel)      // overrides --log-level
				require.Equal(t, "debug", coreParams.LogLevelFn(nil)) // overrides --log-level
				require.True(t, strings.HasPrefix(cliParams.logFile, path.DefaultJMXFlareDirectory))
				require.Equal(t, cliParams.logFile, coreParams.LogFileFn(nil))
				require.Equal(t, "CORE", coreParams.LoggerName())
				require.Equal(t, true, coreParams.ConfigLoadSecrets())
			})
	})
}

func TestListEverythingCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"jmx", "list", "everything"},
		runJmxCommandConsole,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, "list_everything", cliParams.command)
		})
}

func TestListMatchingCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"jmx", "list", "matching"},
		runJmxCommandConsole,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, "list_matching_attributes", cliParams.command)
		})
}

func TestListWithRateMetricsCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"jmx", "list", "with-rate-metrics"},
		runJmxCommandConsole,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, "list_with_rate_metrics", cliParams.command)
		})
}

func TestListLimitedCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"jmx", "list", "limited"},
		runJmxCommandConsole,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, "list_limited_attributes", cliParams.command)
		})
}

func TestListCollectedCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"jmx", "list", "collected"},
		runJmxCommandConsole,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, "list_collected_attributes", cliParams.command)
		})
}

func TestListNotMatchingCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"jmx", "list", "not-matching"},
		runJmxCommandConsole,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, "list_not_matching_attributes", cliParams.command)
		})
}
