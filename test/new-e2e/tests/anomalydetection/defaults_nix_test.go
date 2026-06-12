// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package anomalydetection

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

// observerMetricNames lists anomaly-detection observer telemetry expected only
// when the observer pipeline is enabled.
var observerMetricNames = []string{
	"observer.channel.dropped",
	"observer.rrcf.score",
	"observer.rrcf.threshold",
	"observer.log_pattern_extractor.pattern_count",
	telemetryLogsIngested,
	"observer.logs.processed_bytes",
	"observer.logs.dropped",
	telemetrySeriesCount,
	telemetryReportsEmitted,
	telemetryReportsOngoing,
	telemetryLogsInFlight,
	"observer.storage.series_evicted",
	"observer.storage.capacity_hit",
	"observer.scheduler.advance_skipped",
}

// disabledByDefaultSuite verifies that the observer is a no-op when
// anomaly_detection is not configured. This guards against the component
// activating silently in vanilla agent deployments.
type disabledByDefaultSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestAnomalyDetectionDisabledByDefault runs the agent with a stock config and
// asserts that the observer analysis pipeline is not active.
func TestAnomalyDetectionDisabledByDefault(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &disabledByDefaultSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(scenec2.WithAgentOptions()),
		),
	), e2e.WithStackName("anomalydetection-disabled-default"))
}

// TestObserverSilentWithDefaultConfig asserts the observer pipeline is disabled
// by default by checking that observer telemetry is not present.
func (s *disabledByDefaultSuite) TestObserverSilentWithDefaultConfig() {
	waitForAgentStartup(s)

	// Give the agent time to initialize all components and metadata collectors.
	time.Sleep(10 * time.Second)

	tel := observerTelemetryOutput(s)
	for _, metricName := range observerMetricNames {
		assert.False(s.T(), containsMetric(tel, metricName),
			"observer telemetry %q should not be emitted with default config", metricName)
	}
}
