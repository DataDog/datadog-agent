// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package opw contains E2E tests for the Agent's Observability Pipelines Worker
// (OPW) log forwarder path, in particular the
// `observability_pipelines_worker.logs.send_host_tags` feature.
package opw

import (
	_ "embed"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	fi "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

//go:embed fixtures/custom_logs.yaml
var customLogsConfig string

const (
	logFolder    = "/var/log/e2e_test_logs"
	logFilePath  = logFolder + "/opw-host-tags.log"
	serviceName  = "opw-e2e"
	hostTag      = "custom_host_tag:e2e_test"
	waitDuration = 2 * time.Minute
	tickInterval = 10 * time.Second
)

type opwHostTagsSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestOPWHostTagsSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &opwHostTagsSuite{}, e2e.WithProvisioner(awshost.Provisioner()))
}

// opwAgentConfig renders the Agent YAML config that points the OPW logs URL at
// the running fakeintake. sendHostTags toggles the feature under test.
func opwAgentConfig(fakeintakeURL string, sendHostTags bool) string {
	return fmt.Sprintf(`logs_enabled: true
observability_pipelines_worker:
  logs:
    enabled: true
    url: %q
    send_host_tags: %t
tags:
  - %q
`, fakeintakeURL, sendHostTags, hostTag)
}

func (s *opwHostTagsSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())
	s.Env().RemoteHost.MustExecute("sudo mkdir -p " + logFolder)
	s.Env().RemoteHost.MustExecute("sudo rm -f " + logFilePath)
	s.Env().RemoteHost.MustExecute("sudo touch " + logFilePath)
	s.Env().RemoteHost.MustExecute("sudo chmod 644 " + logFilePath)
}

func (s *opwHostTagsSuite) provisionWithOPW(sendHostTags bool) {
	cfg := opwAgentConfig(s.Env().FakeIntake.URL, sendHostTags)
	s.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(scenec2.WithAgentOptions(
			agentparams.WithAgentConfig(cfg),
			agentparams.WithIntegration("custom_logs.d", customLogsConfig),
		)),
	))
}

// appendLog writes a newline-terminated log line to the tailed file. The file
// must already exist with read permissions; BeforeTest handles creation.
func (s *opwHostTagsSuite) appendLog(content string) {
	s.T().Helper()
	cmd := fmt.Sprintf("echo %q | sudo tee -a %s >/dev/null", content, logFilePath)
	s.Env().RemoteHost.MustExecute(cmd)
}

// TestSendHostTagsEnabled asserts that with send_host_tags=true the forwarded
// log payload carries the user-configured host tag in ddtags.
func (s *opwHostTagsSuite) TestSendHostTagsEnabled() {
	s.provisionWithOPW(true)

	content := "opw-send-host-tags-enabled"
	s.appendLog(content)

	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := s.Env().FakeIntake.Client().FilterLogs(serviceName, fi.WithMessageContaining(content))
		require.NoError(c, err)
		require.NotEmpty(c, logs, "expected at least one log for service %q containing %q", serviceName, content)
		assert.Contains(c, logs[0].Tags, hostTag, "expected host tag %q in ddtags; got %v", hostTag, logs[0].Tags)
	}, waitDuration, tickInterval)
}

// TestSendHostTagsDisabled is the negative case: with send_host_tags=false the
// user-configured host tag must NOT appear in ddtags on OPW-bound payloads.
func (s *opwHostTagsSuite) TestSendHostTagsDisabled() {
	s.provisionWithOPW(false)

	content := "opw-send-host-tags-disabled"
	s.appendLog(content)

	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := s.Env().FakeIntake.Client().FilterLogs(serviceName, fi.WithMessageContaining(content))
		require.NoError(c, err)
		require.NotEmpty(c, logs, "expected log for service %q containing %q", serviceName, content)
		for i, l := range logs {
			assert.NotContains(s.T(), l.Tags, hostTag,
				"log[%d] unexpectedly carried %q; tags=%v", i, hostTag, l.Tags)
		}
	}, waitDuration, tickInterval)
}
