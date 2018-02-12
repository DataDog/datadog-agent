// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package status

import (
	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

var (
	builder *Builder
)

// Source provides some information about a logs source.
type Source struct {
	Type   string   `json:"type"`
	Status string   `json:"status"`
	Inputs []string `json:"inputs"`
	// TCP, UDP
	Port int `json:"port"`
	// File
	Path string `json:"path"`
	// Docker
	Image string `json:"image"`
	Label string `json:"label"`
}

// Integration provides some information about a logs integration.
type Integration struct {
	Name    string   `json:"name"`
	Sources []Source `json:"sources"`
}

// DeprecatedAttribute provides some information about the deprecated attributes that the user is still using in its configuration
type DeprecatedAttribute struct {
	Name        string `json:"name"`
	Replacement string `json:"replacement"`
}

// Status provides some information about logs-agent.
type Status struct {
	IsRunning            bool                  `json:"is_running"`
	DeprecatedAttributes []DeprecatedAttribute `json:"deprecated_attributes"`
	Integrations         []Integration         `json:"integrations"`
}

// Builder is used to build the status.
type Builder struct {
	sources              []*config.LogSource
	deprecatedAttributes []DeprecatedAttribute
}

// Initialize instantiates a builder that holds the deprecated attributes
// and the sources required to build the current status later on.
func Initialize(sources []*config.LogSource, attributes []config.DeprecatedAttribute) {
	// Convert attributes to JSON
	deprecatedAttributes := make([]DeprecatedAttribute, len(attributes))
	for i, attribute := range attributes {
		deprecatedAttributes[i] = DeprecatedAttribute{Name: attribute.Name, Replacement: attribute.Replacement}
	}
	// Instantiate the builder
	builder = &Builder{
		sources:              sources,
		deprecatedAttributes: deprecatedAttributes,
	}
}

// Get returns the status of the logs-agent computed on the fly.
func Get() Status {
	// Sort sources by name (ie. by integration name ~= file name)
	sources := make(map[string][]*config.LogSource)
	for _, source := range builder.sources {
		if _, exists := sources[source.Name]; !exists {
			sources[source.Name] = []*config.LogSource{}
		}
		sources[source.Name] = append(sources[source.Name], source)
	}
	// Convert to JSON
	var integrations []Integration
	for name, sourceList := range sources {
		var sources []Source
		for _, source := range sourceList {
			var status string
			if source.Status.IsPending() {
				status = "Pending"
			} else if source.Status.IsSuccess() {
				status = "OK"
			} else if source.Status.IsError() {
				status = source.Status.GetError()
			}
			sources = append(sources, Source{
				Type:   source.Config.Type,
				Status: status,
				Inputs: source.GetInputs(),
				Port:   source.Config.Port,
				Path:   source.Config.Path,
				Image:  source.Config.Image,
				Label:  source.Config.Label,
			})
		}
		integrations = append(integrations, Integration{Name: name, Sources: sources})
	}
	return Status{
		IsRunning:            true,
		DeprecatedAttributes: builder.deprecatedAttributes,
		Integrations:         integrations,
	}
}
