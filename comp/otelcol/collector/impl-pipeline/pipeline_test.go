// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && otlp
// +build test,otlp

// Package collector implements the collector component
package collector

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"go.uber.org/fx"
)

func TestFlareBuilder(t *testing.T) {
	cfg := fxutil.Test[config.Component](t,
		fx.Options(
			config.MockModule(),
		),
	)
	cfg.Set("otel-agent.enabled", true, pkgconfigmodel.SourceAgentRuntime)
	cfg.Set("otel-agent.flare_port", 7777, pkgconfigmodel.SourceAgentRuntime)

	reqs := Requires{
		Lc:     compdef.NewTestLifecycle(),
		Config: cfg,
	}
	provs, _ := NewComponent(reqs)
	col := provs.Comp.(*collectorImpl)

	f := helpers.NewFlareBuilderMock(t, false)
	col.fillFlare(f.Fb)

	overrideConfigResponseTemplate = `{
	"startup_configuration": {
		"key1": "value1",
		"key2": "value2",
		"key3": "value3"
	},
	"runtime_configuration": {
		"key4": "value4",
		"key5": "value5",
		"key6": "value6"
	},
	"cmdline": "./otel-agent a b c",
	"sources": [
		{
			"name": "prometheus",
			"url": "%s/one",
			"crawl": "true"
		},
		{
			"name": "zpages",
			"url": "%s/two",
			"crawl": "false"
		},
		{
			"name": "healthcheck",
			"url": "%s/three",
			"crawl": "true"
		},
		{
			"name": "pprof",
			"url": "%s/four",
			"crawl": "true"
		}
	],
	"environment": {
		"DD_KEY7": "value7",
		"DD_KEY8": "value8",
		"DD_KEY9": "value9"
	}
}
`
	defer func() { overrideConfigResponse = "" }()

	// TODO: make a server, which servers the paths

	f.AssertFileExists("otel", "otel-response.json")
	f.AssertFileContent("<body>Another source is <a href=\"http://localhost:5788/secret\">here</a></body>", "otel/otel-flare/healthcheck.dat")
	f.AssertFileContent("data-source-4", "otel/otel-flare/pprof.dat")
	f.AssertFileContent("data-source-1", "otel/otel-flare/prometheus.dat")
	f.AssertFileContent("data-source-2", "otel/otel-flare/zpages.dat")
}
