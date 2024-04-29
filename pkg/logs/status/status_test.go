// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

package status

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/util/testutils"
)

func initStatus() {
	InitStatus(pkgConfig.Datadog, testutils.CreateSources([]*sources.LogSource{
		sources.NewLogSource("foo", &config.LogsConfig{Type: "foo"}),
		sources.NewLogSource("bar", &config.LogsConfig{Type: "foo"}),
		sources.NewLogSource("foo", &config.LogsConfig{Type: "foo"}),
	}))
}

func TestSourceAreGroupedByIntegrations(t *testing.T) {
	defer Clear()
	initStatus()

	status := Get(false)
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
	initStatus()

	AddGlobalWarning("bar", "Unique Warning")
	AddGlobalWarning("foo", "Identical Warning")
	AddGlobalWarning("foo", "Identical Warning")
	AddGlobalWarning("foo", "Identical Warning")

	status := Get(false)
	assert.ElementsMatch(t, []string{"Identical Warning", "Unique Warning"}, status.Warnings)

	RemoveGlobalWarning("foo")
	status = Get(false)
	assert.ElementsMatch(t, []string{"Unique Warning"}, status.Warnings)
}

func TestStatusDeduplicateErrors(t *testing.T) {
	defer Clear()
	initStatus()

	AddGlobalError("bar", "Unique Error")
	AddGlobalError("foo", "Identical Error")
	AddGlobalError("foo", "Identical Error")

	status := Get(false)
	assert.ElementsMatch(t, []string{"Identical Error", "Unique Error"}, status.Errors)
}

func TestStatusDeduplicateErrorsAndWarnings(t *testing.T) {
	defer Clear()
	initStatus()

	AddGlobalWarning("bar", "Unique Warning")
	AddGlobalWarning("foo", "Identical Warning")
	AddGlobalWarning("foo", "Identical Warning")
	AddGlobalError("bar", "Unique Error")
	AddGlobalError("foo", "Identical Error")
	AddGlobalError("foo", "Identical Error")

	status := Get(false)
	assert.ElementsMatch(t, []string{"Identical Error", "Unique Error"}, status.Errors)
	assert.ElementsMatch(t, []string{"Identical Warning", "Unique Warning"}, status.Warnings)
}

func TestMetrics(t *testing.T) {
	defer Clear()
	Clear()
	var expected = `{"BytesSent": 0, "DestinationErrors": 0, "DestinationLogsDropped": {}, "EncodedBytesSent": 0, "Errors": "", "HttpDestinationStats": {}, "IsRunning": false, "LogsDecoded": 0, "LogsProcessed": 0, "LogsSent": 0, "RetryCount": 0, "RetryTimeSpent": 0, "SenderLatency": 0, "Warnings": ""}`
	assert.Equal(t, expected, metrics.LogsExpvars.String())

	initStatus()
	AddGlobalWarning("bar", "Unique Warning")
	AddGlobalError("bar", "I am an error")
	expected = `{"BytesSent": 0, "DestinationErrors": 0, "DestinationLogsDropped": {}, "EncodedBytesSent": 0, "Errors": "I am an error", "HttpDestinationStats": {}, "IsRunning": true, "LogsDecoded": 0, "LogsProcessed": 0, "LogsSent": 0, "RetryCount": 0, "RetryTimeSpent": 0, "SenderLatency": 0, "Warnings": "Unique Warning"}`
	assert.Equal(t, expected, metrics.LogsExpvars.String())
}

func TestStatusMetrics(t *testing.T) {
	defer Clear()
	initStatus()

	status := Get(false)
	assert.Equal(t, "0", status.StatusMetrics["LogsProcessed"])
	assert.Equal(t, "0", status.StatusMetrics["LogsSent"])
	assert.Equal(t, "0", status.StatusMetrics["BytesSent"])
	assert.Equal(t, "0", status.StatusMetrics["EncodedBytesSent"])
	assert.Equal(t, "0", status.StatusMetrics["RetryCount"])
	assert.Equal(t, "0s", status.StatusMetrics["RetryTimeSpent"])

	metrics.LogsProcessed.Set(5)
	metrics.LogsSent.Set(3)
	metrics.BytesSent.Set(42)
	metrics.EncodedBytesSent.Set(21)
	metrics.RetryCount.Set(42)
	metrics.RetryTimeSpent.Set(int64(time.Hour * 2))
	status = Get(false)

	assert.Equal(t, "5", status.StatusMetrics["LogsProcessed"])
	assert.Equal(t, "3", status.StatusMetrics["LogsSent"])
	assert.Equal(t, "42", status.StatusMetrics["BytesSent"])
	assert.Equal(t, "21", status.StatusMetrics["EncodedBytesSent"])
	assert.Equal(t, "42", status.StatusMetrics["RetryCount"])
	assert.Equal(t, "2h0m0s", status.StatusMetrics["RetryTimeSpent"])

	metrics.LogsProcessed.Set(math.MaxInt64)
	metrics.LogsProcessed.Add(1)
	status = Get(false)
	assert.Equal(t, fmt.Sprintf("%v", math.MinInt64), status.StatusMetrics["LogsProcessed"])
}

func TestStatusEndpoints(t *testing.T) {
	defer Clear()
	initStatus()

	status := Get(false)
	assert.Equal(t, "Reliable: Sending uncompressed logs in SSL encrypted TCP to agent-intake.logs.datadoghq.com on port 10516", status.Endpoints[0])
}
