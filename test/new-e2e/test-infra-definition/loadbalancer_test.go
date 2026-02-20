// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testinfradefinition

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"

	"github.com/stretchr/testify/assert"
)

type loadBalancerSuiteMetrics struct {
	e2e.BaseSuite[environments.Host]
}

func TestLoadBalancer(t *testing.T) {
	e2e.Run(t, &loadBalancerSuiteMetrics{}, e2e.WithProvisioner(awshost.Provisioner(awshost.WithRunOptions(ec2.WithFakeIntakeOptions(fakeintake.WithLoadBalancer())))), e2e.WithSkipCoverage())
}

func (v *loadBalancerSuiteMetrics) Test_FakeIntakeReceivesMetrics() {
	v.EventuallyWithT(func(c *assert.CollectT) {
		metricNames, err := v.Env().FakeIntake.Client().GetMetricNames()
		assert.NoError(c, err)
		assert.Greater(c, len(metricNames), 0)
	}, 2*time.Minute, 10*time.Second)
}
