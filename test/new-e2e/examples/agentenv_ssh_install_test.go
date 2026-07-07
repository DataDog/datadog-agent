// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package examples

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

// This example demonstrates installing the Agent as a separate, Pulumi-free step. Pulumi provisions
// only the infrastructure (a bare host via ec2.WithoutAgent(), plus a fakeintake); the Agent is then
// installed over SSH via env.InstallAgent, which auto-wires the environment's fakeintake. Once the
// BaseSuite opt-in hook lands, this becomes an option rather than an explicit call in the test body.
type sshInstalledAgentSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestSSHInstalledAgentSuite(t *testing.T) {
	e2e.Run(t, &sshInstalledAgentSuite{},
		e2e.WithProvisioner(awshost.Provisioner(awshost.WithRunOptions(ec2.WithoutAgent()))))
}

// Test0 runs first (suite methods run in name order) and installs the Agent over SSH, so the later
// assertions see a running, fakeintake-configured Agent.
func (v *sshInstalledAgentSuite) Test0_InstallAgentOverSSH() {
	v.Require().NoError(v.Env().InstallAgent(v))
}

func (v *sshInstalledAgentSuite) Test1_FakeIntakeReceivesMetrics() {
	v.EventuallyWithT(func(c *assert.CollectT) {
		metricNames, err := v.Env().FakeIntake.Client().GetMetricNames()
		assert.NoError(c, err)
		assert.Greater(c, len(metricNames), 0)
	}, 5*time.Minute, 10*time.Second)
}
