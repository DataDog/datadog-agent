// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package main

import (
	"flag"
	"os"
	"strings"
	"testing"

	"go.uber.org/fx"

	observerimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl"
	recorderfx "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/fx"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	onlyStr := flag.String("only", "", "Enable ONLY these components (plus extractors); disable everything else.")
	flag.Parse()

	if *onlyStr != "" {
		onlySet := make(map[string]bool)
		for _, name := range strings.Split(*onlyStr, ",") {
			name = strings.TrimSpace(name)
			if name != "" {
				onlySet[name] = true
			}
		}
		overrides := make(map[string]bool)
		for _, entry := range observerimpl.TestbenchCatalogEntries() {
			if entry.Kind == "extractor" {
				continue // extractors always enabled
			}
			overrides[entry.Name] = onlySet[entry.Name]
		}
		benchmarkSettings = observerimpl.ComponentSettings{Enabled: overrides}
	}

	os.Exit(m.Run())
}

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
