// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package anomalydetection

import (
	_ "embed"
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
)

//go:embed fixtures/stable_pipeline_agent.yaml
var stablePipelineAgentConfig string

//go:embed fixtures/stable_pipeline_sender.sh
var stablePipelineSenderScript string

const (
	stablePipelineSmallMetric = "e2e.anomalydetection.stable.small"
	stablePipelineLargeMetric = "e2e.anomalydetection.stable.large"
	stablePipelineDetector    = "stable_pipeline:scanmw"
	stablePipelineGate        = "relative_change"

	stablePipelineControlFile = "/tmp/anomalydetection-stable-e2e-control"
	stablePipelineTraceFile   = "/tmp/anomalydetection-stable-e2e.trace"
	stablePipelineUnitName    = "anomalydetection-stable-e2e-sender"

	stablePipelineAcceptedMarker = "reporter anomaly detected: source="
	stablePipelineKindEmissions  = "observer.detections.kind_emissions"
	stablePipelineGatedCounter   = "observer.detections.gated"
	stablePipelineKindSeries     = "observer.metric_kind.series_count"
	stablePipelineKind           = "stable"
)

type stablePipelineSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestAnomalyDetectionStablePipeline(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &stablePipelineSuite{}, e2e.WithProvisioner(
		awshost.ProvisionerNoFakeIntake(
			awshost.WithRunOptions(scenec2.WithAgentOptions(
				agentparams.WithAgentConfig(stablePipelineAgentConfig),
				agentparams.WithFile(
					"/tmp/anomalydetection-stable-e2e-sender.sh",
					stablePipelineSenderScript,
					false,
				),
			)),
		),
	), e2e.WithStackName("anomalydetection-stable-pipeline"))
}

func (s *stablePipelineSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)
	stopStablePipelineSender(s)
	_, err := s.Env().RemoteHost.Execute(
		fmt.Sprintf("sudo rm -f %s %s /tmp/anomalydetection-stable-e2e-sender.sh",
			stablePipelineControlFile, stablePipelineTraceFile),
	)
	require.NoError(s.T(), err, "remove stable pipeline state")
	require.NoError(s.T(), s.Env().Agent.Client.Restart(), "restart the agent for a clean analysis state")
}

func (s *stablePipelineSuite) AfterTest(suiteName, testName string) {
	stopStablePipelineSender(s)
	s.BaseSuite.AfterTest(suiteName, testName)
}

func (s *stablePipelineSuite) TestStableGaugeSquareWaves() {
	stopStablePipelineSender(s)
	s.T().Cleanup(func() {
		if s.T().Failed() {
			trace, err := s.Env().RemoteHost.ReadFilePrivileged(stablePipelineTraceFile)
			if err != nil {
				s.T().Logf("reading sender trace: %v", err)
			} else {
				s.T().Logf("sender trace:\n%s", trace)
			}
		}
		stopStablePipelineSender(s)
	})

	waitForObserverReady(s)
	journalCursor := stablePipelineJournalCursor(s)
	_, err := s.Env().RemoteHost.Execute(
		fmt.Sprintf("sudo chmod 755 /tmp/anomalydetection-stable-e2e-sender.sh && sudo systemd-run --unit=%s --collect --property=RuntimeMaxSec=420 /tmp/anomalydetection-stable-e2e-sender.sh",
			stablePipelineUnitName),
	)
	require.NoError(s.T(), err, "start stable pipeline sender")

	s.EventuallyWithT(func(c *assert.CollectT) {
		tel := observerTelemetryOutput(s)
		series, ok := telemetryCounterValue(tel, stablePipelineKindSeries,
			map[string]string{"kind": stablePipelineKind})
		s.T().Logf("waiting for stable baseline series=%v", series)
		require.True(c, ok, "expected metric-kind series telemetry")
		require.EqualValues(c, 2, series, "expected both gauges to classify as stable")
	}, 2*time.Minute, 3*time.Second)

	baselineJournal := stablePipelineJournalAfter(s, journalCursor)
	require.Zero(s.T(), countStablePipelineAnomalies(baselineJournal, stablePipelineSmallMetric),
		"the baseline must not accept small-gauge anomalies")
	require.Zero(s.T(), countStablePipelineAnomalies(baselineJournal, stablePipelineLargeMetric),
		"the baseline must not accept large-gauge anomalies")

	_, err = s.Env().RemoteHost.Execute("sudo touch " + stablePipelineControlFile)
	require.NoError(s.T(), err, "switch sender to square-wave mode")

	smallAccepted := false
	var largeJournal, gatedTel, emissionsTel string
	s.EventuallyWithT(func(c *assert.CollectT) {
		largeJournal = stablePipelineJournalAfter(s, journalCursor)
		emissionsTel = observerTelemetryOutput(s)
		gatedTel = emissionsTel

		emissions, _ := telemetryCounterValue(emissionsTel, stablePipelineKindEmissions, map[string]string{
			"detector": stablePipelineDetector,
			"kind":     stablePipelineKind,
		})
		gated, _ := telemetryCounterValue(gatedTel, stablePipelineGatedCounter, map[string]string{
			"detector": stablePipelineDetector,
			"kind":     stablePipelineKind,
			"gate":     stablePipelineGate,
		})
		largeAccepted := countStablePipelineAnomalies(largeJournal, stablePipelineLargeMetric)
		if countStablePipelineAnomalies(largeJournal, stablePipelineSmallMetric) > 0 {
			smallAccepted = true
		}
		s.T().Logf("square-wave state large_anomalies=%d emissions=%v gated=%v small_accepted=%t",
			largeAccepted, emissions, gated, smallAccepted)

		require.False(c, smallAccepted, "the small wave must not be accepted")
		require.Positive(c, largeAccepted, "expected an accepted large-gauge anomaly")
		require.Greater(c, emissions, 0.0, "expected stable-pipeline kind emissions")
		require.Greater(c, gated, 0.0, "expected relative-gate rejections")
		for _, metric := range []string{stablePipelineSmallMetric, stablePipelineLargeMetric} {
			line := findAcceptedAnomalyLine(largeJournal, metric)
			require.NotEmpty(c, line, "expected accepted anomaly record for %s", metric)
			require.Contains(c, line, "detector="+stablePipelineDetector)
		}
		series, ok := telemetryCounterValue(emissionsTel, stablePipelineKindSeries,
			map[string]string{"kind": stablePipelineKind})
		require.True(c, ok, "expected metric-kind series telemetry")
		require.EqualValues(c, 2, series, "both series must remain stable")
	}, 3*time.Minute, 3*time.Second)
	require.False(s.T(), smallAccepted, "the small wave must not be accepted")
}

