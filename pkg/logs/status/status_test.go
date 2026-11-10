// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

package status

import (
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/util/testutils"
)

func initStatus(t *testing.T) {
	mockConfig := configmock.New(t)
	InitStatus(mockConfig, testutils.CreateSources([]*sources.LogSource{
		sources.NewLogSource("foo", &config.LogsConfig{Type: "foo"}),
		sources.NewLogSource("bar", &config.LogsConfig{Type: "foo"}),
		sources.NewLogSource("foo", &config.LogsConfig{Type: "foo"}),
	}))
}

func TestSourceAreGroupedByIntegrations(t *testing.T) {
	defer Clear()
	initStatus(t)

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
			assert.Fail(t, "Expected foo or bar, got "+integration.Name)
		}
	}
}

func TestIntegrationsRespectVerbose(t *testing.T) {
	defer Clear()

	logSource := sources.NewLogSource("foo", &config.LogsConfig{
		Type:        config.FileType,
		Path:        "/var/log/foo.log",
		Identifier:  "foo-id",
		Service:     "foo-service",
		Source:      "foo-source",
		TailingMode: "beginning",
		Format:      "json",
	})
	logSource.Status().Success()
	logSource.AddInput("/var/log/foo.log")
	logSource.Messages.AddMessage("warning", "source warning")
	logSource.RecordBytes(42)

	mockConfig := configmock.New(t)
	InitStatus(mockConfig, testutils.CreateSources([]*sources.LogSource{logSource}))

	status := Get(false)
	require.Len(t, status.Integrations, 1)
	require.Len(t, status.Integrations[0].Sources, 1)
	sourceStatus := status.Integrations[0].Sources[0]
	assert.Equal(t, map[string]interface{}{
		"Bytes Read": int64(42),
		"Path":       "/var/log/foo.log",
		"Service":    "foo-service",
		"Source":     "foo-source",
	}, sourceStatus.Configuration)
	assert.Equal(t, "OK", sourceStatus.Status)
	assert.Equal(t, []string{"source warning"}, sourceStatus.Messages)
	assert.Nil(t, sourceStatus.Inputs)
	assert.Nil(t, sourceStatus.Info)

	verboseStatus := Get(true)
	require.Len(t, verboseStatus.Integrations, 1)
	require.Len(t, verboseStatus.Integrations[0].Sources, 1)
	verboseSourceStatus := verboseStatus.Integrations[0].Sources[0]
	assert.Equal(t, map[string]interface{}{
		"Bytes Read":  int64(42),
		"Format":      "json",
		"Identifier":  "foo-id",
		"Path":        "/var/log/foo.log",
		"Service":     "foo-service",
		"Source":      "foo-source",
		"TailingMode": "beginning",
	}, verboseSourceStatus.Configuration)
	assert.Equal(t, []string{"/var/log/foo.log"}, verboseSourceStatus.Inputs)
	assert.NotContains(t, verboseSourceStatus.Info, "Bytes Read")
	assert.Contains(t, verboseSourceStatus.Info, "Processing Rules")
}

func TestNonFileConfigurationIsAvailableWithoutVerbose(t *testing.T) {
	logSource := sources.NewLogSource("tcp", &config.LogsConfig{
		Type:       config.TCPType,
		Port:       10518,
		Format:     "syslog",
		AllowedIPs: config.StringSliceField{"10.0.0.0/8"},
		DeniedIPs:  config.StringSliceField{"10.0.0.1"},
		Service:    "tcp-service",
		Source:     "tcp-source",
	})

	builder := &Builder{}
	assert.Equal(t, map[string]interface{}{
		"AllowedIPs": "10.0.0.0/8",
		"Bytes Read": int64(0),
		"DeniedIPs":  "10.0.0.1",
		"Format":     "syslog",
		"Port":       10518,
		"Service":    "tcp-service",
		"Source":     "tcp-source",
	}, builder.configToDictionary(logSource, false))
}

func TestStatusDeduplicateWarnings(t *testing.T) {
	defer Clear()
	initStatus(t)

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
	initStatus(t)

	AddGlobalError("bar", "Unique Error")
	AddGlobalError("foo", "Identical Error")
	AddGlobalError("foo", "Identical Error")

	status := Get(false)
	assert.ElementsMatch(t, []string{"Identical Error", "Unique Error"}, status.Errors)
}

