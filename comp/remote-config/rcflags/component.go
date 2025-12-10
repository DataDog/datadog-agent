// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package rcflags provides the Remote Flags component for feature flag management.
package rcflags

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/remoteflags"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// team: remote-config

// Component is the Remote Flags component interface.
// It provides access to the remote flags client for subscribing to feature flags.
type Component interface {
	// GetClient returns the remote flags client for subscribing to feature flags.
	GetClient() *remoteflags.Client
}

// NoneModule returns a None optional type for rcflags.Component.
//
// This helper allows code that needs a disabled Optional type for rcflags to get it.
func NoneModule() fxutil.Module {
	return fxutil.Component(fx.Provide(func() option.Option[Component] {
		return option.None[Component]()
	}))
}
