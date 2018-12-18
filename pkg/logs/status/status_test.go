// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package status

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
)

func consumeSources(sources *config.LogSources) {
	go func() {
		sources := sources.GetAddedForType("foo")
		for range sources {
			// ensure that another component is consuming the channel to prevent
			// the producer to get stuck.
		}
	}()
}

func createSources() *config.LogSources {
	sources := config.NewLogSources()
	consumeSources(sources)
	sources.AddSource(config.NewLogSource("foo", &config.LogsConfig{Type: "foo"}))
	sources.AddSource(config.NewLogSource("bar", &config.LogsConfig{Type: "foo"}))
	sources.AddSource(config.NewLogSource("foo", &config.LogsConfig{Type: "foo"}))
	Initialize(sources)
	return sources
}

func TestSourceAreGroupedByIntegrations(t *testing.T) {
	defer Clear()
	createSources()

	status := Get()
	assert.Equal(t, true, status.IsRunning)
	assert.Equal(t, 2, len(status.Integrations))

	for _, integration := range status.Integrations {
		switch integration.Name {
		case "foo":
			assert.Equal(t, 2, len(integration.Sources))
		case "bar":
			assert.Equal(t, 1, len(integration.Sources))
		default:
			assert.Fail(t, fmt.Sprintf("Expected foo or bar, got %s", integration.Name))
		}
	}
}

func TestStatusDeduplicateWarnings(t *testing.T) {
	defer Clear()
	sources := createSources()

	logSources := sources.GetSources()
	assert.Equal(t, 3, len(logSources))

	logSources[0].Messages.AddWarning("bar", "Unique Warning")
	for _, source := range logSources {
		source.Messages.AddWarning("foo", "Identical Warning")
	}

	status := Get()
	assert.ElementsMatch(t, []string{"Identical Warning", "Unique Warning"}, status.Messages)
}

func TestMetrics(t *testing.T) {
	defer Clear()
	Clear()
	assert.Equal(t, metrics.LogsExpvars.String(), `{"DestinationErrors": 0, "DestinationLogsDropped": {}, "IsRunning": false, "LogsDecoded": 0, "LogsProcessed": 0, "LogsSent": 0, "Warnings": ""}`)

	sources := createSources()
	logSources := sources.GetSources()
	logSources[0].Messages.AddWarning("bar", "Unique Warning")
	assert.Equal(t, metrics.LogsExpvars.String(), `{"DestinationErrors": 0, "DestinationLogsDropped": {}, "IsRunning": true, "LogsDecoded": 0, "LogsProcessed": 0, "LogsSent": 0, "Warnings": "Unique Warning"}`)
}
