// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package anomalydetection

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	logutils "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-log-pipelines/utils"
)

const (
	smartSeverityLogFileName = "smart-severity-profiles.log"
	smartSeverityLogFilePath = logutils.LinuxLogsFolderPath + "/" + smartSeverityLogFileName
	smartSeverityService     = "smart-severity-e2e"
	smartSeverityScorerEWMA  = "observer.scorer.ewma"
	smartSeverityScorerState = "observer.scorer.state"

	smartSeverityFakeintakeTick = 10 * time.Second
	smartSeverityLowThreshold   = 0.04
	smartSeverityStartupTicks   = 1

	// In noisy-log-detection (dry-run) mode, logs that would have been dropped
	// are still forwarded but tagged with noisy_log:true.
	smartSeverityNoisyLogTag = "noisy_log:true"
)

// smartSeverityProfilesSuite validates that smart severity profiles:
//   - activate scorer telemetry when smart severity is enabled,
//   - drive adaptive sampling via scorer transitions, and
//   - change delivered log volume as the scorer transitions Low -> Medium -> Low.
type smartSeverityProfilesSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestSmartSeverityProfilesAdaptiveSampling(t *testing.T) {
	t.Parallel()

	// language=yaml
	logConfig := `
logs:
  - type: file
    path: /var/log/e2e_test_logs/smart-severity-profiles.log
    service: smart-severity-e2e
    source: e2e
    experimental_noisy_log_detection: true
`

	// language=yaml
	agentConfig := `
log_level: debug
logs_config:
  file_scan_period: 1
  experimental_noisy_log_detection: true
  experimental_adaptive_sampling:
    enabled: false
    rate_limit: 0.01
    burst_size: 1
    protect_important_logs: false
    smart_severity_profiles:
      enabled: true
      medium:
        rate_limit: 1000
        burst_size: 200
      high:
        rate_limit: 1000
        burst_size: 200
anomaly_detection:
  metrics:
    enabled: true
    # Discard everything except our own test metric, so the EWMA is only ever
    # driven by the spikes/baselines this test sends, never by unrelated
    # DogStatsD traffic. Rules are evaluated in order, first match wins.
    processing_rules:
      - type: include_at_match
        name: keep_smartseverity_metric
        name_pattern: "e2e.anomalydetection.smartseverity.s*"
      - type: exclude_at_match
        name: drop_everything_else
  logs:
    enabled: false
    internal:
      enabled: false
  anomaly_scorer:
    alpha: 0.3
    window: 5s
    low_threshold: 0.04
    high_threshold: 0.50
    margin_pct: 0.1
    output:
      cooldown: 2s
  detectors:
    cusum:
      enabled: true
    bocpd:
      enabled: false
    holt_residual:
      enabled: false
    tukey_biweight:
      enabled: false
` + baselineAnalysisDisabledYAML

	e2e.Run(t, &smartSeverityProfilesSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(scenec2.WithAgentOptions(
				agentparams.WithLogs(),
				agentparams.WithTelemetry(),
				agentparams.WithAgentConfig(agentConfig),
				agentparams.WithIntegration("custom_logs.d", logConfig),
			)),
		),
	), e2e.WithStackName("anomalydetection-smart-severity-profiles"))
}

func (s *smartSeverityProfilesSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)
	logutils.CleanUp(s)
	s.Env().RemoteHost.MustExecute("sudo mkdir -p " + logutils.LinuxLogsFolderPath)
	s.Env().RemoteHost.MustExecute("sudo rm -f " + smartSeverityLogFilePath)
	s.Env().RemoteHost.MustExecute("sudo touch " + smartSeverityLogFilePath)
	s.Env().RemoteHost.MustExecute("sudo chmod 644 " + smartSeverityLogFilePath)
}

func (s *smartSeverityProfilesSuite) TearDownSuite() {
	logutils.CleanUp(s)
	s.BaseSuite.TearDownSuite()
}

