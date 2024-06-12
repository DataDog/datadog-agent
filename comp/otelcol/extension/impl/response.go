// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package impl defines the OpenTelemetry Extension implementation.
package impl

// BuildInfoResponse is the response struct for BuildInfo
type BuildInfoResponse struct {
	AgentVersion string `json:"version"`
	AgentCommand string `json:"command"`
	AgentDesc    string `json:"description"`
}

// ConfigResponse is the response struct for Config
type ConfigResponse struct {
	CustomerConfig        string `json:"customer_configuration"`
	EnvConfig             string `json:"environment_configuration"`
	RuntimeOverrideConfig string `json:"runtime_override_configuration"`
	RuntimeConfig         string `json:"runtime_configuration"`
}

// OTelFlareSource is the response struct for flare debug sources
type OTelFlareSource struct {
	URL   string `json:"url"`
	Crawl bool   `json:"crawl"`
}

// DebugSourceResponse is the response struct for a map of OTelFlareSource
type DebugSourceResponse struct {
	Sources map[string]OTelFlareSource `json:"sources,omitempty"`
}

// Response is the response struct for API queries
type Response struct {
	BuildInfoResponse
	ConfigResponse
	DebugSourceResponse
	Environment map[string]string `json:"environment,omitempty"`
}
