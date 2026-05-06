// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"testing"

	"go.uber.org/fx"

	recorderfx "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/fx"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/require"
)

func TestFxApp(t *testing.T) {
	fxutil.TestOneShot(t, func() {
		err := fxutil.OneShot(run,
			recorderfx.Module(),
			core.Bundle(),
			fx.Supply(core.BundleParams{
				ConfigParams: config.NewAgentParams(""),
				LogParams:    log.ForOneShot("", "off", true),
			}),
			fx.Supply(CLIParams{}),
		)
		require.NoError(t, err)
	})
}