func (s *smartSeverityProfilesSuite) TestSmartSeverityProfilesDriveAdaptiveSampling() {
	const (
		seriesCount   = 8
		metricPrefix  = "e2e.anomalydetection.smartseverity.s"
		baseline      = 1.0
		spike         = 5000.0
		baselineTicks = 8
		spikeTicks    = 5
		logBurst      = 12
	)

	lowMessage := "smart severity low phase"
	mediumMessage := "smart severity medium phase"

	// 1. Init (low)
	s.T().Log("starting smart severity adaptive sampling test")
	logutils.AssertAgentTailerOK(s, smartSeverityLogFileName)
	s.T().Log("waiting for scorer registration, smart severity profiles must enable anomaly detection + the scorer")
	waitForScorerHelperReady(s)
	s.waitForStartupLowProfile(metricPrefix, seriesCount, baseline)

	s.T().Logf("phase=low-log-burst message=%q count=%d", lowMessage, logBurst)
	s.appendRepeatedLog(lowMessage, logBurst)
	lowLogs := s.waitForLogsDelivered(lowMessage, logBurst)
	lowCount := len(lowLogs)
	lowNoisy := countLogsWithTag(lowLogs, smartSeverityNoisyLogTag)
	s.T().Logf("phase=low-log-burst delivered=%d noisy=%d", lowCount, lowNoisy)
	require.Greater(s.T(), lowNoisy, 0, "expected low-phase logs to be tagged noisy in dry-run mode")

	// 2. Anomaly (medium/high)
	s.T().Log("triggering anomaly")
	s.T().Logf("phase=baseline metrics prefix=%s series=%d ticks=%d value=%.1f", metricPrefix, seriesCount, baselineTicks, baseline)
	s.sendMetricTicks(metricPrefix, seriesCount, baseline, baselineTicks)
	s.T().Logf("phase=spike metrics prefix=%s series=%d ticks=%d value=%.1f", metricPrefix, seriesCount, spikeTicks, spike)
	s.sendMetricTicks(metricPrefix, seriesCount, spike, spikeTicks)
	s.T().Log("waiting for scorer escalation state")
	s.waitForScorerState("direction:escalation")

	s.T().Logf("phase=medium-log-burst message=%q count=%d", mediumMessage, logBurst)
	s.appendRepeatedLog(mediumMessage, logBurst)
	mediumLogs := s.waitForLogsDelivered(mediumMessage, logBurst)
	mediumCount := len(mediumLogs)
	mediumNoisy := countLogsWithTag(mediumLogs, smartSeverityNoisyLogTag)
	s.T().Logf("phase=medium-log-burst delivered=%d noisy=%d", mediumCount, mediumNoisy)
	require.Zero(s.T(), mediumNoisy, "expected medium-phase logs to avoid noisy tagging in dry-run mode")

	s.T().Logf(
		"adaptive sampling dry-run delivered low=%d medium=%d logs; noisy low=%d medium=%d",
		lowCount,
		mediumCount,
		lowNoisy,
		mediumNoisy,
	)

	require.Greater(s.T(), lowNoisy, mediumNoisy, "low severity should tag more logs noisy than medium severity in dry-run mode")
}

func (s *smartSeverityProfilesSuite) sendMetricTicks(metricPrefix string, seriesCount int, value float64, ticks int) {
	s.T().Helper()
	s.T().Logf("sending metric ticks prefix=%s series=%d ticks=%d value=%.1f", metricPrefix, seriesCount, ticks, value)
	for tick := 0; tick < ticks; tick++ {
		for series := 0; series < seriesCount; series++ {
			sendGauge(s, fmt.Sprintf("%s%d", metricPrefix, series), value)
		}
		s.T().Logf("sent metric tick %d/%d value=%.1f", tick+1, ticks, value)
		time.Sleep(time.Second)
	}
	s.T().Logf("finished sending metric ticks prefix=%s value=%.1f", metricPrefix, value)
}

func (s *smartSeverityProfilesSuite) appendRepeatedLog(message string, count int) {
	s.T().Helper()
	s.T().Logf("appending repeated logs message=%q count=%d", message, count)
	logutils.AppendLog(s, smartSeverityLogFileName, message, count)
}

