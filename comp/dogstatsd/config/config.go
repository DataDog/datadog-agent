// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config implements configuration helpers
package config

import (
	"os"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

// team: agent-metric-pipelines

// TODO: Port over more shared configuration settings, specifically listener addresses. Anything that we use in
// `comp/dogstatsd` _and_ other components/packages in the codebase.

// Config implements the configuration for DogStatsD
type Config struct {
	config config.Component
}

// NewConfig creates a new Config instance
func NewConfig(config config.Component) *Config {
	return &Config{
		config: config,
	}
}

// Enabled returns true if DogStatsD is enabled in any mode
//
// This covers both both possible modes of DSD running internally (Core Agent) and via Agent Data Plane.
func (c *Config) Enabled() bool {
	// `use_dogstatsd` is the baseline configuration for enabling DogStatsD:
	// - when `false`, DogStatsD shouldn't be started at all in either Core Agent or ADP
	// - when `true`, then we know either the Core Agent or ADP will handle it, depending on additional configuration
	return c.config.GetBool("use_dogstatsd")
}

// EnabledInternal returns true if DogStatsD is enabled internally
func (c *Config) EnabledInternal() bool {
	// We only enable DSD internally if it's enabled in a baseline fashion _and_ the data plane is not handling it.
	return c.Enabled() && !c.enabledDataPlane()
}

// enabledDataPlane returns true if DogStatsD is enabled via Agent Data Plane
func (c *Config) enabledDataPlane() bool {
	// `DD_ADP_ENABLED` is a deprecated environment variable for signaling that Agent Data Plane is running _and_ that
	// it's handling DogStatsD traffic.
	//
	// This is now split into two separate settings: `data_plane.enabled` and `data_plane.dogstatsd.enabled`, which
	// indicate whether ADP is enabled at all and whether it's handling DogStatsD traffic, respectively.
	dsdEnabledDataPlaneOldStyle := os.Getenv("DD_ADP_ENABLED") == "true"

	// ADP has a global enable flag that controls whether or not it runs, and then a per-feature enable flag, which we
	// check to see if enabled for DogStatsD.
	dsdEnabledDataPlane := c.config.GetBool("data_plane.enabled") && c.config.GetBool("data_plane.dogstatsd.enabled")

	return c.Enabled() && (dsdEnabledDataPlaneOldStyle || dsdEnabledDataPlane)
}
