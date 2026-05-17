// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package defaultforwarder implements a component to send payloads to the backend.
// Deprecated: use comp/forwarder/defaultforwarder/def instead.
package defaultforwarder

import (
	"go.uber.org/fx"

	defaultforwarderdef "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	defaultforwarderfx "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/fx"
	defaultforwarderimpl "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/impl"
	defaultforwardermock "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-metric-pipelines

// Component is the component type.
// Deprecated: use comp/forwarder/defaultforwarder/def.Component instead.
type Component = defaultforwarderdef.Component

// Forwarder interface allows packages to send payload to the backend.
// Deprecated: use comp/forwarder/defaultforwarder/def.Forwarder instead.
type Forwarder = defaultforwarderdef.Forwarder

// ForwarderV2 is the V2 interface.
// Deprecated: use comp/forwarder/defaultforwarder/def.ForwarderV2 instead.
type ForwarderV2 = defaultforwarderdef.ForwarderV2

// Response contains the response details of a successfully posted transaction.
// Deprecated: use comp/forwarder/defaultforwarder/def.Response instead.
type Response = defaultforwarderdef.Response

// Features is a bitmask to enable specific forwarder features.
// Deprecated: use comp/forwarder/defaultforwarder/def.Features instead.
type Features = defaultforwarderdef.Features

// CoreFeatures bitmask.
// Deprecated: use comp/forwarder/defaultforwarder/def instead.
const (
	CoreFeatures     = defaultforwarderdef.CoreFeatures
	TraceFeatures    = defaultforwarderdef.TraceFeatures
	ProcessFeatures  = defaultforwarderdef.ProcessFeatures
	SysProbeFeatures = defaultforwarderdef.SysProbeFeatures
)

// SetFeature sets forwarder features. Deprecated: use def package.
func SetFeature(features, flag Features) Features {
	return defaultforwarderdef.SetFeature(features, flag)
}

// ClearFeature clears forwarder features. Deprecated: use def package.
func ClearFeature(features, flag Features) Features {
	return defaultforwarderdef.ClearFeature(features, flag)
}

// ToggleFeature toggles forwarder features. Deprecated: use def package.
func ToggleFeature(features, flag Features) Features {
	return defaultforwarderdef.ToggleFeature(features, flag)
}

// HasFeature checks forwarder features. Deprecated: use def package.
func HasFeature(features, flag Features) bool { return defaultforwarderdef.HasFeature(features, flag) }

// Params stores configurable options for the forwarder.
// Deprecated: use comp/forwarder/defaultforwarder/impl.Params instead.
type Params = defaultforwarderimpl.Params

// Options contains the configuration options for the forwarder.
// Deprecated: use comp/forwarder/defaultforwarder/impl.Options instead.
type Options = defaultforwarderimpl.Options

// NoopForwarder is a forwarder that does nothing.
// Deprecated: use comp/forwarder/defaultforwarder/impl.NoopForwarder instead.
type NoopForwarder = defaultforwarderimpl.NoopForwarder

// DefaultForwarder is the default forwarder implementation.
// Deprecated: use comp/forwarder/defaultforwarder/impl.DefaultForwarder instead.
type DefaultForwarder = defaultforwarderimpl.DefaultForwarder

// Mock implements mock-specific methods.
// Deprecated: use comp/forwarder/defaultforwarder/mock instead.
type Mock = defaultforwardermock.Mock

// NewParams creates new Params. Deprecated: use impl package.
var NewParams = defaultforwarderimpl.NewParams

// WithResolvers sets the resolver option. Deprecated: use impl package.
var WithResolvers = defaultforwarderimpl.WithResolvers

// WithDisableAPIKeyChecking disables API key checking. Deprecated: use impl package.
var WithDisableAPIKeyChecking = defaultforwarderimpl.WithDisableAPIKeyChecking

// WithFeatures sets features. Deprecated: use impl package.
var WithFeatures = defaultforwarderimpl.WithFeatures

// NewDefaultForwarder creates a new DefaultForwarder. Deprecated: use impl package.
var NewDefaultForwarder = defaultforwarderimpl.NewDefaultForwarder

// NewOptions creates new Options. Deprecated: use impl package.
var NewOptions = defaultforwarderimpl.NewOptions

// NewOptionsWithResolvers creates Options with resolvers. Deprecated: use impl package.
var NewOptionsWithResolvers = defaultforwarderimpl.NewOptionsWithResolvers

// NewHTTPClient creates a new HTTP client. Deprecated: use impl package.
var NewHTTPClient = defaultforwarderimpl.NewHTTPClient

// NewForwarder creates a new forwarder. Deprecated: use impl package.
var NewForwarder = defaultforwarderimpl.NewForwarder

// Module defines the fx options for this component.
// Deprecated: use comp/forwarder/defaultforwarder/fx.Module instead.
func Module(params Params) fxutil.Module {
	return defaultforwarderfx.Module(params)
}

// ModulWithOptionTMP defines the fx options with an option.
// Deprecated: use comp/forwarder/defaultforwarder/fx.ModuleWithOptionTMP instead.
func ModulWithOptionTMP(option fx.Option) fxutil.Module {
	return defaultforwarderfx.ModuleWithOptionTMP(option)
}

// MockModule defines the fx options for the mock component.
// Deprecated: use comp/forwarder/defaultforwarder/mock.MockModule instead.
func MockModule() fxutil.Module {
	return defaultforwardermock.MockModule()
}

// NoopModule provides a stub forwarder that does nothing.
// Deprecated: use comp/forwarder/defaultforwarder/fx.NoopModule instead.
func NoopModule() fxutil.Module {
	return defaultforwarderfx.NoopModule()
}
