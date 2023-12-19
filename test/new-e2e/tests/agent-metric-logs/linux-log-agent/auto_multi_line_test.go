// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logagent

import (
	_ "embed"
	"fmt"
	"os"
	"testing"

	e2e "github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/params"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/require"
)

//go:embed log-config/auto-multi-line-config.yaml
var agentAutoMultiLineConfig string

//go:embed log-config/python-multi-line-script.sh
var pythonScript string

//go:embed log-config/random-logger-service.sh
var randomLogger string

type AutoMultiLineSuite struct {
	e2e.Suite[e2e.FakeIntakeEnv]
	DevMode bool
}

func TestAutoMultiLineSuite(t *testing.T) {
	s := &AutoMultiLineSuite{}
	_, s.DevMode = os.LookupEnv("TESTS_E2E_DEVMODE")
	e2e.Run(t, s, e2e.FakeIntakeStackDef(
		e2e.WithAgentParams(
			agentparams.WithLogs(),
			agentparams.WithAgentConfig(agentAutoMultiLineConfig),
			agentparams.WithIntegration("custom_logs.d", string(logConfig)))),
		params.WithDevMode())
}

func (s *AutoMultiLineSuite) TestAutoMultiLine() {
	s.Run("AutoMultiLine", s.ContainsNewLine)
}

func (s *AutoMultiLineSuite) ContainsNewLine() {
	t := s.T()
	vm := s.Env().VM

	fmt.Println(agentAutoMultiLineConfig)

	s.Env().VM.Execute("sudo touch /var/log/hello-world.log")
	s.Env().VM.Execute("sudo chmod +r /var/log/hello-world.log && echo true")

	_, err := vm.ExecuteWithError(pythonScript)
	require.NoError(t, err, "Failed to generate log generation script ")

	// Create journald log generation service
	logger_service := string(randomLogger)
	_, err = vm.ExecuteWithError(logger_service)
	require.NoError(t, err, "Failed to create journald log generation service ")
}
