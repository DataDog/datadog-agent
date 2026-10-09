// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package expectedtags

import (
	_ "embed"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-log-pipelines/utils"
)

//go:embed config/config.yaml
var logConfig string

const (
	hostTag               = "e2e_expected_tags:present"
	infrastructureModeTag = "infra_mode:cloud_cost_only"

	logFileName = "expected-tags.log"
	logFilePath = utils.LinuxLogsFolderPath + "/" + logFileName
	service     = "expected-tags"
	logContent  = "expected-tags-ccm"
)

const agentConfig = `
logs_config:
  expected_tags_duration: 30m
infrastructure_mode: cloud_cost_only
tags:
  - ` + hostTag + `
`

type expectedTagsCCMSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestExpectedTagsCCMLinux(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &expectedTagsCCMSuite{},
		e2e.WithProvisioner(
			awshost.Provisioner(
				awshost.WithRunOptions(
					scenec2.WithAgentOptions(
						agentparams.WithLogs(),
						agentparams.WithAgentConfig(agentConfig),
						agentparams.WithIntegration("custom_logs.d", logConfig),
					),
				),
			),
		),
		e2e.WithStackName("log-expectedtags-ccm"),
	)
}

func (s *expectedTagsCCMSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)
	utils.CleanUp(s)
	s.Env().RemoteHost.MustExecute("sudo mkdir -p " + utils.LinuxLogsFolderPath)
}

func (s *expectedTagsCCMSuite) TearDownSuite() {
	utils.CleanUp(s)
	s.BaseSuite.TearDownSuite()
}

func (s *expectedTagsCCMSuite) TestLogExpectedTagsWithoutInfraMode() {
	t := s.T()

	s.Env().RemoteHost.MustExecute("sudo touch " + logFilePath)
	output, err := s.Env().RemoteHost.Execute(fmt.Sprintf("sudo chmod +r %s && echo true", logFilePath))
	require.NoError(t, err)
	require.Equal(t, "true", strings.TrimSpace(output))

	utils.AssertAgentTailerOK(s, logFileName)
	utils.AppendLog(s, logFileName, logContent, 1)

	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := utils.FetchAndFilterLogs(s.Env().FakeIntake, service, logContent)
		require.NoError(c, err)
		require.NotEmpty(c, logs, "no logs for service %q with content %q yet", service, logContent)

		log := logs[0]
		assert.Contains(c, log.Tags, hostTag, "expected host tag from logs expected-tags window")
		assert.NotContains(c, log.Tags, infrastructureModeTag,
			"infra_mode must not ride onto logs via expected tags")
	}, 5*time.Minute, 10*time.Second)
}
