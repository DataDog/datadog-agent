// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package mock

import (
	"testing"

	autodiscoveryimpl "github.com/DataDog/datadog-agent/comp/core/autodiscovery/impl"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/scheduler"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	mockTagger "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	hpnoopimpl "github.com/DataDog/datadog-agent/comp/healthplatform/store/noop-impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// MockParams defines the parameters for the mock component.
type MockParams struct {
	Scheduler *scheduler.Controller
}

// MockRequires defines the dependencies of the mock autodiscovery component.
type MockRequires struct {
	compdef.In
	T          testing.TB
	WMeta      option.Option[workloadmeta.Component]
	Params     MockParams
	TaggerComp mockTagger.Mock
	LogsComp   log.Component
	FilterComp workloadfilter.Component
	Telemetry  telemetry.Component
	Secrets    secrets.Component
}

// MockProvides defines the outputs of the mock autodiscovery component.
type MockProvides struct {
	compdef.Out

	Comp Mock
}

// MockModule defines the fx options for the mock autodiscovery component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(NewMockComponent),
	)
}

// NewMockComponent creates a mock AutoConfig for use in tests.
func NewMockComponent(deps MockRequires) MockProvides {
	ac := autodiscoveryimpl.NewAutoConfigFromDeps(
		deps.Params.Scheduler, deps.Secrets, deps.WMeta, deps.TaggerComp,
		deps.LogsComp, deps.Telemetry, deps.FilterComp,
		hpnoopimpl.NewNoopComponent(),
	)
	return MockProvides{Comp: ac}
}
