// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package linuxfiletailing

import (
	_ "embed"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"

	fi "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-metric-logs/log-agent/utils"
)

//go:embed log-config/automulti.yaml
var agentAutoMultiLineConfig string

const singleLineLog = "This is a single line log"
const multiLineLog = "This is\na multi\nline log"

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
	s.Run("AutoMultiLine", s.ContainsLogWithNewLines)
}

func (s *AutoMultiLineSuite) generateMultilineLogs() {
	s.T().Helper()
	var message string

	// auto_multi_line_detection uses the first 500 logs to detect a pattern
	// Alternate between sending single line logs and multi line logs
	for i := 0; i < 700; i++ {
		timestamp := time.Now().Format(time.RFC3339)
		if i%2 == 0 {
			message = fmt.Sprintf("%s | %s", timestamp, singleLineLog)
		} else {
			message = fmt.Sprintf("%s | %s", timestamp, multiLineLog)
		}
		cmd := fmt.Sprintf("echo '%s' | sudo tee -a %s", message, logFilePath)
		s.Env().RemoteHost.MustExecute(cmd)
	}
}

func (s *AutoMultiLineSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)

	// Create a new log folder location
	s.Env().RemoteHost.MustExecute(fmt.Sprintf("sudo mkdir -p %s", utils.LinuxLogsFolderPath))

	s.Env().RemoteHost.Execute(fmt.Sprintf("sudo touch %s", logFilePath))
	s.Env().RemoteHost.Execute(fmt.Sprintf("sudo chmod +r %s", logFilePath))

	s.generateMultilineLogs()
}

func (s *AutoMultiLineSuite) AfterTest(suiteName, testName string) {
	s.BaseSuite.AfterTest(suiteName, testName)

	s.Env().RemoteHost.Execute(fmt.Sprintf("sudo rm -rf %s", utils.LinuxLogsFolderPath))
}

func (s *AutoMultiLineSuite) ContainsLogWithNewLines() {
	client := s.Env().FakeIntake.Client()

	// Raw string since '\n' literal will be in the log.Message
	content := `This is\na multi\nline log`
	service := "hello"

	s.EventuallyWithT(func(c *assert.CollectT) {
		names, err := client.GetLogServiceNames()
		if !assert.NoErrorf(c, err, "Error found: %s", err) {
			return
		}

		// Auto Multiline is working if the log message contains the complete log contents with newlines
		logs, err := client.FilterLogs(service, fi.WithMessageContaining(content))
		if !assert.NoErrorf(c, err, "Error found: %s", err) {
			return
		}

		assert.NotEmpty(c, logs, "Expected at least 1 log with content: '%s', from service: %s but received %s logs.", content, names, logs)
	}, 2*time.Minute, 10*time.Second)
}
