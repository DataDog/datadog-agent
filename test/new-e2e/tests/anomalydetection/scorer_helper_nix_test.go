// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package anomalydetection

// scorer_helper_nix_test.go — anomaly scorer integration test.
//
// Verifies that when anomaly_detection.anomaly_scorer is enabled with output.logs=true and
// the EWMA exceeds low_threshold (driven by simultaneous anomalies on multiple
// metric series), the internal watcher fires OnSeverityTransition and emits its
// distinctive log line to the agent journal.
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
//     this test suite (scorerHelperEscalationMarker, scorerHelperRegisteredMarker).
//
//   - CUSUM is used (consistent with TestAnomalyDetectionMetricsTriggered): it fires
//     deterministically after 5 baseline points and re-emits on every analysis cycle
//     while the spike is active. This gives continuous EWMA input, unlike BOCPD
//     (fires once, requires 120-point warmup by default, and suffers Gaussian PDF
//     underflow on extreme spikes) or HoltResidual (requires 84 baseline points).
//
//   - Thresholds are set very low (low=0.005, high=0.010) and 20 concurrent series
//     are used so that EWMA(1s) ≈ 0.29 with alpha=0.3, >> both thresholds even if
//     several ticks are dropped due to SSH latency. This eliminates flakiness.
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

// scorerHelperSuite exercises the anomaly scorer watcher severity-transition path.
type scorerHelperSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestAnomalyDetectionScorerHelper provisions a Linux VM with the anomaly scorer
// and CUSUM detector enabled. The scorer's output.logs=true so that severity
// transitions are logged. CUSUM re-emits on every analysis cycle so the scorer EWMA
// rises continuously as long as the spike is active. Thresholds are set deliberately
// low so that even a single anomalous series crosses low_threshold within a couple of
// seconds, making the test robust against SSH latency and dropped ticks.
func TestAnomalyDetectionScorerHelper(t *testing.T) {
	t.Parallel()
	// language=yaml
	agentConfig := `
log_level: debug
anomaly_detection:
  anomaly_scorer:
    dry_run:
      enabled: true
    alpha: 0.3
    window: 5s
    low_threshold: 0.005
    high_threshold: 0.010
    output:
      logs: true
  metrics:
    enabled: true
  logs:
    enabled: false
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
	e2e.Run(t, &scorerHelperSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(scenec2.WithAgentOptions(agentparams.WithAgentConfig(agentConfig))),
		),
	), e2e.WithStackName("anomalydetection-scorer-helper"))
}

// TestScorerHelperEmitsSeverityTransitionOnMultiSeriesSpike sends a stable baseline
// on 20 metric series then spikes all 20 simultaneously. CUSUM fires on every analysis
// cycle while the spike is active (continuous re-emission), so the scorer EWMA rises
// steadily and crosses low_threshold=0.005 within 1–2 seconds of the first spike tick.
// With alpha=0.3 and 20 series: EWMA(1 s) ≈ 0.3 × 0.98 ≈ 0.29, >> low_threshold.
//
// The test is self-contained: it does not require fakeintake or a real Datadog backend.
// helper.report_events is left at its default (false).
func (s *scorerHelperSuite) TestScorerHelperEmitsSeverityTransitionOnMultiSeriesSpike() {
	// 20 distinct metric names → 20 independent anomaly series that spike at once.
	// With 20 series: saturation(20, k=5) ≈ 0.98, EWMA(1s) ≈ 0.3×0.98 ≈ 0.29 >> thresholds.
	const (
		seriesCount    = 20
		metricPrefix   = "e2e.anomalydetection.scorer.s"
		baseline       = 1.0
		spike          = 5000.0
		baselinePoints = 15 // matches TestMetricsTriggeredEmitsOnDSDSpike
		spikePoints    = 10 // matches TestMetricsTriggeredEmitsOnDSDSpike
	)

	waitForScorerHelperReady(s)

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
		// All 20 series run in lockstep using the same ticker so they spike together.
		s.T().Logf("sending %d baseline points on %d series...", baselinePoints, seriesCount)
		for i := 0; i < baselinePoints; i++ {
			select {
			case <-ctx.Done():
				return
			default:
			}
			for n := 0; n < seriesCount; n++ {
				sendGauge(s, fmt.Sprintf("%s%d", metricPrefix, n), baseline)
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

		// Phase 2: spike — all 20 series simultaneously so the scorer sees
		// 20 concurrent anomaly sources in the same advance window.
		s.T().Logf("sending %d spike points on %d series...", spikePoints, seriesCount)
		for i := 0; i < spikePoints; i++ {
			select {
			case <-ctx.Done():
				return
			default:
			}
			for n := 0; n < seriesCount; n++ {
				sendGauge(s, fmt.Sprintf("%s%d", metricPrefix, n), spike)
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

	// Poll the journal for the scorer watcher's severity-escalation log line.
	// The watcher emits: "[observer] anomaly scorer anomaly_scorer severity escalation to Medium (was Low, t=...)"
	s.T().Log("polling journal for scorer severity escalation marker...")
	s.EventuallyWithT(func(c *assert.CollectT) {
		out, err := s.Env().RemoteHost.Execute("sudo journalctl -u datadog-agent --no-pager")
		assert.NoError(c, err, "journalctl execution failed")
		assert.Contains(c, out, scorerHelperEscalationMarker,
			"journald should contain the scorer watcher's severity escalation log line")
	}, 5*time.Minute, 5*time.Second)

	s.T().Log("scorer severity escalation marker found — anomaly scorer watcher wired correctly")
}
