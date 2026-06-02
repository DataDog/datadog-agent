// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package anomalydetection

// anomaly_event_severity_nix_test.go verifies the severity-progression output
// of the StdoutAnomalyEventConsumer that is wired into the live observer.
//
// Why multiple series are required for HIGH severity
//
// The anomaly event scorer combines per-signal scores via noisy-OR, then applies
// a logarithmic count cap that grows with the number of anomalies in the 60 s
// sliding window. A single series is capped at ~0.60 (N=1), keeping the EWMA
// below the high threshold (0.75). Only when several distinct series each
// produce an anomaly within the same window does the cap grow enough for the
// EWMA to cross 0.75.
//
// Test strategy
//
// We enable the CUSUM detector and send a 15-point baseline + one large spike
// for each of 7 distinct metrics. Each spike fires one CUSUM anomaly; the 7
// anomalies all fall inside the 60 s scoring window. The EWMA trajectory is:
//
//	events 1–2 → LOW    (·) — EWMA < 0.40
//	events 3–6 → MEDIUM (●) — EWMA ∈ [0.40, 0.75)
//	event  7   → HIGH   (▲) — EWMA > 0.75
//
// The StdoutAnomalyEventConsumer prints one line per event to stdout, which
// journald captures. We poll the journal for the three severity symbols in order.

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

const (
	// anomalyEventMarker is the prefix printed by StdoutAnomalyEventConsumer.
	anomalyEventMarker = "[anomaly-event]"

	// anomalyEventLowSymbol / MediumSymbol / HighSymbol are the UTF-8 severity
	// symbols printed by StdoutAnomalyEventConsumer.ProcessAnomalyEvent.
	anomalyEventLowSymbol    = "·"
	anomalyEventMediumSymbol = "●"
	anomalyEventHighSymbol   = "▲"
)

