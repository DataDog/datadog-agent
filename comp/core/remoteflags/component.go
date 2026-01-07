// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package remoteflags

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/remoteflags"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// team: agent-metrics-pipeline

// Component is the Remote Flags component interface.
type Component interface {
	// GetClient returns the remote flags client for subscribing to feature flags.
	GetClient() *remoteflags.Client
}

// NoneModule returns a None optional type for remoteflags.Component.
func NoneModule() fxutil.Module {
	return fxutil.Component(fx.Provide(func() option.Option[Component] {
		return option.None[Component]()
	}))
}
