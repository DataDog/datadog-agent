// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package status

// Error provides some information about an error that occurred when parsing a logs integration file
type Error struct {
	Message string `json:"message"`
}

// Source provides some information about a logs source from an integration file
type Source struct {
	Type string `json:"type"`
	Info string `json:"info"`
}

// Integration provides some information about a logs integration file
type Integration struct {
	Name    string   `json:"name"`
	Sources []Source `json:"sources"`
	Errors  []Error  `json:"errors"`
}

// Status provides some information about logs-agent
type Status struct {
	IsEnabled    bool          `json:"is_enabled"`
	Integrations []Integration `json:"integrations"`
}
