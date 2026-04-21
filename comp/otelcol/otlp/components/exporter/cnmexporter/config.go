// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package cnmexporter

import "go.opentelemetry.io/collector/component"

const defaultMaxConnsPerMessage = 1000

// Config is the configuration for the CNM Datadog exporter.
type Config struct {
	// MaxConnsPerMessage controls the maximum number of connections per
	// CollectorConnections protobuf batch (default 1000).
	MaxConnsPerMessage int `mapstructure:"max_conns_per_message"`
}

func createDefaultConfig() component.Config {
	return &Config{
		MaxConnsPerMessage: defaultMaxConnsPerMessage,
	}
}
