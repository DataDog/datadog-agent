// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type vmSuiteEx6 struct {
	e2e.BaseSuite[environments.Host]
}

func TestVMSuiteEx6(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &vmSuiteEx6{}, e2e.WithProvisioner(awshost.Provisioner(awshost.WithRunOptions(scenec2.WithAgentOptions(agentparams.WithSystemProbeConfig(systemProbeConfigNPM))))))
}

func (v *vmSuiteEx6) Test1_FakeIntakeNPM() {
	t := v.T()

	// force pulumi to deploy before running the test
	v.Env().RemoteHost.MustExecute("curl http://www.datadoghq.com")
	v.Env().FakeIntake.Client().FlushServerAndResetAggregators()

	// This loop waits for agent and system-probe to be ready, stated by
	// checking we eventually receive a payload
	v.EventuallyWithT(func(c *assert.CollectT) {
		v.Env().RemoteHost.MustExecute("curl http://www.datadoghq.com")

		hostnameNetID, err := v.Env().FakeIntake.Client().GetConnectionsNames()
		require.NoError(c, err, "fakeintake GetConnectionsNames() error")

		if assert.NotZero(c, len(hostnameNetID), "no connections yet") {
			t.Logf("hostname+networkID %v seen connections", hostnameNetID)
		}
	}, 60*time.Second, time.Second, "")
}
