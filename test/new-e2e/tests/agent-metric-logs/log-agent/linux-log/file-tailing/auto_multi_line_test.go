// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filetailing

import (
	_ "embed"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fi "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
)

//go:embed log-config/auto-multi-line-config.yaml
var agentAutoMultiLineConfig string

//go:embed scripts/python-multi-line-script.sh
var pythonScript string

//go:embed scripts/random-logger-service.sh
var randomLogger string

type AutoMultiLineSuite struct {
	e2e.BaseSuite[environments.Host]
	DevMode bool
}

func TestAutoMultiLineSuite(t *testing.T) {
	s := &AutoMultiLineSuite{}
	devModeEnv, _ := os.LookupEnv("E2E_DEVMODE")
	options := []e2e.SuiteOption{
		e2e.WithProvisioner(awshost.Provisioner(awshost.WithAgentOptions(agentparams.WithLogs(), agentparams.WithAgentConfig(agentAutoMultiLineConfig), agentparams.WithIntegration("custom_logs.d", logConfig)))),
	}
	if devMode, err := strconv.ParseBool(devModeEnv); err == nil && devMode {
		options = append(options, e2e.WithDevMode())
	}

	e2e.Run(t, s, options...)
}

func (s *AutoMultiLineSuite) TestAutoMultiLine() {
	s.Run("AutoMultiLine", s.ContainsNewLine)
}

func (s *AutoMultiLineSuite) ContainsNewLine() {
	t := s.T()

	fmt.Println(agentAutoMultiLineConfig)

	s.Env().RemoteHost.Execute("sudo touch /var/log/hello-world.log")
	s.Env().RemoteHost.Execute("sudo chmod +r /var/log/hello-world.log && echo true")

	_, err := s.Env().RemoteHost.Execute(pythonScript)
	require.NoError(t, err, "Failed to generate log generation script ")

	// Create multi-line log generation service
	logger_service := string(randomLogger)
	_, err = s.Env().RemoteHost.Execute(logger_service)
	require.NoError(t, err, "Failed to create multi-line log generation service ")

	_, err = s.Env().RemoteHost.Execute("sudo systemctl daemon-reload")
	require.NoError(t, err, "Failed to reload service")

	// Start multi-linelog generation service
	_, err = s.Env().RemoteHost.Execute("sudo systemctl enable --now random-logger.service")
	require.NoError(t, err, "Failed to enable service")

	// Restart agent
	_, err = s.Env().RemoteHost.Execute("sudo service datadog-agent restart")
	require.NoError(t, err, "Failed to restart the agent")

	client := s.Env().FakeIntake.Client()
	t.Helper()
	service := "hello"
	content := `An error is\nusually an exception that\nhas been caught and not handled.`

	s.EventuallyWithT(func(c *assert.CollectT) {
		names, err := client.GetLogServiceNames()
		if !assert.NoErrorf(c, err, "Error found: %s", err) {
			return
		}

		logs, err := client.FilterLogs(service, fi.WithMessageContaining(content))
		if !assert.NoErrorf(c, err, "Error found: %s", err) {
			return
		}
		assert.NotEmpty(c, logs, "Expected at least 1 log with content: '%s', from service: %s but received %s logs.", content, names, logs)
	}, 2*time.Minute, 10*time.Second)

	fmt.Println("done!")
}