// waitForStartupLowProfile nudges the scorer with stable baseline input and waits
// for the EWMA to settle back under the configured low threshold before the first
// low-phase log burst. This avoids assuming the scorer starts clean when a reused
// dev-mode agent may still carry some prior non-zero EWMA state.
func (s *smartSeverityProfilesSuite) waitForStartupLowProfile(metricPrefix string, seriesCount int, baseline float64) {
	s.T().Helper()
	s.T().Logf(
		"settling scorer toward low profile with %d baseline ticks before low-phase logs",
		smartSeverityStartupTicks,
	)
	// Wake up the scorer with some data
	s.sendMetricTicks(metricPrefix, seriesCount, baseline, smartSeverityStartupTicks)
	s.EventuallyWithT(func(c *assert.CollectT) {
		tel := observerTelemetryOutput(s)
		line := findMetricLine(tel, metricNameVariants(smartSeverityScorerEWMA)...)
		s.T().Logf("startup low-profile probe ewma line=%q", line)
		require.NotEmpty(c, line, "expected scorer ewma telemetry while settling startup state")
		ewma, ok := scorerStateValue(line)
		require.True(c, ok, "could not parse scorer ewma value from line %q", line)
		require.LessOrEqual(c, ewma, smartSeverityLowThreshold,
			"expected startup ewma %.6f to be at or below low threshold %.2f before low-phase logs",
			ewma, smartSeverityLowThreshold,
		)
	}, 2*time.Minute, 3*time.Second)
}

func (s *smartSeverityProfilesSuite) waitForScorerState(directionTag string) {
	s.T().Helper()
	s.EventuallyWithT(func(c *assert.CollectT) {
		tel := observerTelemetryOutput(s)
		tagParts := strings.SplitN(directionTag, ":", 2)
		require.Len(s.T(), tagParts, 2, "direction tag must be key:value")
		s.T().Logf(
			"poll scorer state telemetry: present=%t wanted_tag=%q tagged=%t",
			containsMetric(tel, smartSeverityScorerState),
			directionTag,
			containsMetricWithTag(tel, smartSeverityScorerState, tagParts[0], tagParts[1]),
		)
		line := findMetricLine(tel, metricNameVariants(smartSeverityScorerState)...)
		if line != "" {
			s.T().Logf("sample scorer state line=%s", line)
		}
		require.True(c, containsMetric(tel, smartSeverityScorerState), "expected scorer state telemetry")
		require.True(c, containsMetricWithTag(tel, smartSeverityScorerState, tagParts[0], tagParts[1]),
			"expected scorer state telemetry tagged with %s", directionTag)
	}, 2*time.Minute, smartSeverityFakeintakeTick)
}

// waitForLogsDelivered polls fakeintake until at least wantCount logs matching
// message have been delivered, then returns them. Fetches are cumulative
// (fakeintake never drops previously-seen payloads), so the count only grows;
// waiting for ">=" rather than "==" avoids getting stuck if delivery
// occasionally over-delivers (e.g. an agent-side retry redelivering a line).
func (s *smartSeverityProfilesSuite) waitForLogsDelivered(message string, wantCount int) []*aggregator.Log {
	s.T().Helper()
	var delivered []*aggregator.Log
	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := logutils.FetchAndFilterLogs(s.Env().FakeIntake, smartSeverityService, message)
		require.NoError(c, err, "failed to fetch logs for %q", message)
		noisy := countLogsWithTag(logs, smartSeverityNoisyLogTag)
		s.T().Logf("poll logs message=%q delivered=%d/%d noisy=%d err=%v", message, len(logs), wantCount, noisy, err)
		require.GreaterOrEqual(c, len(logs), wantCount, "expected at least %d delivered logs for %q", wantCount, message)
		delivered = logs
	}, 1*time.Minute, smartSeverityFakeintakeTick)
	return delivered
}

func countLogsWithTag(logs []*aggregator.Log, want string) int {
	count := 0
	for _, log := range logs {
		for _, tag := range log.GetTags() {
			if tag == want {
				count++
				break
			}
		}
	}
	return count
}

func findMetricLine(output string, prefixes ...string) string {
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		for _, prefix := range prefixes {
			if strings.HasPrefix(trimmed, prefix) {
				return trimmed
			}
		}
	}
	return ""
}

func scorerStateValue(line string) (float64, bool) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return 0, false
	}
	value, err := strconv.ParseFloat(fields[len(fields)-1], 64)
	return value, err == nil
}
