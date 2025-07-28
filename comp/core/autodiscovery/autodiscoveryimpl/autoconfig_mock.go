// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package autodiscoveryimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/scheduler"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	mockTagger "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// MockParams defines the parameters for the mock component.
type MockParams struct {
	Scheduler *scheduler.Controller
}

type mockdependencies struct {
	fx.In
	WMeta      option.Option[workloadmeta.Component]
	Params     MockParams
	TaggerComp mockTagger.Mock
	LogsComp   log.Component
	FilterComp workloadfilter.Component
	Telemetry  telemetry.Component
	Secrets    secrets.Component
}

type mockprovides struct {
	fx.Out

	Comp autodiscovery.Mock
}

func newMockAutoConfig(deps mockdependencies) mockprovides {
	ac := createNewAutoConfig(deps.Params.Scheduler, deps.Secrets, deps.WMeta, deps.TaggerComp, deps.LogsComp, deps.Telemetry, deps.FilterComp)
	return mockprovides{
		Comp: ac,
	}
}

// MockModule provides the default autoconfig without other components configured, and not started
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMockAutoConfig),
	)
}
