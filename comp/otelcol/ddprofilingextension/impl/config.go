// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddprofilingextensionimpl defines the OpenTelemetry Extension implementation.
package ddprofilingextensionimpl

import "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/config"

// Config contains the config of the profiler.
type Config struct {
	// API contains the configuration for the api for the case of agentless uploads.
	// api site is used in non agentless upload setups as well.
	// Not setting API section leads to upload to an agent with.
	API             config.APIConfig `mapstructure:"api"`
	ProfilerOptions ProfilerOptions  `mapstructure:"profiler_options"`
	// Endpoint reports the endpoint used for profiles.
	// Default: BuildInfo.Version (e.g. v0.117.0)
	Endpoint string `mapstructure:"endpoint"`
}

// ProfilerOptions defines settings relevant to the profiler.
type ProfilerOptions struct {
	// Service the profiler will report with.
	// Default: BuildInfo.Command (e.g. otel-agent)
	Service string `mapstructure:"service"`
	// Env the profiler will report with.
	// Default: none
	Env string `mapstructure:"env"`
	// Version the profiler will report with.
	// Default: BuildInfo.Version (e.g. v0.117.0)
	Version string `mapstructure:"version"`
	// Period in seconds the profiler will report with.
	// Default: 60s
	Period int `mapstructure:"period"`
	// ProfileTypes specifies additional profile types to enable.
	// supported values are blockprofile, mutexprofile and goroutineprofile.
	// By default CPU and Heap profiles are enabled.
	ProfileTypes []string `mapstructure:"profile_types"`
}
