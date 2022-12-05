// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package status

import (
	"expvar"
	"strings"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/status"
	sourcesPkg "github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// Builder is used to build the status.
type Builder struct {
	isRunning   *atomic.Bool
	endpoints   *config.Endpoints
	sources     *sourcesPkg.LogSources
	warnings    *config.Messages
	errors      *config.Messages
	logsExpVars *expvar.Map
}

// NewBuilder returns a new builder.
func NewBuilder(isRunning *atomic.Bool, endpoints *config.Endpoints, sources *sourcesPkg.LogSources, warnings *config.Messages, errors *config.Messages, logExpVars *expvar.Map) *Builder {
	return &Builder{
		isRunning:   isRunning,
		endpoints:   endpoints,
		sources:     sources,
		warnings:    warnings,
		errors:      errors,
		logsExpVars: logExpVars,
	}
}

// BuildStatus returns the status of the logs-agent.
func (b *Builder) BuildStatus() Status {
	return Status{
		IsRunning:     b.getIsRunning(),
		Endpoints:     b.getEndpoints(),
		Integrations:  b.getIntegrations(),
		StatusMetrics: b.getMetricsStatus(),
		Warnings:      b.getWarnings(),
		Errors:        b.getErrors(),
		UseHTTP:       b.getUseHTTP(),
	}
}

// getIsRunning returns true if the agent is running,
// this needs to be thread safe as it can be accessed
// from different commands (start, stop, status).
func (b *Builder) getIsRunning() bool {
	return b.isRunning.Load()
}

func (b *Builder) getUseHTTP() bool {
	return b.endpoints.UseHTTP
}

func (b *Builder) getEndpoints() []string {
	return b.endpoints.GetStatus()
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
				AllTimeAvgLatency:  source.LatencyStats.AllTimeAvg() / int64(time.Millisecond),
				AllTimePeakLatency: source.LatencyStats.AllTimePeak() / int64(time.Millisecond),
				RecentAvgLatency:   source.LatencyStats.MovingAvg() / int64(time.Millisecond),
				RecentPeakLatency:  source.LatencyStats.MovingPeak() / int64(time.Millisecond),
				Type:               source.Config.Type,
				Configuration:      b.toDictionary(source.Config),
				Status:             b.toString(source.Status),
				Inputs:             source.GetInputs(),
				Messages:           source.Messages.GetMessages(),
				Info:               source.GetInfoStatus(),
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
func (b *Builder) groupSourcesByName() map[string][]*sourcesPkg.LogSource {
	sources := make(map[string][]*sourcesPkg.LogSource)
	for _, source := range b.sources.GetSources() {
		if source.IsHiddenFromStatus() {
			continue
		}
		if _, exists := sources[source.Name]; !exists {
			sources[source.Name] = []*sourcesPkg.LogSource{}
		}
		sources[source.Name] = append(sources[source.Name], source)
	}
	return sources
}

// toString returns a representation of a status.
func (b *Builder) toString(status *status.LogStatus) string {
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
	dictionary["Service"] = c.Service
	dictionary["Source"] = c.Source
	switch c.Type {
	case config.TCPType, config.UDPType:
		dictionary["Port"] = c.Port
	case config.FileType:
		dictionary["Path"] = c.Path
		dictionary["TailingMode"] = c.TailingMode
		dictionary["Identifier"] = c.Identifier
	case config.DockerType:
		dictionary["Image"] = c.Image
		dictionary["Label"] = c.Label
		dictionary["Name"] = c.Name
	case config.JournaldType:
		dictionary["IncludeSystemUnits"] = strings.Join(c.IncludeSystemUnits, ", ")
		dictionary["ExcludeSystemUnits"] = strings.Join(c.ExcludeSystemUnits, ", ")
		dictionary["IncludeUserUnits"] = strings.Join(c.IncludeUserUnits, ", ")
		dictionary["ExcludeUserUnits"] = strings.Join(c.ExcludeUserUnits, ", ")
		dictionary["IncludeMatches"] = strings.Join(c.IncludeMatches, ", ")
		dictionary["ExcludeMatches"] = strings.Join(c.ExcludeMatches, ", ")
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

// getMetricsStatus exposes some aggregated metrics of the log agent on the agent status
func (b *Builder) getMetricsStatus() map[string]int64 {
	var metrics = make(map[string]int64, 2)
	metrics["LogsProcessed"] = b.logsExpVars.Get("LogsProcessed").(*expvar.Int).Value()
	metrics["LogsSent"] = b.logsExpVars.Get("LogsSent").(*expvar.Int).Value()
	metrics["BytesSent"] = b.logsExpVars.Get("BytesSent").(*expvar.Int).Value()
	metrics["EncodedBytesSent"] = b.logsExpVars.Get("EncodedBytesSent").(*expvar.Int).Value()
	return metrics
}
