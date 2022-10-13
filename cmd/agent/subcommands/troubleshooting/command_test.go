// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package troubleshooting

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestPayloadV5Command(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"troubleshooting", "metadata_v5"},
		printPayload,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, "v5", cliParams.payloadName)
			require.Equal(t, false, coreParams.ConfigLoadSecrets)
		})
}

func TestPayloadInventoryCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"troubleshooting", "metadata_inventory"},
		printPayload,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, "inventory", cliParams.payloadName)
			require.Equal(t, false, coreParams.ConfigLoadSecrets)
		})
}
