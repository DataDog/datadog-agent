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

func createSources() *config.LogSources {
	return CreateSources([]*config.LogSource{
		config.NewLogSource("foo", &config.LogsConfig{Type: "foo"}),
		config.NewLogSource("bar", &config.LogsConfig{Type: "foo"}),
		config.NewLogSource("foo", &config.LogsConfig{Type: "foo"}),
	})
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
	createSources()

	AddGlobalWarning("bar", "Unique Warning")
	AddGlobalWarning("foo", "Identical Warning")
	AddGlobalWarning("foo", "Identical Warning")
	AddGlobalWarning("foo", "Identical Warning")

	status := Get()
	assert.ElementsMatch(t, []string{"Identical Warning", "Unique Warning"}, status.Messages)

	RemoveGlobalWarning("foo")
	status = Get()
	assert.ElementsMatch(t, []string{"Unique Warning"}, status.Messages)
}

func TestMetrics(t *testing.T) {
	defer Clear()
	Clear()
	assert.Equal(t, metrics.LogsExpvars.String(), `{"DestinationErrors": 0, "DestinationLogsDropped": {}, "IsRunning": false, "LogsDecoded": 0, "LogsProcessed": 0, "LogsSent": 0, "Warnings": ""}`)

	createSources()
	AddGlobalWarning("bar", "Unique Warning")
	assert.Equal(t, metrics.LogsExpvars.String(), `{"DestinationErrors": 0, "DestinationLogsDropped": {}, "IsRunning": true, "LogsDecoded": 0, "LogsProcessed": 0, "LogsSent": 0, "Warnings": "Unique Warning"}`)
}