func TestStatusDeduplicateErrorsAndWarnings(t *testing.T) {
	defer Clear()
	initStatus(t)

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
	var expected = `{"BytesMissed": 0, "BytesSent": 0, "DestinationErrors": 0, "DestinationLogsDropped": {}, "EncodedBytesSent": 0, "Errors": "", "HttpDestinationStats": {}, "IsRunning": false, "LogsDecoded": 0, "LogsProcessed": 0, "LogsSent": 0, "LogsTruncated": 0, "RetryCount": 0, "RetryTimeSpent": 0, "SenderLatency": 0, "Warnings": ""}`
	assert.Equal(t, expected, metrics.LogsExpvars.String())

	initStatus(t)
	AddGlobalWarning("bar", "Unique Warning")
	AddGlobalError("bar", "I am an error")
	expected = `{"BytesMissed": 0, "BytesSent": 0, "DestinationErrors": 0, "DestinationLogsDropped": {}, "EncodedBytesSent": 0, "Errors": "I am an error", "HttpDestinationStats": {}, "IsRunning": true, "LogsDecoded": 0, "LogsProcessed": 0, "LogsSent": 0, "LogsTruncated": 0, "RetryCount": 0, "RetryTimeSpent": 0, "SenderLatency": 0, "Warnings": "Unique Warning"}`
	assert.Equal(t, expected, metrics.LogsExpvars.String())
}

func TestStatusMetrics(t *testing.T) {
	defer Clear()
	initStatus(t)

	status := Get(false)
	assert.Len(t, status.StatusMetrics, 4)
	assert.Equal(t, "0", status.StatusMetrics["LogsProcessed"])
	assert.Equal(t, "0", status.StatusMetrics["LogsSent"])
	assert.Equal(t, "0", status.StatusMetrics["BytesSent"])
	assert.Equal(t, "0", status.StatusMetrics["RetryCount"])
	assert.NotContains(t, status.StatusMetrics, "EncodedBytesSent")
	assert.NotContains(t, status.StatusMetrics, "RetryTimeSpent")
	assert.NotContains(t, status.StatusMetrics, "LogsTruncated")
	assert.Empty(t, status.ProcessFileStats)

	status = Get(true)
	assert.Equal(t, "0", status.StatusMetrics["LogsProcessed"])
	assert.Equal(t, "0", status.StatusMetrics["LogsSent"])
	assert.Equal(t, "0", status.StatusMetrics["BytesSent"])
	assert.Equal(t, "0", status.StatusMetrics["EncodedBytesSent"])
	assert.Equal(t, "0", status.StatusMetrics["RetryCount"])
	assert.Equal(t, "0s", status.StatusMetrics["RetryTimeSpent"])
	assert.Equal(t, "0", status.StatusMetrics["LogsTruncated"])

	metrics.LogsProcessed.Set(5)
	metrics.LogsSent.Set(3)
	metrics.BytesSent.Set(42)
	metrics.EncodedBytesSent.Set(21)
	metrics.RetryCount.Set(42)
	metrics.RetryTimeSpent.Set(int64(time.Hour * 2))
	metrics.LogsTruncated.Set(64)
	status = Get(false)
	assert.Len(t, status.StatusMetrics, 4)
	assert.Equal(t, "5", status.StatusMetrics["LogsProcessed"])
	assert.Equal(t, "3", status.StatusMetrics["LogsSent"])
	assert.Equal(t, "42", status.StatusMetrics["BytesSent"])
	assert.Equal(t, "42", status.StatusMetrics["RetryCount"])

	status = Get(true)

	assert.Equal(t, "5", status.StatusMetrics["LogsProcessed"])
	assert.Equal(t, "3", status.StatusMetrics["LogsSent"])
	assert.Equal(t, "42", status.StatusMetrics["BytesSent"])
	assert.Equal(t, "21", status.StatusMetrics["EncodedBytesSent"])
	assert.Equal(t, "42", status.StatusMetrics["RetryCount"])
	assert.Equal(t, "2h0m0s", status.StatusMetrics["RetryTimeSpent"])
	assert.Equal(t, "64", status.StatusMetrics["LogsTruncated"])

	metrics.LogsProcessed.Set(math.MaxInt64)
	metrics.LogsProcessed.Add(1)
	status = Get(true)
	assert.Equal(t, strconv.Itoa(math.MinInt64), status.StatusMetrics["LogsProcessed"])
}

func TestStatusEndpoints(t *testing.T) {
	defer Clear()
	initStatus(t)

	status := Get(false)
	assert.Equal(t, "Reliable: Sending uncompressed logs in SSL encrypted TCP to agent-intake.logs.datadoghq.com. on port 10516 (API Key: ********)", status.Endpoints[0])
}

// Tests for getBackpressureStatus, called directly with a crafted utilization slice (no agent infra).

func TestGetBackpressureStatus_Healthy(t *testing.T) {
	b := &Builder{}
	utils := []ComponentUtilization{
		{Name: "processor", Instance: "0", AvgRatio: 0.5},
	}
	bp := b.getBackpressureStatus(utils)
	assert.Equal(t, "HEALTHY", bp.State)
	assert.Empty(t, bp.Reason)
}

