// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package anomalydetection

import (
	"fmt"
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
	smartSeverityScorerEWMA  = "datadog.agent.observer.observer_scorer_ewma"
	smartSeverityScorerState = "datadog.agent.observer.observer_scorer_state"

	smartSeverityFakeintakeTick = 10 * time.Second
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
    adaptive_sampling:
      enabled: true
`

	// language=yaml
	agentConfig := `
log_level: debug
logs_config:
  file_scan_period: 1
  experimental_adaptive_sampling:
    enabled: true
    rate_limit: 0.01
    burst_size: 1
    protect_important_logs: false
    smart_severity_profiles:
      enabled: true
      medium:
        rate_limit: 100
        burst_size: 20
      high:
        rate_limit: 100
        burst_size: 20
anomaly_detection:
  metrics:
    enabled: true
  logs:
    enabled: false
    internal:
      enabled: false
  anomaly_scorer:
    enabled: false
    alpha: 0.3
    window_secs: 5
    low_threshold: 0.08
    high_threshold: 0.30
    margin_pct: 0.1
    output:
      cooldown_secs: 2
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
		seriesCount    = 20
		metricPrefix   = "e2e.anomalydetection.smartseverity.s"
		baseline       = 1.0
		spike          = 5000.0
		baselineTicks  = 15
		spikeTicks     = 10
		recoveryTicks  = 15
		logBurst       = 12
		lowPhaseMax    = 2
		mediumPhaseMin = 10
	)

	lowMessage := "smart severity low phase"
	mediumMessage := "smart severity medium phase"
	lowRecoveryMessage := "smart severity low recovery phase"

	logutils.AssertAgentTailerOK(s, smartSeverityLogFileName)
	waitForObserverReady(s)

	s.sendMetricTicks(metricPrefix, seriesCount, baseline, baselineTicks)

	s.appendRepeatedLog(lowMessage, logBurst)
	lowCount := s.waitForLogCountAtMost(lowMessage, lowPhaseMax)

	s.sendMetricTicks(metricPrefix, seriesCount, spike, spikeTicks)

	s.waitForScorerEWMATelemetry()
	s.waitForScorerState("direction:escalation")

	s.appendRepeatedLog(mediumMessage, logBurst)
	mediumCount := s.waitForLogCountAtLeast(mediumMessage, mediumPhaseMin)

	s.sendMetricTicks(metricPrefix, seriesCount, baseline, recoveryTicks)
	s.waitForScorerState("direction:deescalation")

	s.appendRepeatedLog(lowRecoveryMessage, logBurst)
	lowRecoveryCount := s.waitForLogCountAtMost(lowRecoveryMessage, lowPhaseMax)

	s.T().Logf(
		"adaptive sampling delivered low=%d medium=%d low-recovery=%d logs",
		lowCount,
		mediumCount,
		lowRecoveryCount,
	)

	require.Greater(s.T(), mediumCount, lowCount, "medium severity should let more logs through than low severity")
	require.Greater(s.T(), mediumCount, lowRecoveryCount, "de-escalating back to low should tighten sampling again")
}

func (s *smartSeverityProfilesSuite) sendGauge(name string, value float64) {
	cmd := fmt.Sprintf("bash -c 'echo -n \"%s:%f|g\" > /dev/udp/127.0.0.1/8125'", name, value)
	if _, err := s.Env().RemoteHost.Execute(cmd); err != nil {
		s.T().Logf("sendGauge(%q, %f): SSH error (metric may not have been sent): %v", name, value, err)
	}
}

func (s *smartSeverityProfilesSuite) sendMetricTicks(metricPrefix string, seriesCount int, value float64, ticks int) {
	s.T().Helper()
	for tick := 0; tick < ticks; tick++ {
		for series := 0; series < seriesCount; series++ {
			s.sendGauge(fmt.Sprintf("%s%d", metricPrefix, series), value)
		}
		time.Sleep(time.Second)
	}
}

func (s *smartSeverityProfilesSuite) appendRepeatedLog(message string, count int) {
	s.T().Helper()
	logutils.AppendLog(s, smartSeverityLogFileName, message, count)
}

func (s *smartSeverityProfilesSuite) waitForScorerEWMATelemetry() {
	s.T().Helper()
	s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics(smartSeverityScorerEWMA)
		assert.NoError(c, err, "failed to fetch scorer ewma metric from fakeintake")
		assert.NotEmpty(c, metrics,
			"expected scorer ewma telemetry when smart severity profiles are enabled")
	}, 2*time.Minute, smartSeverityFakeintakeTick)
}

func (s *smartSeverityProfilesSuite) waitForScorerState(directionTag string) {
	s.T().Helper()
	s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics(smartSeverityScorerState)
		assert.NoError(c, err, "failed to fetch scorer state metric from fakeintake")
		assert.NotEmpty(c, metrics, "expected scorer state telemetry")
		assert.True(c, hasMetricWithTag(metrics, directionTag),
			"expected scorer state telemetry tagged with %s", directionTag)
	}, 2*time.Minute, smartSeverityFakeintakeTick)
}

func hasMetricWithTag(metrics []*aggregator.MetricSeries, want string) bool {
	for _, metric := range metrics {
		for _, tag := range metric.GetTags() {
			if tag == want || strings.Contains(tag, want) {
				return true
			}
		}
	}
	return false
}

func (s *smartSeverityProfilesSuite) waitForLogCountAtMost(message string, max int) int {
	s.T().Helper()
	var count int
	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := logutils.FetchAndFilterLogs(s.Env().FakeIntake, smartSeverityService, message)
		assert.NoError(c, err, "failed to fetch logs for %q", message)
		count = len(logs)
		assert.NotZero(c, count, "expected at least one delivered log for %q", message)
		assert.LessOrEqual(c, count, max, "expected adaptive sampling to heavily limit %q", message)
	}, 2*time.Minute, smartSeverityFakeintakeTick)
	return count
}

func (s *smartSeverityProfilesSuite) waitForLogCountAtLeast(message string, min int) int {
	s.T().Helper()
	var count int
	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := logutils.FetchAndFilterLogs(s.Env().FakeIntake, smartSeverityService, message)
		assert.NoError(c, err, "failed to fetch logs for %q", message)
		count = len(logs)
		assert.GreaterOrEqual(c, count, min, "expected the medium profile to admit most of %q", message)
	}, 2*time.Minute, smartSeverityFakeintakeTick)
	return count
}
