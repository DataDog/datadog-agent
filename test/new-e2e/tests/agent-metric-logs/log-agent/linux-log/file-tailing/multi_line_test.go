// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package linuxfiletailing

import (
	_ "embed"
	"fmt"
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
var autoMultiLineConfig string

//go:embed log-config/pattern.yaml
var multiLineLogPatternConfig string

const singleLineLog = "This is a single line log"
const multiLineLog = "This is a multi\nline log"

// MultiLineSuite defines a test suite for testing the log agents
// auto_multi_line_detection and multi_line features
type MultiLineSuite struct {
	e2e.BaseSuite[environments.Host]
	DevMode bool
}

func TestMultiLineSuite(t *testing.T) {
	s := &MultiLineSuite{}
	options := []e2e.SuiteOption{
		e2e.WithProvisioner(awshost.Provisioner(awshost.WithAgentOptions(agentparams.WithLogs()))),
	}

	e2e.Run(t, s, options...)
}

func (s *MultiLineSuite) TestMultiLine() {
	s.Run("MultiLinePattern", s.detectsPatternMultiLine)
	s.Run("AutoMultiLine", s.detectsAutoMultiLine)
}

func (s *MultiLineSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)

	// flush intake
	s.EventuallyWithT(func(c *assert.CollectT) {
		err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
		if assert.NoErrorf(c, err, "Having issues flushing server and resetting aggregators, retrying...") {
			s.T().Log("Successfully flushed server and reset aggregators.")
		}
	}, 1*time.Minute, 10*time.Second)

	// Create a new log folder location
	s.Env().RemoteHost.MustExecute(fmt.Sprintf("sudo mkdir -p %s", utils.LinuxLogsFolderPath))

	s.Env().RemoteHost.Execute(fmt.Sprintf("sudo touch %s", logFilePath))
	s.Env().RemoteHost.Execute(fmt.Sprintf("sudo chmod +r %s", logFilePath))

}

func (s *MultiLineSuite) AfterTest(suiteName, testName string) {
	s.BaseSuite.AfterTest(suiteName, testName)

	s.Env().RemoteHost.Execute(fmt.Sprintf("sudo rm -rf %s", utils.LinuxLogsFolderPath))
}

func (s *MultiLineSuite) generateMultilineLogs(prefix string, count int) {
	s.T().Helper()
	var message string

	// Alternate between sending single line logs and multi line logs
	for i := 0; i < count; i++ {
		if i%2 == 0 {
			message = fmt.Sprintf("%s | %s", prefix, singleLineLog)
		} else {
			message = fmt.Sprintf("%s | %s", prefix, multiLineLog)
		}
		s.Env().RemoteHost.AppendFile("linux", logFilePath, []byte(message))
	}
}

func (s *MultiLineSuite) detectsAutoMultiLine() {
	agentOptions := []agentparams.Option{
		agentparams.WithLogs(),
		agentparams.WithIntegration("custom_logs.d", autoMultiLineConfig),
	}
	// Update env to enable auto_multi_line_detection
	s.UpdateEnv(awshost.Provisioner(awshost.WithAgentOptions(agentOptions...)))

	// auto_multi_line_detection uses the first 500 logs to detect a pattern
	s.generateMultilineLogs(time.Now().Format(time.RFC3339), 700)

	client := s.Env().FakeIntake.Client()

	// Raw string since '\n' literal will be in the log.Message
	content := `This is a multi\nline log`
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

func (s *MultiLineSuite) detectsPatternMultiLine() {
	agentOptions := []agentparams.Option{
		agentparams.WithLogs(),
		agentparams.WithIntegration("custom_logs.d", multiLineLogPatternConfig),
	}
	// Update env to add log_processing_rules with multi_line pattern
	s.UpdateEnv(awshost.Provisioner(awshost.WithAgentOptions(agentOptions...)))

	s.generateMultilineLogs("fake-log-prefix", 100)

	client := s.Env().FakeIntake.Client()

	// regex pattern to ensure that the log consists entirely of only one multi line log
	content := `^fake-log-prefix \| This is a multi\\nline log$`
	service := "hello"

	s.EventuallyWithT(func(c *assert.CollectT) {
		names, err := client.GetLogServiceNames()
		if !assert.NoErrorf(c, err, "Error found: %s", err) {
			return
		}

		// multi_line pattern is working if the log message contains the complete log contents with newlines
		logs, err := client.FilterLogs(service, fi.WithMessageMatching(content))
		if !assert.NoErrorf(c, err, "Error found: %s", err) {
			return
		}

		assert.NotEmpty(c, logs, "Expected at least 1 log with content: '%s', from service: %s but received %s logs.", content, names, logs)
	}, 2*time.Minute, 10*time.Second)
}
