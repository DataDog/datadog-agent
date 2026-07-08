// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package examples

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

// This example demonstrates installing the Agent as a separate, Pulumi-free step. Pulumi provisions
// only the infrastructure (a bare host plus a fakeintake); the Agent is then installed over SSH after
// provisioning via awshost.WithSSHInstalledAgent, which also auto-wires the environment's fakeintake.
// The install is transparent — no per-test wiring — and the test body is identical to a suite whose
// Agent was installed by Pulumi.
type sshInstalledAgentSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestSSHInstalledAgentSuite(t *testing.T) {
	e2e.Run(t, &sshInstalledAgentSuite{},
		e2e.WithProvisioner(awshost.Provisioner(awshost.WithSSHInstalledAgent())))
}

func (v *sshInstalledAgentSuite) TestFakeIntakeReceivesMetrics() {
	v.EventuallyWithT(func(c *assert.CollectT) {
		metricNames, err := v.Env().FakeIntake.Client().GetMetricNames()
		assert.NoError(c, err)
		assert.Greater(c, len(metricNames), 0)
	}, 5*time.Minute, 10*time.Second)
}
