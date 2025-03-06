// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types provides types for the API package
package types

import (
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
)

// EventPlatformEvent represents an event from the event platform
type EventPlatformEvent struct {
	RawEvent          string `json:",omitempty"`
	EventType         string
	UnmarshalledEvent map[string]interface{} `json:",omitempty"`
}

// AggregatorData represents the data from the aggregator
type AggregatorData struct {
	Series              metrics.Series                  `json:"series"`
	SketchSeries        []*metrics.SketchSeries         `json:"sketch_series"`
	ServiceCheck        servicecheck.ServiceChecks      `json:"service_check"`
	Events              event.Events                    `json:"events"`
	EventPlatformEvents map[string][]EventPlatformEvent `json:"event_platform_events"`
}

// CheckResponse represents the response of a check
type CheckResponse struct {
	Results        []*stats.Stats                    `json:"results"`
	Errors         []string                          `json:"errors"`
	Warnings       []string                          `json:"warnings"`
	Metadata       map[string]map[string]interface{} `json:"metadata"`
	AggregatorData AggregatorData                    `json:"aggregator_data"`
}

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
