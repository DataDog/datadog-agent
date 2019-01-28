// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package status

import (
	"expvar"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
)

var (
	builder *Builder
)

// Source provides some information about a logs source.
type Source struct {
	Type          string                 `json:"type"`
	Configuration map[string]interface{} `json:"configuration"`
	Status        string                 `json:"status"`
	Inputs        []string               `json:"inputs"`
	Messages      []string               `json:"messages"`
}

// Integration provides some information about a logs integration.
type Integration struct {
	Name    string   `json:"name"`
	Sources []Source `json:"sources"`
}

// Status provides some information about logs-agent.
type Status struct {
	IsRunning    bool          `json:"is_running"`
	Integrations []Integration `json:"integrations"`
	Messages     []string      `json:"messages"`
}

// Builder is used to build the status.
type Builder struct {
	sources  *config.LogSources
	warnings *config.Messages
}

// Initialize instantiates a builder that holds the sources required to build the current status later on.
func Initialize(sources *config.LogSources) {
	builder = &Builder{
		sources:  sources,
		warnings: config.NewMessages(),
	}
}

// Clear clears the status (which means it needs to be initialized again to be used).
func Clear() {
	builder = nil
}

// Get returns the status of the logs-agent computed on the fly.
func Get() Status {
	if builder == nil {
		return Status{
			IsRunning: false,
		}
	}
	// Sort sources by name (ie. by integration name ~= file name)
	sources := make(map[string][]*config.LogSource)
	for _, source := range builder.sources.GetSources() {
		if _, exists := sources[source.Name]; !exists {
			sources[source.Name] = []*config.LogSource{}
		}
		sources[source.Name] = append(sources[source.Name], source)
	}
	// Convert to json
	var integrations []Integration
	warningsDeduplicator := make(map[string]struct{})
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
				Type:          source.Config.Type,
				Configuration: toDictionary(source.Config),
				Status:        status,
				Inputs:        source.GetInputs(),
				Messages:      source.Messages.GetMessages(),
			})

			for _, warning := range builder.warnings.GetWarnings() {
				warningsDeduplicator[warning] = struct{}{}
			}
		}
		integrations = append(integrations, Integration{Name: name, Sources: sources})
	}

	var warnings []string
	for warning := range warningsDeduplicator {
		warnings = append(warnings, warning)
	}

	return Status{
		IsRunning:    true,
		Integrations: integrations,
		Messages:     warnings,
	}
}

// AddGlobalWarning create a warning
func AddGlobalWarning(key string, warning string) {
	if builder != nil {
		builder.warnings.AddWarning(key, warning)
	}
}

// RemoveGlobalWarning removes a warning
func RemoveGlobalWarning(key string) {
	if builder != nil {
		builder.warnings.RemoveWarning(key)
	}
}

// toDictionary returns a representation of the configuration
func toDictionary(c *config.LogsConfig) map[string]interface{} {
	dictionary := make(map[string]interface{})
	switch c.Type {
	case config.TCPType, config.UDPType:
		dictionary["Port"] = c.Port
	case config.FileType:
		dictionary["Path"] = c.Path
	case config.DockerType:
		dictionary["Image"] = c.Image
		dictionary["Label"] = c.Label
		dictionary["Name"] = c.Name
	case config.JournaldType:
		dictionary["IncludeUnits"] = strings.Join(c.IncludeUnits, ", ")
		dictionary["ExcludeUnits"] = strings.Join(c.ExcludeUnits, ", ")
	case config.WindowsEventType:
		dictionary["ChannelPath"] = c.ChannelPath
		dictionary["Query"] = c.Query
	}
	for k, v := range dictionary {
		if v == "" {
			delete(dictionary, k)
		}
	}
	return dictionary
}

func init() {
	metrics.LogsExpvars.Set("Warnings", expvar.Func(func() interface{} {
		return strings.Join(Get().Messages, ", ")
	}))
	metrics.LogsExpvars.Set("IsRunning", expvar.Func(func() interface{} {
		return Get().IsRunning
	}))
}
