// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"errors"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/cenkalti/backoff/v4"
	"github.com/stretchr/testify/require"
)

const NPMsystemProbeConfig = `
network_config:
  enabled: true
`

type vmSuiteEx6 struct {
	e2e.Suite[e2e.FakeIntakeEnv]
}

func TestVMSuiteEx6(t *testing.T) {
	e2e.Run(t, &vmSuiteEx6{}, e2e.FakeIntakeStackDef(nil, agentparams.WithSystemProbeConfig(NPMsystemProbeConfig)))
}

func (v *vmSuiteEx6) Test1_FakeIntakeNPM() {
	t := v.T()

	err := backoff.Retry(func() error {

		v.Env().VM.Execute("curl http://httpbin.org/anything")

		hostnameNetID, err := v.Env().Fakeintake.GetConnectionsNames()
		if err != nil {
			return err
		}
		if len(hostnameNetID) == 0 {
			return errors.New("no connections yet")
		}

		t.Logf("hostname+networkID %v seen connections", hostnameNetID)
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(time.Second), 60))
	require.NoError(t, err)
}
