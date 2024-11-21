// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package integrationslogs

import (
	_ "embed"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-metrics-logs/log-agent/utils"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
)

type IntegrationsLogsSuite struct {
	e2e.BaseSuite[environments.Host]
}

//go:embed fixtures/tenLogs.py
var writeTenLogsCheck string

//go:embed fixtures/tenLogs.yaml
var writeTenLogsConfig string

//go:embed fixtures/rotation.py
var rotationCheck string

//go:embed fixtures/rotation.yaml
var rotationConfig string

// TestLinuxFakeIntakeSuite
func TestIntegrationsLogsSuite(t *testing.T) {
	suiteParams := []e2e.SuiteOption{
		e2e.WithProvisioner(awshost.Provisioner(awshost.WithAgentOptions(
			agentparams.WithLogs(),
			agentparams.WithAgentConfig("logs_config.integrations_logs_files_max_size: 1"),
			agentparams.WithFile("/etc/datadog-agent/checks.d/writeTenLogs.py", writeTenLogsCheck, true),
			agentparams.WithFile("/etc/datadog-agent/conf.d/writeTenLogs.yaml", writeTenLogsConfig, true),
			agentparams.WithFile("/etc/datadog-agent/checks.d/rotation.py", rotationCheck, true),
			agentparams.WithFile("/etc/datadog-agent/conf.d/rotation.yaml", rotationConfig, true))))}

	suiteParams = append(suiteParams, e2e.WithDevMode())

	e2e.Run(t, &IntegrationsLogsSuite{}, suiteParams...)
}

// TestWriteTenLogsCheck ensures a check that logs are written to the file ten
// logs at a time
func (v *IntegrationsLogsSuite) TestWriteTenLogsCheck() {
	utils.CheckLogsExpected(v.T(), v.Env().FakeIntake, "ten_logs_service", "Custom log message", []string{"env:dev", "bar:foo"})
}

// TestIntegrationLogFileRotation ensures logs are captured after a integration
// log file is rotated
func (v *IntegrationsLogsSuite) TestIntegrationLogFileRotation() {
	// Since it's not yet possible to write to the integration log file by calling
	// the agent check command, we can test if the file rotation works using the following method:
	// 1. Send logs until to the logs from integrations launcher until their
	// cumulative size is greater than integration_logs_files_max_size (for logs
	// of size 256kb, this will be four logs given the max size of 1mb)
	// 2. Check that the logs received from fakeIntake are unique by checking the
	// UUID and at the same time ensure monotonic_count is equal to 1 (indicating
	// a 1:1 correlation between a log and metric)

	seen := make(map[string]bool)

	// Check each log is received and is unique
	for i := 0; i < 5; i++ {
		assert.EventuallyWithT(v.T(), func(c *assert.CollectT) {
			logs, err := utils.FetchAndFilterLogs(v.Env().FakeIntake, "rotation_service", ".*message.*")
			assert.NoError(c, err)

			if assert.NotEmpty(c, logs) {
				log := logs[i]
				// Take the first 48 characters of the log, this part contains the UUID
				logID := log.Message[:48]
				assert.False(v.T(), seen[logID])
				seen[logID] = true
			}

			// Check each log received has increased the monotonic counter by 1
			metrics, err := v.Env().FakeIntake.Client().FilterMetrics("rotate_logs_sent")
			assert.NoError(v.T(), err)
			points := metrics[len(metrics)-1].Points
			point := metrics[len(metrics)-1].Points[len(points)-1].Value
			assert.Equal(v.T(), 1.0, point)
		}, 2*time.Minute, 5*time.Second)

	}
}
