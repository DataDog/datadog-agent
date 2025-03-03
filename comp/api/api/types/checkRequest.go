// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types provides types for the API package
package types

// MemoryProfileConfig represents the configuration for memory profiling
type MemoryProfileConfig struct {
	Dir     string `json:"dir"`
	Frames  string `json:"frames"`
	GC      string `json:"gc"`
	Combine string `json:"combine"`
	Sort    string `json:"sort"`
	Limit   string `json:"limit"`
	Diff    string `json:"diff"`
	Filters string `json:"filters"`
	Unit    string `json:"unit"`
	Verbose string `json:"verbose"`
}

// CheckRequest represents the request to run a check
type CheckRequest struct {
	Name          string              `json:"name"`
	Times         int                 `json:"times"`
	Pause         int                 `json:"pause"`
	Delay         int                 `json:"delay"`
	ProfileConfig MemoryProfileConfig `json:"profileConfig"`
	Breakpoint    string              `json:"breakpoint"`
}
