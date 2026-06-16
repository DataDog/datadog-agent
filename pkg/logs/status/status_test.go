// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

package status

import (
	"expvar"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
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

func TestStatusPerformanceProfileOffByDefault(t *testing.T) {
	defer Clear()
	initStatus(t)

	status := Get(false)
	assert.Nil(t, status.PerformanceProfile, "no profile section when profiles are off")
}

func TestStatusPerformanceProfile(t *testing.T) {
	defer Clear()

	mockConfig := configmock.New(t)
	mockConfig.Set("logs_config.profile", "high-throughput", model.SourceFile)
	InitStatus(mockConfig, testutils.CreateSources([]*sources.LogSource{
		sources.NewLogSource("foo", &config.LogsConfig{Type: "foo"}),
	}))

	status := Get(false)

	require.NotNil(t, status.PerformanceProfile)
	assert.Equal(t, "high-throughput", status.PerformanceProfile.Name)
	assert.Equal(t, 1, status.PerformanceProfile.Version)
	require.NotEmpty(t, status.PerformanceProfile.Settings)

	var foundPipelines bool
	for _, s := range status.PerformanceProfile.Settings {
		assert.NotEmpty(t, s.Key)
		assert.NotEmpty(t, s.Source, "each touched setting must report its config source")
		if s.Key == "logs_config.pipelines" {
			foundPipelines = true
		}
	}
	assert.True(t, foundPipelines, "high-throughput must report the pipelines setting it touches")
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

	assert.Equal(t, "5", status.StatusMetrics["LogsProcessed"])
	assert.Equal(t, "3", status.StatusMetrics["LogsSent"])
	assert.Equal(t, "42", status.StatusMetrics["BytesSent"])
	assert.Equal(t, "21", status.StatusMetrics["EncodedBytesSent"])
	assert.Equal(t, "42", status.StatusMetrics["RetryCount"])
	assert.Equal(t, "2h0m0s", status.StatusMetrics["RetryTimeSpent"])
	assert.Equal(t, "64", status.StatusMetrics["LogsTruncated"])

	metrics.LogsProcessed.Set(math.MaxInt64)
	metrics.LogsProcessed.Add(1)
	status = Get(false)
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

// getProfileRecommendation signature: (utils, activeProfile, latencyMs, dropped, missed, delivering).
// Loss (dropped or missed > 0) is the gate; saturation only localizes the bottleneck.

func TestProfileRecommendation_ProcessorBottleneckIsCPUBound(t *testing.T) {
	b := &Builder{}
	// Read-side loss is occurring; processor saturated, downstream keeping up: CPU-bound.
	utils := []ComponentUtilization{
		{Name: "processor", Instance: "0", AvgRatio: 0.97, CurrentlySaturated: true, Saturated30mSeconds: 120},
		{Name: "strategy", Instance: "0", AvgRatio: 0.20},
		{Name: "worker", Instance: "0", AvgRatio: 0.15},
	}

	rec := b.getProfileRecommendation(utils, "", 0, 0, 1, true)

	require.NotNil(t, rec)
	assert.Equal(t, "high-throughput", rec.Profile)
	assert.True(t, pkgconfigsetup.LogsPerformanceProfileExists(rec.Profile))
	assert.Contains(t, rec.Reason, "processor")
}

func TestProfileRecommendation_DownstreamBottleneckIsNetworkBound(t *testing.T) {
	b := &Builder{}
	// Destination saturated; upstream also lights up from propagation. The
	// most-downstream saturated stage (destination) is the true bottleneck.
	utils := []ComponentUtilization{
		{Name: "processor", Instance: "0", AvgRatio: 0.92, CurrentlySaturated: true, Saturated30mSeconds: 100},
		{Name: "strategy", Instance: "0", AvgRatio: 0.93, CurrentlySaturated: true, Saturated30mSeconds: 100},
		{Name: "worker", Instance: "0", AvgRatio: 0.95, CurrentlySaturated: true, Saturated30mSeconds: 110},
		{Name: "destination_reliable_0", Instance: "q0s0", AvgRatio: 0.98, CurrentlySaturated: true, Saturated30mSeconds: 120},
	}

	rec := b.getProfileRecommendation(utils, "", 0, 0, 1, true)

	require.NotNil(t, rec)
	assert.Equal(t, "high-concurrency", rec.Profile, "downstream bottleneck must map to high-concurrency, not high-throughput")
	assert.True(t, pkgconfigsetup.LogsPerformanceProfileExists(rec.Profile))
	assert.NotEmpty(t, rec.Reason)
}

func TestProfileRecommendation_DownstreamHighLatencyCitesLatency(t *testing.T) {
	b := &Builder{}
	utils := []ComponentUtilization{
		{Name: "destination_reliable_0", Instance: "q0s0", AvgRatio: 0.97, CurrentlySaturated: true, Saturated30mSeconds: 120},
	}

	// High intake latency: the recommendation should cite it.
	rec := b.getProfileRecommendation(utils, "", 400, 0, 1, true)

	require.NotNil(t, rec)
	assert.Equal(t, "high-concurrency", rec.Profile)
	assert.Contains(t, rec.Reason, "latency")
	assert.Contains(t, rec.Reason, "400")
}

func TestProfileRecommendation_DownstreamLowLatencyNoLatencyMention(t *testing.T) {
	b := &Builder{}
	utils := []ComponentUtilization{
		{Name: "destination_reliable_0", Instance: "q0s0", AvgRatio: 0.97, CurrentlySaturated: true, Saturated30mSeconds: 120},
	}

	// Normal latency: still high-concurrency, but no latency claim in the reason.
	rec := b.getProfileRecommendation(utils, "", 10, 0, 1, true)

	require.NotNil(t, rec)
	assert.Equal(t, "high-concurrency", rec.Profile)
	assert.NotContains(t, rec.Reason, "latency")
}

func TestProfileRecommendation_StrategyBottleneckIsCPUBound(t *testing.T) {
	b := &Builder{}
	// Strategy and processor saturated (propagation), worker keeping up:
	// compression/batching is the bottleneck.
	utils := []ComponentUtilization{
		{Name: "processor", Instance: "0", AvgRatio: 0.93, CurrentlySaturated: true, Saturated30mSeconds: 100},
		{Name: "strategy", Instance: "0", AvgRatio: 0.96, CurrentlySaturated: true, Saturated30mSeconds: 110},
		{Name: "worker", Instance: "0", AvgRatio: 0.20},
	}

	rec := b.getProfileRecommendation(utils, "", 0, 0, 1, true)

	require.NotNil(t, rec)
	assert.Equal(t, "high-throughput", rec.Profile)
	assert.Contains(t, rec.Reason, "compression")
}

func TestProfileRecommendation_LossStatedInReason(t *testing.T) {
	b := &Builder{}
	utils := []ComponentUtilization{
		{Name: "processor", Instance: "0", AvgRatio: 0.97, CurrentlySaturated: true, Saturated30mSeconds: 120},
	}

	rec := b.getProfileRecommendation(utils, "", 0, 0, 1, true)

	require.NotNil(t, rec)
	assert.Contains(t, rec.Reason, "lost", "reason should make clear logs are actually being lost")
}

func TestProfileRecommendation_NoLossIsSilent(t *testing.T) {
	b := &Builder{}
	// Send stage saturated, but no logs lost: saturation alone must NOT recommend.
	utils := []ComponentUtilization{
		{Name: "strategy", Instance: "0", AvgRatio: 0.96, CurrentlySaturated: true, Saturated30mSeconds: 110},
		{Name: "worker", Instance: "q0s0", AvgRatio: 0.97, CurrentlySaturated: true, Saturated30mSeconds: 110},
	}

	assert.Nil(t, b.getProfileRecommendation(utils, "", 0, 0, 0, true))
}

func TestProfileRecommendation_MissedButNothingSaturatedIsSilent(t *testing.T) {
	b := &Builder{}
	// Bytes missed (rotation outran an idle reader) but no stage is saturated:
	// the fix is close_timeout, not a performance profile.
	utils := []ComponentUtilization{
		{Name: "processor", Instance: "0", AvgRatio: 0.10},
		{Name: "worker", Instance: "q0s0", AvgRatio: 0.05},
	}

	assert.Nil(t, b.getProfileRecommendation(utils, "", 0, 0, 1, true))
}

func TestProfileRecommendation_DroppedWithSendStageSaturatedRecommends(t *testing.T) {
	b := &Builder{}
	utils := []ComponentUtilization{
		{Name: "worker", Instance: "q0s0", AvgRatio: 0.97, CurrentlySaturated: true, Saturated30mSeconds: 110},
	}

	rec := b.getProfileRecommendation(utils, "", 0, 5, 0, true)

	require.NotNil(t, rec)
	assert.Equal(t, "high-concurrency", rec.Profile)
}

func TestProfileRecommendation_DroppedWithNoDownstreamSaturationIsSilent(t *testing.T) {
	b := &Builder{}
	// Logs dropped but the send stage is not saturated: permanent send errors
	// (e.g. 4xx/auth), which no profile fixes.
	utils := []ComponentUtilization{
		{Name: "processor", Instance: "0", AvgRatio: 0.95, CurrentlySaturated: true, Saturated30mSeconds: 100},
	}

	assert.Nil(t, b.getProfileRecommendation(utils, "", 0, 5, 0, true))
}

func TestProfileRecommendation_SendStageNotDeliveringIsSilent(t *testing.T) {
	b := &Builder{}
	// Loss is occurring and the send stage is saturated, but the intake is not
	// delivering (rejecting/unreachable): high-concurrency would be misleading.
	utils := []ComponentUtilization{
		{Name: "strategy", Instance: "0", AvgRatio: 0.96, CurrentlySaturated: true, Saturated30mSeconds: 110},
		{Name: "worker", Instance: "q0s0", AvgRatio: 0.97, CurrentlySaturated: true, Saturated30mSeconds: 110},
	}

	assert.Nil(t, b.getProfileRecommendation(utils, "", 0, 0, 1, false))
	assert.Nil(t, b.getProfileRecommendation(utils, "", 0, 5, 0, false))
}

func TestProfileRecommendation_AlreadyOnRecommendedProfile(t *testing.T) {
	b := &Builder{}
	utils := []ComponentUtilization{
		{Name: "destination_reliable_0", Instance: "q0s0", AvgRatio: 0.97, CurrentlySaturated: true, Saturated30mSeconds: 120},
	}

	// Already running the profile we'd recommend (high-concurrency for a
	// downstream bottleneck): no point recommending it again.
	assert.Nil(t, b.getProfileRecommendation(utils, "high-concurrency", 0, 0, 1, true))
}

func TestProfileRecommendation_RecentSaturationLocalizes(t *testing.T) {
	b := &Builder{}
	// No stage is currently saturated, but strategy was saturated recently and
	// loss occurred: fall back to recent saturation to localize the bottleneck.
	utils := []ComponentUtilization{
		{Name: "strategy", Instance: "0", AvgRatio: 0.6, Saturated1mSeconds: 30, Saturated30mSeconds: 90},
	}

	rec := b.getProfileRecommendation(utils, "", 0, 0, 1, true)
	require.NotNil(t, rec)
	assert.Equal(t, "high-throughput", rec.Profile)
}

func TestProfileRecommendation_ActiveProfileCoversRecommendedIsSilent(t *testing.T) {
	b := &Builder{}
	// A send-stage bottleneck maps to high-concurrency, but the agent is already
	// on high-throughput, which is a superset of high-concurrency. Switching would
	// only lower pipeline/buffer settings, so suppress the recommendation.
	utils := []ComponentUtilization{
		{Name: "worker", Instance: "q0s0", AvgRatio: 0.97, CurrentlySaturated: true, Saturated30mSeconds: 110},
	}

	assert.Nil(t, b.getProfileRecommendation(utils, "high-throughput", 0, 0, 1, true),
		"must not recommend a profile the active one already covers")
}

func TestDestinationDelivering(t *testing.T) {
	defer Clear()
	b := &Builder{logsExpVars: metrics.LogsExpvars}

	metrics.LogsProcessed.Set(0)
	metrics.LogsSent.Set(0)
	defer func() {
		metrics.LogsProcessed.Set(0)
		metrics.LogsSent.Set(0)
	}()

	// Nothing processed yet: treat as delivering (no evidence of a problem).
	assert.True(t, b.destinationDelivering())

	// Processed but nothing sent: intake is rejecting/unreachable.
	metrics.LogsProcessed.Set(100)
	metrics.LogsSent.Set(0)
	assert.False(t, b.destinationDelivering())

	// Some logs are getting through: delivering.
	metrics.LogsSent.Set(1)
	assert.True(t, b.destinationDelivering())
}

func TestStatusMetricsIncludesLoss(t *testing.T) {
	defer Clear()
	initStatus(t)

	defer func() {
		metrics.BytesMissed.Set(0)
		metrics.DestinationLogsDropped.Init()
	}()

	metrics.BytesMissed.Set(4096)
	dropped := &expvar.Int{}
	dropped.Set(7)
	metrics.DestinationLogsDropped.Set("host-a", dropped)
	dropped2 := &expvar.Int{}
	dropped2.Set(3)
	metrics.DestinationLogsDropped.Set("host-b", dropped2)

	status := Get(false)
	assert.Equal(t, "4096", status.StatusMetrics["BytesMissed"])
	assert.Equal(t, "10", status.StatusMetrics["LogsDropped"], "LogsDropped must sum drops across all destinations")
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
