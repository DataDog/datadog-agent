// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddflareextensionfx provides fx access for the provider component
package ddflareextensionfx

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/confignet"
	"go.uber.org/fx"
	"go.uber.org/zap"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	extension "github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/def"
	extensionimpl "github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Params holds non-component parameters for the ddflareextension FX module.
type Params struct {
	BYOC bool
}

// NewParams creates Params for the ddflareextension FX module.
func NewParams(byoc bool) Params { return Params{BYOC: byoc} }

// fxRequires holds the FX-injectable dependencies for the ddflareextension component.
// Both fields are optional: Params defaults to {BYOC: false} when not supplied, and
// IpcComp resolves to None when ipc.Component is absent from the graph.
type fxRequires struct {
	fx.In

	Params  Params        `optional:"true"`
	IpcComp ipc.Component `optional:"true"`
}

type fxProvides struct {
	fx.Out

	Comp extension.Component
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newComponent),
		fxutil.ProvideOptional[extension.Component](),
	)
}

// newComponent wraps NewComponent for FX injection.
// It uses a default HTTP config and providedConfigSupported=false because
// factories and configProviderSettings are not available at FX startup —
// they are supplied by the OTel collector at runtime via the factory.
func newComponent(reqs fxRequires) (fxProvides, error) {
	var ipcOpt option.Option[ipc.Component]
	if reqs.IpcComp != nil {
		ipcOpt = option.New(reqs.IpcComp)
	}
	comp, err := extensionimpl.NewComponent(
		context.Background(),
		&extensionimpl.Config{
			HTTPConfig: &confighttp.ServerConfig{
				NetAddr: confignet.AddrConfig{
					Endpoint:  fmt.Sprintf("localhost:%d", 7777),
					Transport: confignet.TransportTypeTCP,
				},
			},
		},
		component.TelemetrySettings{Logger: zap.NewNop()},
		component.BuildInfo{},
		ipcOpt,
		false,
		reqs.Params.BYOC,
	)
	if err != nil {
		return fxProvides{}, err
	}
	return fxProvides{Comp: comp}, nil
}
