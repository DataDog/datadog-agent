// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddprofilingextensionfx provides fx access for the provider component
package ddprofilingextensionfx

import (
	"go.opentelemetry.io/collector/component"
	"go.uber.org/fx"

	corelog "github.com/DataDog/datadog-agent/comp/core/log/def"
	ddprofilingextension "github.com/DataDog/datadog-agent/comp/otelcol/ddprofilingextension/def"
	ddprofilingextensionimpl "github.com/DataDog/datadog-agent/comp/otelcol/ddprofilingextension/impl"
	traceagent "github.com/DataDog/datadog-agent/comp/trace/agent/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: opentelemetry-agent

// fxRequires holds the FX-injectable dependencies for the ddprofilingextension component.
// The OTel factory's Create method provides the real *Config and BuildInfo at collector startup.
type fxRequires struct {
	fx.In

	TraceAgent traceagent.Component
	Log        corelog.Component
}

type fxProvides struct {
	fx.Out

	Comp ddprofilingextension.Component
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newComponent),
		fxutil.ProvideOptional[ddprofilingextension.Component](),
	)
}

// newComponent wraps NewComponent for FX injection.
// It uses a default Config and zero BuildInfo; the OTel factory's Create method
// overrides these with the real values when the collector starts.
func newComponent(reqs fxRequires) (fxProvides, error) {
	comp, err := ddprofilingextensionimpl.NewComponent(
		&ddprofilingextensionimpl.Config{},
		component.BuildInfo{},
		reqs.TraceAgent,
		reqs.Log,
	)
	if err != nil {
		return fxProvides{}, err
	}
	return fxProvides{Comp: comp}, nil
}
