// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package integrationslogs

import (
	_ "embed"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-metrics-logs/log-agent/utils"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
)

type IntegrationsLogsSuite struct {
	e2e.BaseSuite[environments.Host]
}

//go:embed fixtures/integration.py
var customIntegration string

type Config struct {
	InitConfig interface{}  `yaml:"init_config"`
	Instances  []Instance   `yaml:"instances"`
	Logs       []LogsConfig `yaml:"logs"`
}

type Instance struct {
	LogMessage      string `yaml:"log_message"`
	UniqueMessage   bool   `yaml:"unique_message"`
	LogSize         int    `yaml:"log_size"`
	LogCount        int    `yaml:"log_count"`
	IntegrationTags string `yaml:"integration_tags"`
}

type LogsConfig struct {
	Type    string `yaml:"type"`
	Source  string `yaml:"source"`
	Service string `yaml:"service"`
}

// TestLinuxFakeIntakeSuite
func TestIntegrationsLogsSuite(t *testing.T) {
	suiteParams := []e2e.SuiteOption{
		e2e.WithProvisioner(awshost.Provisioner(awshost.WithAgentOptions(
			agentparams.WithLogs(),
			agentparams.WithAgentConfig("logs_config.integrations_logs_files_max_size: 1"))))}

	suiteParams = append(suiteParams, e2e.WithDevMode())

	e2e.Run(t, &IntegrationsLogsSuite{}, suiteParams...)
}

// TestWriteTenLogsCheck ensures a check that logs are written to the file ten
// logs at a time
func (v *IntegrationsLogsSuite) TestWriteTenLogsCheck() {
	tags := []string{"foo:bar", "env:dev"}
	yamlData, err := generateYaml("Custom log message", false, 1, 10, tags, "logs_from_integrations_source", "logs_from_integrations_service")
	assert.NoError(v.T(), err)

	v.UpdateEnv(awshost.Provisioner(awshost.WithAgentOptions(
		agentparams.WithLogs(),
		agentparams.WithFile("/etc/datadog-agent/conf.d/writeTenLogs.yaml", string(yamlData), true),
		agentparams.WithFile("/etc/datadog-agent/checks.d/writeTenLogs.py", customIntegration, true))))

	logs, err := utils.FetchAndFilterLogs(v.Env().FakeIntake, "logs_from_integrations_service", "Custom log message")
	assert.Nil(v.T(), err)
	assert.GreaterOrEqual(v.T(), len(logs), 10)
}

// TestIntegrationLogFileRotation ensures logs are captured after a integration
// log file is rotated
func (v *IntegrationsLogsSuite) TestIntegrationLogFileRotation() {
	// Since it's not yet possible to write to the integration log file by calling
	// the agent check command, we can test if the file rotation works using the following method:
	// 1. Send logs until to the logs from integrations launcher until their
	// cumulative size is greater than integration_logs_files_max_size (for logs
	// of size 256kb, this will be four logs given the max size of 1mb)
	// 2. Put a counter in the logs that increases by 1 every time the check is
	// called, then check that each log's counter is 1 more than the previous log

	tags := []string{"test:rotate"}
	yamlData, err := generateYaml("a", true, 1024*230, 1, tags, "rotation_source", "rotation_service")
	assert.NoError(v.T(), err)

	v.UpdateEnv(awshost.Provisioner(awshost.WithAgentOptions(
		agentparams.WithLogs(),
		agentparams.WithFile("/etc/datadog-agent/conf.d/rotation.yaml", string(yamlData), true),
		agentparams.WithFile("/etc/datadog-agent/checks.d/rotation.py", customIntegration, true))))

	// Check each log is received and is unique
	prevLogCount := -1
	for i := 0; i < 5; i++ {
		assert.EventuallyWithT(v.T(), func(c *assert.CollectT) {
			logs, err := utils.FetchAndFilterLogs(v.Env().FakeIntake, "rotation_service", ".*counter: \\d+.*")
			assert.NoError(c, err)

			if assert.NotEmpty(c, logs) && len(logs) >= i+1 {
				log := logs[i]
				regex := regexp.MustCompile(`counter: (\d+)`)
				matches := regex.FindStringSubmatch(log.Message)
				assert.Greater(c, len(matches), 1, "Did not find count in log")

				number := matches[1]
				count, err := strconv.Atoi(number)
				assert.Nil(c, err)

				if prevLogCount != -1 {
					assert.Equal(c, prevLogCount+1, count)
				}

				prevLogCount = count
			}
		}, 2*time.Minute, 5*time.Second)
	}
}

// generateYaml Generates a YAML config for checks to use
func generateYaml(logMessage string, uniqueMessage bool, logSize int, logCount int, integrationTags []string, logSource string, logService string) ([]byte, error) {
	// Define the YAML structure
	config := Config{
		InitConfig: nil,
		Instances: []Instance{
			{
				LogMessage:      logMessage,
				UniqueMessage:   uniqueMessage,
				LogSize:         logSize,
				LogCount:        logCount,
				IntegrationTags: strings.Join(integrationTags, ","),
			},
		},
		Logs: []LogsConfig{
			{
				Type:    "integration",
				Source:  logSource,
				Service: logService,
			},
		},
	}

	return yaml.Marshal(&config)
}
