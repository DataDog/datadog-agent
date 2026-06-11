// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package anomalydetection

// scorer_helper_nix_test.go — anomaly scorer helper integration test.
//
// Verifies that when anomaly_detection.detectors.anomaly_scorer is enabled and
// the EWMA exceeds low_threshold (driven by simultaneous anomalies on multiple
// metric series), the built-in anomalyScorerHelper fires OnSeverityTransition and
// emits its distinctive log line to the agent journal.
//
// Design notes:
//
//   - The EWMA approach (polling Prometheus for a non-zero gauge) is fragile: with
//     the default alpha=0.014 a single anomalous series raises the EWMA too slowly
//     to reliably cross low_threshold=0.040 within the test window, and the gauge is
//     reset to 0 on every advance tick when no anomalies are queued.
//
//   - Instead we check for the helper's explicit log line — a binary signal that
//     OnSeverityTransition fired — which is robust and consistent with the rest of
//     this test suite (observerReportMarker, observerReadyMarker).
//
//   - To reliably cross low_threshold we need simultaneous anomalies on multiple
//     series: 5 concurrent metric names each spiking at the same time give
//     saturation(5,k=5)×meanWeight ≈ 0.63 input per second, so with alpha=0.3 the
//     EWMA crosses 0.040 in ~2 advance ticks, well within the 3-minute timeout.
//
//   - BOCPD is the default-enabled detector and its warmup_points can be set
//     directly via the agent YAML (unlike holt_residual / tukey_biweight which only
//     accept JSON overrides). Setting warmup_points=20 matches the existing log-
//     triggered test and keeps the total test time well under 3 minutes.
//     holt_residual is also enabled; its default warmup of 24 points means it starts
//     scoring shortly after BOCPD, adding a second anomaly source per series.
//     tukey_biweight is left disabled (default min_points=80 is too slow for a test).
//
//   - helper.report_events is intentionally left at its default (false): we do not
//     need a fakeintake here because the log line is the assertion target.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

// scorerHelperSuite exercises the anomalyScorerHelper severity-transition path.
type scorerHelperSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestAnomalyDetectionScorerHelper provisions a Linux VM with the anomaly scorer
// and CUSUM detector enabled, using a fast alpha so that simultaneous anomalies on
// multiple metric series push the EWMA above low_threshold within seconds.
func TestAnomalyDetectionScorerHelper(t *testing.T) {
	t.Parallel()
	// language=yaml
	agentConfig := `
log_level: debug
anomaly_detection:
  enabled: true
  metrics:
    enabled: true
  logs:
    enabled: false
  agent_logs:
    enabled: false
  detectors:
    cusum:
      enabled: false
    bocpd:
      enabled: true
      warmup_points: 20
    holt_residual:
      enabled: true
    tukey_biweight:
      enabled: false
    anomaly_scorer:
      enabled: true
      alpha: 0.3
      window_secs: 5
      low_threshold: 0.040
      high_threshold: 0.060
`
	e2e.Run(t, &scorerHelperSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(scenec2.WithAgentOptions(agentparams.WithAgentConfig(agentConfig))),
		),
	), e2e.WithStackName("anomalydetection-scorer-helper"))
}

// sendHelperGauge sends one DogStatsD gauge over UDP to the local agent.
func (s *scorerHelperSuite) sendHelperGauge(name string, value float64) {
	cmd := fmt.Sprintf("bash -c 'echo -n \"%s:%f|g\" > /dev/udp/127.0.0.1/8125'", name, value)
	if _, err := s.Env().RemoteHost.Execute(cmd); err != nil {
		s.T().Logf("sendHelperGauge(%q, %f): SSH error (metric may not have been sent): %v", name, value, err)
	}
}

// TestScorerHelperEmitsSeverityTransitionOnMultiSeriesSpike sends a stable baseline
// on 5 metric series (each distinct so they count as separate anomaly sources), then
// spikes all 5 simultaneously. The 5 concurrent BOCPD/HoltResidual anomalies drive
// the scorer EWMA above low_threshold in ~2 advance ticks, causing the
// anomalyScorerHelper to call OnSeverityTransition and emit its "severity escalation"
// log line to the journal.
//
// The test is self-contained: it does not require fakeintake or a real Datadog backend.
// helper.report_events is left at its default (false).
func (s *scorerHelperSuite) TestScorerHelperEmitsSeverityTransitionOnMultiSeriesSpike() {
	// 5 distinct metric names → 5 independent anomaly series that spike at once.
	// BOCPD needs warmup_points=20, holt_residual needs 24 points; we send 30 baseline
	// ticks to give a comfortable margin against SSH-latency-induced dropped ticks.
	// Using 5 series gives saturation(5, k=5) ≈ 0.63 input per advance tick.
	// With alpha=0.3, EWMA ≈ 0.3×0.63 = 0.19 after the very first spike second,
	// well above low_threshold=0.040.
	const (
		seriesCount    = 5
		metricPrefix   = "e2e.anomalydetection.scorer.s"
		baseline       = 1.0
		spike          = 5000.0
		baselinePoints = 30 // comfortable margin above BOCPD warmup_points=20 and holt_residual WarmupPoints=24
		spikePoints    = 10 // 10 seconds of spike; EWMA crosses threshold in the first tick
	)

	waitForObserverReady(s)

	ctx, cancel := context.WithCancel(s.T().Context())
	ticker := time.NewTicker(time.Second)
	done := make(chan struct{})
	defer func() {
		cancel()
		<-done
		ticker.Stop()
	}()

	go func() {
		defer close(done)

		// Phase 1: baseline — send one point per second on each series.
		// All 5 series run in lockstep using the same ticker so they spike together.
		s.T().Logf("sending %d baseline points on %d series...", baselinePoints, seriesCount)
		for i := 0; i < baselinePoints; i++ {
			select {
			case <-ctx.Done():
				return
			default:
			}
			for n := 0; n < seriesCount; n++ {
				s.sendHelperGauge(fmt.Sprintf("%s%d", metricPrefix, n), baseline)
			}
			if (i+1)%5 == 0 {
				s.T().Logf("baseline: tick %d/%d", i+1, baselinePoints)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}

		// Phase 2: spike — all 5 series simultaneously so the scorer sees
		// 5 concurrent anomaly sources in the same advance window.
		s.T().Logf("sending %d spike points on %d series...", spikePoints, seriesCount)
		for i := 0; i < spikePoints; i++ {
			select {
			case <-ctx.Done():
				return
			default:
			}
			for n := 0; n < seriesCount; n++ {
				s.sendHelperGauge(fmt.Sprintf("%s%d", metricPrefix, n), spike)
			}
			if (i+1)%5 == 0 {
				s.T().Logf("spike: tick %d/%d", i+1, spikePoints)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
		s.T().Log("done sending metrics")
	}()

	// Poll the journal for the helper's severity-escalation log line.
	// The helper emits: "[observer] anomaly scorer anomaly_scorer severity escalation to Medium (was Low, t=...)"
	s.T().Log("polling journal for scorer helper severity escalation marker...")
	s.EventuallyWithT(func(c *assert.CollectT) {
		out, err := s.Env().RemoteHost.Execute("sudo journalctl -u datadog-agent --no-pager")
		assert.NoError(c, err, "journalctl execution failed")
		assert.Contains(c, out, scorerHelperEscalationMarker,
			"journald should contain the helper's severity escalation log line")
	}, 3*time.Minute, 5*time.Second)

	dumpObserverLines(s.T(), s.Env())
	s.T().Log("scorer helper severity escalation marker found — anomalyScorerHelper wired correctly")
}
