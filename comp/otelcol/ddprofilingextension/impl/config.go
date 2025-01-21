// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddprofilingextensionimpl defines the OpenTelemetry Extension implementation.
package ddprofilingextensionimpl

import "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/config"

// Config has the configuration for the api for the case of agentless uploads.
type Config struct {
	API config.APIConfig `mapstructure:"api"`
}
