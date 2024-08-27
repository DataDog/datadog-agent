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
		func(_ *cliParams, _ core.BundleParams, secretParams secrets.Params) {
			require.Equal(t, false, secretParams.Enabled)
		})
}

func TestShowMetadataV5Command(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "show-metadata", "v5"},
		printPayload,
		func(_ core.BundleParams, secretParams secrets.Params) {
			require.Equal(t, false, secretParams.Enabled)
		})
}

func TestShowMetadataGohaiCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "show-metadata", "gohai"},
		printPayload,
		func(_ core.BundleParams, secretParams secrets.Params) {
			require.Equal(t, false, secretParams.Enabled)
		})
}

func TestShowMetadataInventoryAgentCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "show-metadata", "inventory-agent"},
		printPayload,
		func(_ core.BundleParams, secretParams secrets.Params) {
			require.Equal(t, false, secretParams.Enabled)
		})
}

func TestShowMetadataInventoryHostCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "show-metadata", "inventory-host"},
		printPayload,
		func(_ core.BundleParams, secretParams secrets.Params) {
			require.Equal(t, false, secretParams.Enabled)
		})
}

func TestShowMetadataInventoryChecksCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "show-metadata", "inventory-checks"},
		printPayload,
		func(_ core.BundleParams, secretParams secrets.Params) {
			require.Equal(t, false, secretParams.Enabled)
		})
}

func TestShowMetadataInventoryOtelCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "show-metadata", "inventory-otel"},
		printPayload,
		func(_ core.BundleParams, secretParams secrets.Params) {
			require.Equal(t, false, secretParams.Enabled)
		})
}

func TestShowMetadataPkgSigningCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "show-metadata", "package-signing"},
		printPayload,
		func(_ core.BundleParams, secretParams secrets.Params) {
			require.Equal(t, false, secretParams.Enabled)
		})
}

func TestShowMetadataSystemProbeCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "show-metadata", "system-probe"},
		printPayload,
		func(_ core.BundleParams, secretParams secrets.Params) {
			require.Equal(t, false, secretParams.Enabled)
		})
}

func TestShowMetadataSecurityAgentCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"diagnose", "show-metadata", "security-agent"},
		printPayload,
		func(_ core.BundleParams, secretParams secrets.Params) {
			require.Equal(t, false, secretParams.Enabled)
		})
}
