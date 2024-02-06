// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secret

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"secret"},
		showSecretInfo,
		func(coreParams core.BundleParams, secretParams secrets.Params) {
			require.Equal(t, false, secretParams.Enabled)
		})
}

func TestRefreshCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"secret", "refresh"},
		secretRefresh,
		func(coreParams core.BundleParams, secretParams secrets.Params) {
			require.Equal(t, false, secretParams.Enabled)
		})
}
