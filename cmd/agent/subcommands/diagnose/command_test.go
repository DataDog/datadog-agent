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
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestDiagnoseCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose"},
		cmdDiagnose,
		func(cliParams *cliParams, coreParams core.BundleParams, secretParams secrets.Params) {
			require.Equal(t, false, secretParams.Enabled)
		})
}

func TestShowMetadataV5Command(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "show-metadata", "v5"},
		printPayload,
		func(cliParams *cliParams, coreParams core.BundleParams, secretParams secrets.Params) {
			require.Equal(t, false, secretParams.Enabled)
			require.Equal(t, "v5", cliParams.payloadName)
		})
}

func TestShowMetadataGohaiCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "show-metadata", "gohai"},
		printPayload,
		func(cliParams *cliParams, coreParams core.BundleParams, secretParams secrets.Params) {
			require.Equal(t, false, secretParams.Enabled)
			require.Equal(t, "gohai", cliParams.payloadName)
		})
}

func TestShowMetadataInventoryCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "show-metadata", "inventory"},
		printPayload,
		func(cliParams *cliParams, coreParams core.BundleParams, secretParams secrets.Params) {
			require.Equal(t, false, secretParams.Enabled)
			require.Equal(t, "inventory", cliParams.payloadName)
		})
}

func TestShowMetadataInventoryAgentCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "show-metadata", "inventory-agent"},
		printPayload,
		func(cliParams *cliParams, coreParams core.BundleParams, secretParams secrets.Params) {
			require.Equal(t, false, secretParams.Enabled)
			require.Equal(t, "inventory-agent", cliParams.payloadName)
		})
}

func TestShowMetadataInventoryHostCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "show-metadata", "inventory-host"},
		printPayload,
		func(cliParams *cliParams, coreParams core.BundleParams, secretParams secrets.Params) {
			require.Equal(t, false, secretParams.Enabled)
			require.Equal(t, "inventory-host", cliParams.payloadName)
		})
}
