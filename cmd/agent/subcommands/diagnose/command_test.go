// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diagnose

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestDiagnoseCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose"},
		runAll,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, false, coreParams.ConfigLoadSecrets())
		})
}

func TestMetadataAvailabilityCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "metadata-availability"},
		runAll,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, false, coreParams.ConfigLoadSecrets())
		})
}

func TestShowMetadataV5Command(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "show-metadata", "v5"},
		printPayload,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, false, coreParams.ConfigLoadSecrets())
			require.Equal(t, "v5", cliParams.payloadName)
		})
}

func TestShowMetadataInventoryCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "show-metadata", "inventory"},
		printPayload,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, false, coreParams.ConfigLoadSecrets())
			require.Equal(t, "inventory", cliParams.payloadName)
		})
}

func TestDatadogConnectivityCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "datadog-connectivity", "--no-trace"},
		runDatadogConnectivityDiagnose,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, false, coreParams.ConfigLoadSecrets())
			require.Equal(t, true, cliParams.noTrace)
		})
}