// anomalyEventSeveritySuite exercises the scored anomaly event pipeline end-to-end.
// It sends baselines + spikes for 7 distinct DSD metrics, expects the observer to
// emit LOW, MEDIUM, and then HIGH scored events via StdoutAnomalyEventConsumer.
type anomalyEventSeveritySuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestAnomalyEventSeverityProgression provisions a Linux VM with the CUSUM
// detector enabled and runs the severity progression test.
func TestAnomalyEventSeverityProgression(t *testing.T) {
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
      enabled: true
    bocpd:
      enabled: false
`
	e2e.Run(t, &anomalyEventSeveritySuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(scenec2.WithAgentOptions(agentparams.WithAgentConfig(agentConfig))),
		),
	), e2e.WithStackName("anomalydetection-severity-progression"))
}

// sendGauge sends one DogStatsD gauge over UDP to the local agent.
func (s *anomalyEventSeveritySuite) sendGauge(name string, value float64) {
	cmd := fmt.Sprintf("bash -c 'echo -n \"%s:%f|g\" > /dev/udp/127.0.0.1/8125'", name, value)
	if _, err := s.Env().RemoteHost.Execute(cmd); err != nil {
		s.T().Logf("sendGauge(%q, %f): SSH error: %v", name, value, err)
	}
}

// TestSeverityProgressionEmitsLowMediumHigh sends a 15-point baseline followed
// by a large spike for each of 7 distinct DSD metrics. The 7 resulting CUSUM
// anomalies all land within the 60 s scoring window; the EWMA rises through
// LOW, MEDIUM, and HIGH, printing the corresponding symbol on each event line.
func (s *anomalyEventSeveritySuite) TestSeverityProgressionEmitsLowMediumHigh() {
	const (
		// Seven distinct metric names produce seven distinct series. Each series
		// generates exactly one CUSUM anomaly on its spike, giving 7 anomalies in
		// the 60 s scorer window — enough to push the EWMA above 0.75 (HIGH).
		baseline       = 1.0
		spike          = 5000.0
		baselinePoints = 15 // CUSUM min_points default is 5; 15 gives a clean baseline
	)
	metrics := []string{
		"e2e.anomaly.severity.cpu",
		"e2e.anomaly.severity.mem",
		"e2e.anomaly.severity.disk",
		"e2e.anomaly.severity.net",
		"e2e.anomaly.severity.latency",
		"e2e.anomaly.severity.errors",
		"e2e.anomaly.severity.db",
	}

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

		// Baseline phase: one point per second for all metrics simultaneously.
		// All 7 series accumulate 15 baseline points before any spike fires.
		s.T().Logf("sending %d baseline points for %d metrics...", baselinePoints, len(metrics))
		for i := 0; i < baselinePoints; i++ {
			select {
			case <-ctx.Done():
				return
			default:
			}
			for _, m := range metrics {
				s.sendGauge(m, baseline)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}

		// Spike phase: spike one metric per second so all 7 anomalies land within
		// the 60 s scoring window. A 5000x spike reliably trips CUSUM regardless
		// of SSH-induced jitter on the baseline points.
		s.T().Logf("spiking %d metrics one per second...", len(metrics))
		for i, m := range metrics {
			select {
			case <-ctx.Done():
				return
			default:
			}
			s.sendGauge(m, spike)
			s.T().Logf("spiked %s (%d/%d)", m, i+1, len(metrics))
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}

		// Sentinel phase: send a few more baseline points to push analysis past
		// the last spike second (the scheduler only analyzes second T when T+1 data
		// arrives, same pattern as TestLogsTriggeredEmitsOnFileSpike).
		s.T().Log("sending sentinel points...")
		for i := 0; i < 3; i++ {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			for _, m := range metrics {
				s.sendGauge(m, baseline)
			}
		}
		s.T().Log("done sending metrics")
	}()

	// Poll the journal for the HIGH severity symbol. Because the EWMA is
	// monotonically increasing (every new anomaly raises the instant score),
	// LOW and MEDIUM events are guaranteed to appear before HIGH.
	s.T().Log("polling journal for HIGH severity anomaly event...")
	s.EventuallyWithT(func(c *assert.CollectT) {
		out, err := s.Env().RemoteHost.Execute("sudo journalctl -u datadog-agent --no-pager")
		assert.NoError(c, err, "journalctl execution failed")
		assert.Contains(c, out, anomalyEventMarker+` `, "journal must contain at least one scored anomaly event line")
		assert.Contains(c, out, anomalyEventHighSymbol, "journal must contain HIGH severity (▲) anomaly event")
	}, 3*time.Minute, 5*time.Second)

	// After HIGH is confirmed, assert the full progression is present.
	out, err := s.Env().RemoteHost.Execute("sudo journalctl -u datadog-agent --no-pager")
	assert.NoError(s.T(), err, "reading journal for final assertions")
	assert.Contains(s.T(), out, anomalyEventLowSymbol, "journal must contain LOW severity (·) event")
	assert.Contains(s.T(), out, anomalyEventMediumSymbol, "journal must contain MEDIUM severity (●) event")
	assert.Contains(s.T(), out, anomalyEventHighSymbol, "journal must contain HIGH severity (▲) event")

	// Verify ordering: LOW appears before MEDIUM, MEDIUM before HIGH.
	lowIdx := strings.Index(out, anomalyEventLowSymbol)
	mediumIdx := strings.Index(out, anomalyEventMediumSymbol)
	highIdx := strings.Index(out, anomalyEventHighSymbol)
	assert.Less(s.T(), lowIdx, mediumIdx, "LOW event must appear before first MEDIUM event")
	assert.Less(s.T(), mediumIdx, highIdx, "MEDIUM event must appear before HIGH event")

	dumpAnomalyEventLines(s.T(), s.Env())
}

// dumpAnomalyEventLines logs all [anomaly-event] journal lines for post-mortem diagnosis.
func dumpAnomalyEventLines(t *testing.T, env *environments.Host) {
	t.Helper()
	out, err := env.RemoteHost.Execute("sudo journalctl -u datadog-agent --no-pager | grep -F '[anomaly-event]' || true")
	if err != nil {
		t.Logf("warning: could not retrieve anomaly-event journal lines: %v", err)
		return
	}
	t.Logf("anomaly-event lines:\n%s", out)
}
