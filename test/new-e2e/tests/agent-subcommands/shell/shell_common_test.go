// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package shell

import (
	"strings"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
)

type baseShellSuite struct {
	e2e.BaseSuite[environments.Host]
}

func (s *baseShellSuite) TestShellEcho() {
	output := s.Env().Agent.Client.Shell(agentclient.WithArgs([]string{"-c", "echo hi"}))
	assert.Equal(s.T(), "hi", strings.TrimSpace(output))
}
