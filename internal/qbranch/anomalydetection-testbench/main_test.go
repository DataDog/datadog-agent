// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"testing"

	"go.uber.org/fx"

	hfrunnernoop "github.com/DataDog/datadog-agent/comp/anomalydetection/hfrunner/fx-noop"
	observerfx "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/fx"
	recordernoop "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/fx-noop"
	reportertestbenchfx "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/fx-testbench"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	taggerdef "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilterdef "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmetadef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/stretchr/testify/require"
)

func TestFxApp(t *testing.T) {
	fxutil.TestOneShot(t, func() {
		err := fxutil.OneShot(run,
			recordernoop.Module(),
			hfrunnernoop.Module(),
			observerfx.Module(),
			reportertestbenchfx.Module(),
			fx.Supply(option.None[workloadmetadef.Component]()),
			fx.Supply(option.None[workloadfilterdef.Component]()),
			fx.Supply(option.None[taggerdef.Component]()),
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
