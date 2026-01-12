// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	localhost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/local/host"
	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

type vmLocalPodmanSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestVMLocalPodmanSuite runs tests for the VM interface provisioned on a local podman managed container.
func TestVMLocalPodmanSuite(t *testing.T) {
	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(localhost.PodmanProvisioner())}

	e2e.Run(t, &vmLocalPodmanSuite{}, suiteParams...)
}

func (v *vmLocalPodmanSuite) TestExecute() {
	vm := v.Env().RemoteHost

	out, err := vm.Execute("whoami")
	v.Require().NoError(err)
	v.Require().NotEmpty(out)
}

func (v *vmLocalPodmanSuite) TestFakeIntakeReceivesSystemLoadMetric() {
	v.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := v.Env().FakeIntake.Client().FilterMetrics("system.load.1")
		assert.NoError(c, err)
		assert.Greater(c, len(metrics), 0, "no 'system.load.1' metrics yet")
	}, 5*time.Minute, 10*time.Second)
}

func (v *vmLocalPodmanSuite) TestFakeIntakeReceivesSystemUptimeHigherThanZero() {
	v.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := v.Env().FakeIntake.Client().FilterMetrics("system.uptime", client.WithMetricValueHigherThan(0))
		assert.NoError(c, err)
		assert.Greater(c, len(metrics), 0, "no 'system.uptime' with value higher than 0 yet")
	}, 5*time.Minute, 10*time.Second)
}