func TestFormatBackpressureSectionRespectsVerbose(t *testing.T) {
	b := &Builder{}
	utils := []ComponentUtilization{{Name: "processor", Instance: "0"}}

	healthy := BackpressureStatus{State: "HEALTHY"}
	assert.Empty(t, b.formatBackpressureSection(utils, healthy, false))
	assert.Contains(t, b.formatBackpressureSection(utils, healthy, true), "Component")

	warning := BackpressureStatus{State: "WARNING", Reason: "processor pipeline 0 was saturated"}
	section := b.formatBackpressureSection(utils, warning, false)
	assert.Contains(t, section, "Overall state: WARNING")
	assert.Contains(t, section, warning.Reason)
	assert.NotContains(t, section, "Component")
}

func TestGetBackpressureStatus_Saturated(t *testing.T) {
	b := &Builder{}
	utils := []ComponentUtilization{
		{Name: "processor", Instance: "0", AvgRatio: 0.95, CurrentlySaturated: true, Saturated30mSeconds: 120},
	}
	bp := b.getBackpressureStatus(utils)
	assert.Equal(t, "SATURATED", bp.State)
	assert.Contains(t, bp.Reason, "processor")
	assert.Contains(t, bp.Reason, "2m0s") // 120s formatted
}

// TestGetBackpressureStatus_FrozenRatioNotSaturated checks a frozen-high AvgRatio with stale saturation reads WARNING, not SATURATED.
func TestGetBackpressureStatus_FrozenRatioNotSaturated(t *testing.T) {
	b := &Builder{}
	utils := []ComponentUtilization{
		{Name: "processor", Instance: "0", AvgRatio: 0.95, CurrentlySaturated: false, Saturated30mSeconds: 120},
	}
	bp := b.getBackpressureStatus(utils)
	assert.NotEqual(t, "SATURATED", bp.State, "a frozen high AvgRatio must not read as live saturation")
	assert.Equal(t, "WARNING", bp.State)
}

// TestGetBackpressureStatus_FrozenRatioFullyIdleHealthy checks a frozen-high EWMA with no recent saturation reads HEALTHY.
func TestGetBackpressureStatus_FrozenRatioFullyIdleHealthy(t *testing.T) {
	b := &Builder{}
	utils := []ComponentUtilization{
		{Name: "processor", Instance: "0", AvgRatio: 0.95, CurrentlySaturated: false},
	}
	bp := b.getBackpressureStatus(utils)
	assert.Equal(t, "HEALTHY", bp.State)
	assert.Empty(t, bp.Reason)
}

func TestGetBackpressureStatus_WarningSat1m(t *testing.T) {
	b := &Builder{}
	utils := []ComponentUtilization{
		{Name: "sender", Instance: "1", AvgRatio: 0.7, Saturated1mSeconds: 30, Saturated30mSeconds: 90},
	}
	bp := b.getBackpressureStatus(utils)
	assert.Equal(t, "WARNING", bp.State)
	assert.Contains(t, bp.Reason, "sender")
	assert.Contains(t, bp.Reason, "1m30s") // 90s
}

func TestGetBackpressureStatus_WarningSat30mOnly(t *testing.T) {
	b := &Builder{}
	utils := []ComponentUtilization{
		{Name: "worker", Instance: "2", AvgRatio: 0.5, Saturated1mSeconds: 0, Saturated30mSeconds: 45},
	}
	bp := b.getBackpressureStatus(utils)
	assert.Equal(t, "WARNING", bp.State)
	assert.Contains(t, bp.Reason, "worker")
	assert.Contains(t, bp.Reason, "45s")
}

func TestGetBackpressureStatus_SaturatedPicksHighestRatio(t *testing.T) {
	b := &Builder{}
	utils := []ComponentUtilization{
		{Name: "processor", Instance: "0", AvgRatio: 0.85, CurrentlySaturated: true, Saturated30mSeconds: 10},
		{Name: "sender", Instance: "1", AvgRatio: 0.98, CurrentlySaturated: true, Saturated30mSeconds: 60},
	}
	bp := b.getBackpressureStatus(utils)
	assert.Equal(t, "SATURATED", bp.State)
	assert.Contains(t, bp.Reason, "sender", "highest AvgRatio component must appear in reason")
}

func TestGetBackpressureStatus_WarningPicksHighestSat1m(t *testing.T) {
	b := &Builder{}
	utils := []ComponentUtilization{
		{Name: "processor", Instance: "0", AvgRatio: 0.3, Saturated1mSeconds: 10, Saturated30mSeconds: 20},
		{Name: "sender", Instance: "1", AvgRatio: 0.5, Saturated1mSeconds: 55, Saturated30mSeconds: 120},
	}
	bp := b.getBackpressureStatus(utils)
	assert.Equal(t, "WARNING", bp.State)
	assert.Contains(t, bp.Reason, "sender", "component with highest Saturated1mSeconds must appear in reason")
}
