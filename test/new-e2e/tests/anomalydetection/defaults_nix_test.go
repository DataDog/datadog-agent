// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package anomalydetection

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

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

// TestObserverSilentWithDefaultConfig asserts the observer analysis pipeline is
// not wired when anomaly_detection is not configured.
//
// Note: some [observer] lines (e.g. "[observer] getting handle for system-checks-hf")
// may appear because the HFRunner calls GetHandle unconditionally even on the noop
// path. The meaningful signal is observerReadyMarker — the "all-metrics" handle that
// the aggregator creates only when the full analysis pipeline is active.
func (s *disabledByDefaultSuite) TestObserverSilentWithDefaultConfig() {
	waitForAgentStartup(s)

	// Give the agent time to emit any potential observer startup logs.
	time.Sleep(10 * time.Second)

	out, err := s.Env().RemoteHost.ReadFilePrivileged("/var/log/datadog/agent.log")
	assert.NoError(s.T(), err, "reading agent.log for observer check")
	agentLog := string(out)

	// Collect any [observer] lines as evidence for the failure message.
	// The match uses "[observer" (no closing bracket) to catch all observer-prefixed
	// tags including [observer/hfrunner], [observer/engine], etc.
	var culprits []string
	for _, line := range strings.Split(agentLog, "\n") {
		if strings.Contains(line, "[observer") {
			culprits = append(culprits, line)
		}
	}

	assert.NotContains(s.T(), agentLog, observerReadyMarker,
		"observer analysis pipeline must not start with default config; [observer] lines found:\n%s",
		strings.Join(culprits, "\n"))
}
