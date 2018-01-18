// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package status

var (
	builder *Builder
)

// Source provides some information about a logs source
type Source struct {
	Type   string `json:"type"`
	Status string `json:"status"`
}

// Integration provides some information about a logs integration
type Integration struct {
	Name    string   `json:"name"`
	Sources []Source `json:"sources"`
}

// Status provides some information about logs-agent
type Status struct {
	IsRunning    bool          `json:"is_running"`
	Integrations []Integration `json:"integrations"`
}

// Initialize instanciates builder
func Initialize(sourcesToTrack []*SourceToTrack) {
	builder = NewBuilder(sourcesToTrack)
}

// Get returns the status of logs-agent computed on the fly
func Get() Status {
	return builder.Build()
}
