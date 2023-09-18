// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
)

type vmSuiteEx6 struct {
	e2e.Suite[e2e.FakeIntakeEnv]
}

func TestVMSuiteEx6(t *testing.T) {
	e2e.Run(t, &vmSuiteEx6{}, e2e.FakeIntakeStackDef(nil, agentparams.WithSystemProbeConfig(SystemProbeConfig)))
}

func (v *vmSuiteEx6) Test1_FakeIntakeNPM() {
	t := v.T()

	// force pulumi to deploy before running the test
	v.Env().VM.Execute("curl http://httpbin.org/anything")

	// This loop waits for agent and system-probe to be ready, stated by
	// checking we eventually receive a payload
	v.EventuallyWithT(func(c *assert.CollectT) {
		v.Env().VM.Execute("curl http://httpbin.org/anything")

		hostnameNetID, err := v.Env().Fakeintake.GetConnectionsNames()
		assert.NoError(c, err, "fakeintake GetConnectionsNames() error")

		assert.NotZero(c, len(hostnameNetID), "no connections yet")

		if len(hostnameNetID) > 0 {
			t.Logf("hostname+networkID %v seen connections", hostnameNetID)
		}
	}, 60*time.Second, time.Second, "")
}
