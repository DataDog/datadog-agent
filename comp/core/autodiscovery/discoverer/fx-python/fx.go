// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fxpython assembles the configuration-discovery engine with a Python probe.
package fxpython

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/discoverer"
	discovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/discoverer/def"
	collectorpython "github.com/DataDog/datadog-agent/pkg/collector/python"
)

type pythonConfigDiscoverer struct{}

func (pythonConfigDiscoverer) DiscoverConfig(integrationName, serviceJSON string) (string, error) {
	return collectorpython.DiscoverConfig(integrationName, serviceJSON)
}

// Module provides the optional discovery engine backed by Python integrations.
func Module() fx.Option {
	return fx.Module(
		"comp/core/autodiscovery/discoverer/fx-python",
		fx.Provide(func() discovery.Factory {
			return discoverer.NewFactory(pythonConfigDiscoverer{})
		}),
	)
}
