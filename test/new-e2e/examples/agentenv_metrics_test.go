// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"testing"
	"time"

	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/hostagent"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/fakeintake/client"

	"github.com/stretchr/testify/assert"
)

type fakeintakeSuiteMetrics struct {
	e2e.BaseSuite[environments.Host]
}

func TestVMSuiteEx5(t *testing.T) {
	// Provisioner creates infrastructure only — VM + fakeintake, no agent.
	e2e.Run(t, &fakeintakeSuiteMetrics{}, e2e.WithProvisioner(
		awshost.Provisioner(awshost.WithRunOptions(scenec2.WithoutAgent())),
	))
}

func (v *fakeintakeSuiteMetrics) SetupSuite() {
	v.BaseSuite.SetupSuite()
	defer v.CleanupOnSetupFailure()

	// Install the agent via SSH. hostagent.Install detects the fakeintake
	// in the environment and automatically configures the agent to send
	// data to it.
	hostagent.Install(v.T(), v.Env())
}

func (v *fakeintakeSuiteMetrics) Test1_FakeIntakeReceivesMetrics() {
	v.EventuallyWithT(func(c *assert.CollectT) {
		metricNames, err := v.Env().FakeIntake.Client().GetMetricNames()
		assert.NoError(c, err)
		assert.Greater(c, len(metricNames), 0)
	}, 5*time.Minute, 10*time.Second)
}

func (v *fakeintakeSuiteMetrics) Test2_FakeIntakeReceivesSystemLoadMetric() {
	v.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := v.Env().FakeIntake.Client().FilterMetrics("system.load.1")
		assert.NoError(c, err)
		assert.Greater(c, len(metrics), 0, "no 'system.load.1' metrics yet")
	}, 5*time.Minute, 10*time.Second)
}

func (v *fakeintakeSuiteMetrics) Test3_FakeIntakeReceivesSystemUptimeHigherThanZero() {
	v.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := v.Env().FakeIntake.Client().FilterMetrics("system.uptime", client.WithMetricValueHigherThan(0))
		assert.NoError(c, err)
		assert.Greater(c, len(metrics), 0, "no 'system.uptime' with value higher than 0 yet")
	}, 5*time.Minute, 10*time.Second)
}
