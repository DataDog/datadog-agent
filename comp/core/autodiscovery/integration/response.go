// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package integration

// ConfigResponse holds information about the config
// the instance IDs are precomputed to avoid discrepancies between the server and the client
// The InstanceIDs must have the same order as the instances in the Config struct
type ConfigResponse struct {
	InstanceIDs []string `json:"instance_ids"`
	Config      Config   `json:"config"`
}

// ServiceResponse holds information about a tracked service
type ServiceResponse struct {
	ServiceID      string            `json:"service_id"`
	ADIdentifiers  []string          `json:"ad_identifiers"`
	Hosts          map[string]string `json:"hosts,omitempty"`
	Ports          []string          `json:"ports,omitempty"`
	PID            int               `json:"pid,omitempty"`
	Hostname       string            `json:"hostname,omitempty"`
	IsReady        bool              `json:"is_ready"`
	FiltersMetrics bool              `json:"filters_metrics"`
	FiltersLogs    bool              `json:"filters_logs"`
}

// ConfigCheckResponse holds the config check response
type ConfigCheckResponse struct {
	Configs         []ConfigResponse    `json:"configs"`
	ResolveWarnings map[string][]string `json:"resolve_warnings"`
	ConfigErrors    map[string]string   `json:"config_errors"`
	Unresolved      map[string]Config   `json:"unresolved"`
	Services        []ServiceResponse   `json:"services,omitempty"`
}
