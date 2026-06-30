// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package mock provides a mock config files discovery component.
package mock

import (
	"go.uber.org/fx"

	configfilesdiscovery "github.com/DataDog/datadog-agent/comp/core/configfilesdiscovery/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type mockComponent struct{}

// Module provides a no-op mock component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(func() configfilesdiscovery.Component {
			return mockComponent{}
		}),
		fxutil.ProvideOptional[configfilesdiscovery.Component](),
	)
}
