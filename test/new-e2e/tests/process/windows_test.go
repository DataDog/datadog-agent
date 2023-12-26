// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package process

import (
	"testing"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/params"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
)

type windowsTestSuite struct {
	e2e.Suite[e2e.FakeIntakeEnv]
}

func TestWindowsTestSuite(t *testing.T) {
	e2e.Run(t, &windowsTestSuite{},
		e2e.FakeIntakeStackDef(
			e2e.WithAgentParams(agentparams.WithAgentConfig(processCheckConfigStr)),
		))
}

func (s *linuxTestSuite) SetupSuite() {
	s.Suite.SetupSuite()
}
