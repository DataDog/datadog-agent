// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package status

import (
	"fmt"
	"math"
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
	assert.ElementsMatch(t, []string{"Identical Warning", "Unique Warning"}, status.Warnings)

	RemoveGlobalWarning("foo")
	status = Get()
	assert.ElementsMatch(t, []string{"Unique Warning"}, status.Warnings)
}

func TestStatusDeduplicateErrors(t *testing.T) {
	defer Clear()
	createSources()

	AddGlobalError("bar", "Unique Error")
	AddGlobalError("foo", "Identical Error")
	AddGlobalError("foo", "Identical Error")

	status := Get()
	assert.ElementsMatch(t, []string{"Identical Error", "Unique Error"}, status.Errors)
}

func TestStatusDeduplicateErrorsAndWarnings(t *testing.T) {
	defer Clear()
	createSources()

	AddGlobalWarning("bar", "Unique Warning")
	AddGlobalWarning("foo", "Identical Warning")
	AddGlobalWarning("foo", "Identical Warning")
	AddGlobalError("bar", "Unique Error")
	AddGlobalError("foo", "Identical Error")
	AddGlobalError("foo", "Identical Error")

	status := Get()
	assert.ElementsMatch(t, []string{"Identical Error", "Unique Error"}, status.Errors)
	assert.ElementsMatch(t, []string{"Identical Warning", "Unique Warning"}, status.Warnings)
}

func TestMetrics(t *testing.T) {
	defer Clear()
	Clear()
	var expected = `{"BytesSent": 0, "DestinationErrors": 0, "DestinationLogsDropped": {}, "EncodedBytesSent": 0, "Errors": "", "IsRunning": false, "LogsDecoded": 0, "LogsProcessed": 0, "LogsSent": 0, "Warnings": ""}`
	assert.Equal(t, expected, metrics.LogsExpvars.String())

	createSources()
	AddGlobalWarning("bar", "Unique Warning")
	AddGlobalError("bar", "I am an error")
	expected = `{"BytesSent": 0, "DestinationErrors": 0, "DestinationLogsDropped": {}, "EncodedBytesSent": 0, "Errors": "I am an error", "IsRunning": true, "LogsDecoded": 0, "LogsProcessed": 0, "LogsSent": 0, "Warnings": "Unique Warning"}`
	assert.Equal(t, expected, metrics.LogsExpvars.String())
}

func TestStatusMetrics(t *testing.T) {
	defer Clear()
	createSources()

	status := Get()
	assert.Equal(t, int64(0), status.StatusMetrics["LogsProcessed"])
	assert.Equal(t, int64(0), status.StatusMetrics["LogsSent"])

	metrics.LogsProcessed.Set(5)
	metrics.LogsSent.Set(3)
	status = Get()

	assert.Equal(t, int64(5), status.StatusMetrics["LogsProcessed"])
	assert.Equal(t, int64(3), status.StatusMetrics["LogsSent"])

	metrics.LogsProcessed.Set(math.MaxInt64)
	metrics.LogsProcessed.Add(1)
	status = Get()
	assert.Equal(t, int64(math.MinInt64), status.StatusMetrics["LogsProcessed"])
}
