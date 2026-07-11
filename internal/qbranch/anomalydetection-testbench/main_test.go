// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"testing"

	"go.uber.org/fx"

	observerfx "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/fx"
	recordernoop "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/fx-noop"
	reportertestbenchfx "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/fx-testbench"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	taggerdef "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilterdef "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmetadef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/stretchr/testify/require"
)

func TestFxApp(t *testing.T) {
	fxutil.TestOneShot(t, func() {
		err := fxutil.OneShot(run,
			recordernoop.Module(),
			observerfx.Module(),
			reportertestbenchfx.Module(),
			fx.Supply(option.None[workloadmetadef.Component]()),
			fx.Supply(option.None[workloadfilterdef.Component]()),
			fx.Supply(option.None[taggerdef.Component]()),
			core.Bundle(),
			// Mirror main(): force scorer dry-run on so NewComponent yields the full
			// observerImpl (with DebugView) instead of the disabled stub, and
			// keep the agent-internal log tap off so it never ingests scenario data.
			fx.Decorate(func(c config.Component) config.Component {
				c.Set("anomaly_detection.anomaly_scorer.dry_run.enabled", true, pkgconfigmodel.SourceAgentRuntime)
				c.Set("anomaly_detection.logs.internal.enabled", false, pkgconfigmodel.SourceAgentRuntime)
				return c
			}),
			fx.Supply(core.BundleParams{
				ConfigParams: config.NewAgentParams(""),
				LogParams:    log.ForOneShot("", "off", true),
			}),
			fx.Supply(CLIParams{}),
		)
		require.NoError(t, err)
	})
}
