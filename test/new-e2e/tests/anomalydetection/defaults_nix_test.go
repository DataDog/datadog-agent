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

// disabledByDefaultSuite verifies that the observer is a no-op when
// anomaly_detection is not configured. This guards against the component
// activating silently in vanilla agent deployments.
type disabledByDefaultSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestAnomalyDetectionDisabledByDefault runs the agent with a stock config and
// asserts that no [observer] log lines are emitted.
func TestAnomalyDetectionDisabledByDefault(t *testing.T) {
	e2e.Run(t, &disabledByDefaultSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(scenec2.WithAgentOptions()),
		),
	), e2e.WithStackName("anomalydetection-disabled-default"))
}

// TestObserverSilentWithDefaultConfig asserts no [observer] lines appear in
// agent.log after the agent fully starts with default configuration.
func (s *disabledByDefaultSuite) TestObserverSilentWithDefaultConfig() {
	waitForAgentStartup(s)

	// Give the agent time to emit any potential observer startup logs.
	time.Sleep(10 * time.Second)

	out, err := s.Env().RemoteHost.ReadFilePrivileged("/var/log/datadog/agent.log")
	assert.NoError(s.T(), err, "reading agent.log for observer check")
	assert.NotContains(s.T(), string(out), "[observer]",
		"[observer] lines must not appear in agent.log when anomaly_detection is not configured")
}
