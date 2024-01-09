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
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fi "github.com/DataDog/datadog-agent/test/fakeintake/client"
	e2e "github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/params"
)

//go:embed log-config/auto-multi-line-config.yaml
var agentAutoMultiLineConfig string

//go:embed scripts/python-multi-line-script.sh
var pythonScript string

//go:embed scripts/random-logger-service.sh
var randomLogger string

//go:embed log-config/config.yaml
var logConfig string

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

	// Create multi-line log generation service
	logger_service := string(randomLogger)
	_, err = vm.ExecuteWithError(logger_service)
	require.NoError(t, err, "Failed to create multi-line log generation service ")

	_, err = vm.ExecuteWithError("sudo systemctl daemon-reload")
	require.NoError(t, err, "Failed to reload service")

	// Start multi-linelog generation service
	_, err = vm.ExecuteWithError("sudo systemctl enable --now random-logger.service")
	require.NoError(t, err, "Failed to enable service")

	// Restart agent
	_, err = vm.ExecuteWithError("sudo service datadog-agent restart")
	require.NoError(t, err, "Failed to restart the agent")

	client := s.Env().Fakeintake
	t.Helper()
	service := "hello"
	content := `An error is \nusually an exception that \nhas been caught and not handled.`
	s.EventuallyWithT(func(c *assert.CollectT) {
		names, err := client.GetLogServiceNames()
		if !assert.NoErrorf(c, err, "Error found: %s", err) {
			return
		}

		logs, err := client.FilterLogs(service)
		if !assert.NotEmpty(c, logs, "No logs with service matching '%s' found, instead got '%s'", service, names) {
			return
		}

		logs, err = client.FilterLogs(service, fi.WithMessageContaining(content))
		if !assert.NoErrorf(c, err, "Error found: %s", err) {
			return
		}
		assert.NotEmpty(c, logs, "Expected at least 1 log with content: '%s', from service: %s but received %s logs.", content, names, logs)
	}, 4*time.Minute, 10*time.Second)

	// utils.CheckLogs(s)
}

// logsToString converts a slice of logs to a string.
func logsToString(logs []*aggregator.Log) []string {
	var logsStrings []string
	for _, log := range logs {
		logsStrings = append(logsStrings, log.Message)
	}
	return logsStrings
}