func stopStablePipelineSender(s observerTestSuite) {
	s.T().Helper()
	_, err := s.Env().RemoteHost.Execute(
		fmt.Sprintf("sudo systemctl stop %s.service 2>/dev/null || true; sudo systemctl reset-failed %s.service 2>/dev/null || true",
			stablePipelineUnitName, stablePipelineUnitName),
	)
	if err != nil {
		s.T().Logf("stopping stable pipeline sender: %v", err)
	}
}

func stablePipelineJournalCursor(s observerTestSuite) string {
	s.T().Helper()
	out, err := s.Env().RemoteHost.Execute("sudo journalctl -u datadog-agent -n 0 --show-cursor --no-pager")
	if err != nil {
		s.T().Logf("reading journal cursor: %v", err)
		return ""
	}
	for _, line := range strings.Split(out, "\n") {
		if cursor, found := strings.CutPrefix(line, "-- cursor: "); found {
			fields := strings.Fields(cursor)
			if len(fields) > 0 {
				return fields[0]
			}
		}
	}
	return ""
}

func stablePipelineJournalAfter(s observerTestSuite, cursor string) string {
	s.T().Helper()
	command := "sudo journalctl -u datadog-agent --no-pager"
	if cursor != "" {
		command += " --after-cursor=" + cursor
	}
	out, err := s.Env().RemoteHost.Execute(command)
	require.NoError(s.T(), err, "read agent journal")
	return out
}

func countStablePipelineAnomalies(journal, metric string) int {
	count := 0
	for _, line := range strings.Split(journal, "\n") {
		if isAcceptedAnomalyLine(line, metric) {
			count++
		}
	}
	return count
}

func findAcceptedAnomalyLine(journal, metric string) string {
	for _, line := range strings.Split(journal, "\n") {
		if isAcceptedAnomalyLine(line, metric) {
			return line
		}
	}
	return ""
}

func isAcceptedAnomalyLine(line, metric string) bool {
	if !strings.Contains(line, stablePipelineAcceptedMarker) {
		return false
	}
	for _, field := range strings.Fields(line) {
		if strings.HasPrefix(field, "source="+metric+":") || strings.HasPrefix(field, "source="+metric+"{") {
			return true
		}
	}
	return false
}

func telemetryCounterValue(telemetryOutput, metric string, tags map[string]string) (float64, bool) {
	var total float64
	found := false
	for _, line := range strings.Split(telemetryOutput, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || !matchesTelemetryName(trimmed, metric) {
			continue
		}
		openIndex := strings.Index(trimmed, "{")
		closeIndex := strings.LastIndex(trimmed, "}")
		if openIndex < 0 || closeIndex <= openIndex {
			continue
		}
		if !telemetryLineHasTags(trimmed[openIndex+1:closeIndex], tags) {
			continue
		}
		fields := strings.Fields(trimmed[closeIndex+1:])
		if len(fields) == 0 {
			continue
		}
		value, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			continue
		}
		total += value
		found = true
	}
	return total, found
}

func matchesTelemetryName(line, metric string) bool {
	open := strings.Index(line, "{")
	name := line
	if open >= 0 {
		name = line[:open]
	}
	name = strings.TrimSuffix(name, "_total")
	snake := strings.ReplaceAll(metric, ".", "_")
	for _, variant := range []string{metric, "observer__" + metric, snake, "observer__" + snake} {
		if name == variant || strings.HasSuffix(name, "_"+variant) {
			return true
		}
	}
	return false
}

func telemetryLineHasTags(tagsText string, want map[string]string) bool {
	observed := make(map[string]string)
	for _, pair := range strings.Split(tagsText, ",") {
		key, value, found := strings.Cut(pair, "=")
		if !found {
			continue
		}
		observed[key] = strings.Trim(value, `"`)
	}
	for key, expected := range want {
		if observed[key] != expected {
			return false
		}
	}
	return true
}
