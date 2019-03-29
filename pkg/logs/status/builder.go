// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package status

import (
	"strings"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

// Builder is used to build the status.
type Builder struct {
	isRunning *int32
	sources   *config.LogSources
	warnings  *config.Messages
	errors    *config.Messages
}

// NewBuilder returns a new builder.
func NewBuilder(isRunning *int32, sources *config.LogSources, warnings *config.Messages, errors *config.Messages) *Builder {
	return &Builder{
		isRunning: isRunning,
		sources:   sources,
		warnings:  warnings,
		errors:    errors,
	}
}

// BuildStatus returns the status of the logs-agent.
func (b *Builder) BuildStatus() Status {
	return Status{
		IsRunning:    b.getIsRunning(),
		Integrations: b.getIntegrations(),
		Warnings:     b.getWarnings(),
		Errors:       b.getErrors(),
	}
}

// getIsRunning returns true if the agent is running,
// this needs to be thread safe as it can be accessed
// from different commands (start, stop, status).
func (b *Builder) getIsRunning() bool {
	return atomic.LoadInt32(b.isRunning) != 0
}

// getWarnings returns all the warning messages that
// have been accumulated during the life cycle of the logs-agent.
func (b *Builder) getWarnings() []string {
	return b.warnings.GetMessages()
}

// getErrors returns all the errors messages which are responsible
// for shutting down the logs-agent
func (b *Builder) getErrors() []string {
	return b.errors.GetMessages()
}

// getIntegrations returns all the information about the logs integrations.
func (b *Builder) getIntegrations() []Integration {
	var integrations []Integration
	for name, logSources := range b.groupSourcesByName() {
		var sources []Source
		for _, source := range logSources {
			sources = append(sources, Source{
				Type:          source.Config.Type,
				Configuration: b.toDictionary(source.Config),
				Status:        b.toString(source.Status),
				Inputs:        source.GetInputs(),
				Messages:      source.Messages.GetMessages(),
			})
		}
		integrations = append(integrations, Integration{
			Name:    name,
			Sources: sources,
		})
	}
	return integrations
}

// groupSourcesByName groups all logs sources by name so that they get properly displayed
// on the agent status.
func (b *Builder) groupSourcesByName() map[string][]*config.LogSource {
	sources := make(map[string][]*config.LogSource)
	for _, source := range b.sources.GetSources() {
		if _, exists := sources[source.Name]; !exists {
			sources[source.Name] = []*config.LogSource{}
		}
		sources[source.Name] = append(sources[source.Name], source)
	}
	return sources
}

// toString returns a representation of a status.
func (b *Builder) toString(status *config.LogStatus) string {
	var value string
	if status.IsPending() {
		value = "Pending"
	} else if status.IsSuccess() {
		value = "OK"
	} else if status.IsError() {
		value = status.GetError()
	}
	return value
}

// toDictionary returns a representation of the configuration.
func (b *Builder) toDictionary(c *config.LogsConfig) map[string]interface{} {
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
