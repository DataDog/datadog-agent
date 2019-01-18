// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package status

import (
	"expvar"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/types"
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
	sources  types.LogSources
	messages *types.Messages
}

// Initialize instantiates a builder that holds the sources required to build the current status later on.
func Initialize(sources types.LogSources) {
	builder = &Builder{
		sources:  sources,
		messages: types.NewMessages(),
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
	sources := make(map[string][]types.LogSource)
	for _, source := range builder.sources.GetSources() {
		if _, exists := sources[source.GetName()]; !exists {
			sources[source.GetName()] = []types.LogSource{}
		}
		sources[source.GetName()] = append(sources[source.GetName()], source)
	}
	// Convert to json
	var integrations []Integration
	warningsDeduplicator := make(map[string]struct{})
	for name, sourceList := range sources {
		var sources []Source
		for _, source := range sourceList {
			var status string
			if source.GetStatus().IsPending() {
				status = "Pending"
			} else if source.GetStatus().IsSuccess() {
				status = "OK"
			} else if source.GetStatus().IsError() {
				status = source.GetStatus().GetError()
			}

			sources = append(sources, Source{
				Type:          source.GetConfig().GetType(),
				Configuration: toDictionary(source.GetConfig()),
				Status:        status,
				Inputs:        source.GetInputs(),
				Messages:      source.GetMessages().GetMessages(),
			})

			for _, warning := range builder.messages.GetWarnings() {
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

// AddWarning create a warning
func AddGlobalWarning(key string, warning string) {
	builder.messages.AddWarning(key, warning)
}

// RemoveWarning removes a warning
func RemoveGlobalWarning(key string) {
	builder.messages.RemoveWarning(key)
}

// toDictionary returns a representation of the configuration
func toDictionary(c types.LogsConfig) map[string]interface{} {
	dictionary := make(map[string]interface{})
	switch c.GetType() {
	case types.TCPType, types.UDPType:
		dictionary["Port"] = c.GetPort()
	case types.FileType:
		dictionary["Path"] = c.GetPath()
	case types.DockerType:
		dictionary["Image"] = c.GetImage()
		dictionary["Label"] = c.GetLabel()
		dictionary["Name"] = c.GetName()
	case types.JournaldType:
		dictionary["IncludeUnits"] = strings.Join(c.GetIncludeUnits(), ", ")
		dictionary["ExcludeUnits"] = strings.Join(c.GetExcludeUnits(), ", ")
	case types.WindowsEventType:
		dictionary["ChannelPath"] = c.GetChannelPath()
		dictionary["Query"] = c.GetQuery()
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
