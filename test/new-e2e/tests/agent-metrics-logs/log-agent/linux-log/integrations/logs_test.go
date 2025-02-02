// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package integrationslogs

import (
	_ "embed"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
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

	// 1. Set the max log file size to 1 MB and individual log size to a size
	// large enough to cause rotations every 4 logs. 255 KB was chosen since the
	// log is JSON formatted and includes information such as tags, service,
	// source, and other information in the log.
	// 2. Send five (or more) logs to the agent, causing the log file to rotate.
	// 3. Check the logs to ensure that each is unique, ensuring the rotation worked correctly.

	tags := []string{"test:rotate"}
	serviceString := fmt.Sprintf("%s_%s", uuid.NewString(), time.Now().String())
	yamlData, err := generateYaml("a", true, 1024*255, 1, tags, "rotation_source", serviceString)
	assert.NoError(v.T(), err)

	v.UpdateEnv(awshost.Provisioner(awshost.WithAgentOptions(
		agentparams.WithLogs(),
		agentparams.WithFile("/etc/datadog-agent/conf.d/rotation.yaml", string(yamlData), true),
		agentparams.WithFile("/etc/datadog-agent/checks.d/rotation.py", customIntegration, true))))

	// The log file should rotate every four logs, so checking through five
	// iterations should guarantee a rotation. The counters in the logs should
	// also contain the numbers 1 through 5

	// Accumulate logs until there are at least 5
	var receivedLogs []*aggregator.Log
	assert.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		receivedLogs, err = utils.FetchAndFilterLogs(v.Env().FakeIntake, serviceString, ".*counter: \\d+.*")
		assert.NoError(c, err)
		assert.GreaterOrEqual(c, len(receivedLogs), 5)

	}, 2*time.Minute, 5*time.Second)

	// Check the logs to ensure they're unique and rotation worked correctly

	// Sort the logs slice in ascending order according to timestamp. This
	// guarantees that the last written log will be in last position in the
	// slice. This is needed since FetchAndFilterLogs doesn't guarantee order
	sort.Slice(receivedLogs, func(j, k int) bool {
		return receivedLogs[j].Timestamp < receivedLogs[k].Timestamp
	})

	// Extract the count from the first log
	compareCount := extractNumberFromLog(v.T(), receivedLogs[0])

	// Ensure numbers from subsequent logs are increasing
	for i := 1; i < 5; i++ {
		compareCount++
		currentLogCount := extractNumberFromLog(v.T(), receivedLogs[i])
		assert.Equal(v.T(), compareCount, currentLogCount)
	}
}

// extractNumberFromLog extracts the a number from the log message
func extractNumberFromLog(t *testing.T, log *aggregator.Log) int {
	regex := regexp.MustCompile(`counter: (\d+)`)
	matches := regex.FindStringSubmatch(log.Message)
	assert.Greater(t, len(matches), 1, "Did not find matching \"count\" regular expression in log")
	number := matches[1]
	count, err := strconv.Atoi(number)
	assert.Nil(t, err)
	return count
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
