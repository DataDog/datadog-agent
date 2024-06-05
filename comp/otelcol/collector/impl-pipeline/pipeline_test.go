// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test
// +build test

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

	f.AssertFileExists("otel", "otel-response.json")
}
