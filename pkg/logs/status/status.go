// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package status

import (
	"expvar"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
)

var (
	builder  *Builder
	warnings *config.Messages
	errors   *config.Messages
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
	IsRunning     bool             `json:"is_running"`
	Endpoints     []string         `json:"endpoints"`
	StatusMetrics map[string]int64 `json:"metrics"`
	Integrations  []Integration    `json:"integrations"`
	Errors        []string         `json:"errors"`
	Warnings      []string         `json:"warnings"`
	UseHTTP       bool             `json:"use_http"`
}

// Init instantiates the builder that builds the status on the fly.
func Init(isRunning *int32, endpoints *config.Endpoints, sources *config.LogSources, logExpVars *expvar.Map) {
	warnings = config.NewMessages()
	errors = config.NewMessages()
	builder = NewBuilder(isRunning, endpoints, sources, warnings, errors, logExpVars)
}

// Clear clears the status which means it needs to be initialized again to be used.
func Clear() {
	builder = nil
	warnings = nil
	errors = nil
}

// Get returns the status of the logs-agent computed on the fly.
func Get() Status {
	if builder == nil {
		return Status{
			IsRunning: false,
		}
	}
	return builder.BuildStatus()
}

// AddGlobalWarning keeps track of a warning message to display on the status.
func AddGlobalWarning(key string, warning string) {
	if warnings != nil {
		warnings.AddMessage(key, warning)
	}
}

// RemoveGlobalWarning loses track of a warning message
// that does not need to be displayed on the status anymore.
func RemoveGlobalWarning(key string) {
	if warnings != nil {
		warnings.RemoveMessage(key)
	}
}

// AddGlobalError an error message for the status display (errors will stop the agent)
func AddGlobalError(key string, errorMessage string) {
	if errors != nil {
		errors.AddMessage(key, errorMessage)
	}
}

func init() {
	metrics.LogsExpvars.Set("Errors", expvar.Func(func() interface{} {
		return strings.Join(Get().Errors, ", ")
	}))
	metrics.LogsExpvars.Set("Warnings", expvar.Func(func() interface{} {
		return strings.Join(Get().Warnings, ", ")
	}))
	metrics.LogsExpvars.Set("IsRunning", expvar.Func(func() interface{} {
		return Get().IsRunning
	}))
}
