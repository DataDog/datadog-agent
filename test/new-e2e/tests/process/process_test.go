// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package process

import (
	_ "embed"
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
)

//go:embed config/process_check.yaml
var configStr string

type processTestSuite struct {
	e2e.Suite[e2e.FakeIntakeEnv]
}

func TestProcessTestSuite(t *testing.T) {
	e2e.Run(t, &processTestSuite{},
		e2e.FakeIntakeStackDef(e2e.WithAgentParams(agentparams.WithAgentConfig(configStr))))
}

func (s *processTestSuite) TestProcessCheck() {
	fakeIntake := s.Env().Fakeintake

	s.EventuallyWithT(func(c *assert.CollectT) {
		payloads, err := fakeIntake.GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")
		assert.NotEmpty(c, payloads, "no process payloads returned")

		for _, payload := range payloads {
			assert.NotEmpty(c, payload.Processes, "no processes in payload")
		}
	}, 2*time.Minute, 10*time.Second)
}
