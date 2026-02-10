// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the fx module for the configstreamconsumer component
package fx

import (
	"go.uber.org/fx"

	configstreamconsumer "github.com/DataDog/datadog-agent/comp/core/configstreamconsumer/def"
	configstreamconsumerimpl "github.com/DataDog/datadog-agent/comp/core/configstreamconsumer/impl"
)

// Module defines the fx options for this component
func Module() fx.Option {
	return fx.Module("configstreamconsumer",
		fx.Provide(configstreamconsumerimpl.NewComponent))
}

// MockModule defines the fx options for the mock component
func MockModule() fx.Option {
	return fx.Module("configstreamconsumer",
		fx.Provide(func() configstreamconsumer.Component {
			return nil
		}))
}
