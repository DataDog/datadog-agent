// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package status

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/stretchr/testify/assert"
)

func TestSourceAreGroupedByIntegrations(t *testing.T) {
	sources := config.NewLogSources()
	go func() {
		sources := sources.GetSourceStreamForType("foo")
		for range sources {
			// ensure that another component is consuming the channel to prevent
			// the producer to get stuck.
		}
	}()
	sources.AddSource(config.NewLogSource("foo", &config.LogsConfig{Type: "foo"}))
	sources.AddSource(config.NewLogSource("bar", &config.LogsConfig{Type: "foo"}))
	sources.AddSource(config.NewLogSource("foo", &config.LogsConfig{Type: "foo"}))

	Initialize(sources)

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
	sources := config.NewLogSources()
	go func() {
		sources := sources.GetSourceStreamForType("foo")
		for range sources {
			// ensure that another component is consuming the channel to prevent
			// the producer to get stuck.
		}
	}()
	sources.AddSource(config.NewLogSource("foo", &config.LogsConfig{Type: "foo"}))
	sources.AddSource(config.NewLogSource("bar", &config.LogsConfig{Type: "foo"}))
	sources.AddSource(config.NewLogSource("foo", &config.LogsConfig{Type: "foo"}))

	Initialize(sources)

	logSources := sources.GetSources()
	assert.Equal(t, 3, len(logSources))

	logSources[0].Messages.AddWarning("bar", "Unique Warning")
	for _, source := range logSources {
		source.Messages.AddWarning("foo", "Identical Warning")
	}

	status := Get()
	assert.ElementsMatch(t, []string{"Identical Warning", "Unique Warning"}, status.Messages)
}
