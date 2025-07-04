// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package defaultforwarder implements a component to send payloads to the backend
package defaultforwarder

import (
	"go.uber.org/fx"

	def "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	fxmod "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/fx"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-metric-pipelines

// Component is the component type.
type Component = def.Component

// Params is the params type for this component
type Params = def.Params

// Features are the component features
type Features = def.Features

// Mock implements mock-specific methods.
type Mock = def.Mock

// NewParams initializes a new Params struct
func NewParams(options ...def.OptionParams) def.Params {
	return def.NewParams(options...)
}

// WithResolvers enables the forwarder to use resolvers
func WithResolvers() def.OptionParams {
	return def.WithResolvers()
}

// WithDisableAPIKeyChecking disables the API key checking
func WithDisableAPIKeyChecking() def.OptionParams {
	return def.WithDisableAPIKeyChecking()
}

// WithFeatures sets a features to the forwarder
func WithFeatures(features ...def.Features) def.OptionParams {
	return def.WithFeatures(features...)
}

// WithNoopForwarder sets the forwarder to use the noop forwarder
func WithNoopForwarder() def.OptionParams {
	return def.WithNoopForwarder()
}

// Feature constants forwarding
const (
	CoreFeatures     = def.CoreFeatures
	TraceFeatures    = def.TraceFeatures
	ProcessFeatures  = def.ProcessFeatures
	SysProbeFeatures = def.SysProbeFeatures
)

// Module defines the fx options for this component.
func Module(params def.Params) fxutil.Module {
	return fxmod.Module(params)
}

// ModulWithOptionTMP defines the fx options for this component with an option.
// This is a temporary function to until configsync is cleanup.
func ModulWithOptionTMP(option fx.Option) fxutil.Module {
	return fxmod.ModulWithOptionTMP(option)
}

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxmod.MockModule()
}
