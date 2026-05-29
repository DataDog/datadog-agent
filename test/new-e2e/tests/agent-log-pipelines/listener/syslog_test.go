// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package listener

import (
	_ "embed"
	"encoding/json"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-log-pipelines/utils"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/docker"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
)

//go:embed testdata/syslog-tcp-compose.yaml
var syslogTCPCompose string

type dockerSyslogSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

func TestSyslogTCPListener(t *testing.T) {
	e2e.Run(t,
		&dockerSyslogSuite{},
		e2e.WithProvisioner(
			awsdocker.Provisioner(
				awsdocker.WithRunOptions(
					ec2docker.WithAgentOptions(
						dockeragentparams.WithLogs(),
						dockeragentparams.WithExtraComposeManifest("syslog-tcp", pulumi.String(syslogTCPCompose)),
					))),
		))
}

func (s *dockerSyslogSuite) TestSyslogStructuredOutput() {
	t := s.T()

	s.EventuallyWithT(func(c *assert.CollectT) {
		agentReady := s.Env().Agent.Client.IsReady()
		assert.True(c, agentReady)
	}, 1*time.Minute, 5*time.Second, "Agent was not ready")

	t.Logf("Testing Agent Version '%v'", s.Env().Agent.Client.Version())

	// Send a single RFC 5424 syslog message via the BusyBox sender container.
	// PRI 134 = facility 16 (local0) * 8 + severity 6 (info).
	// -w 1: BusyBox nc won't exit after stdin EOF because the agent holds
	// the TCP connection open; the timeout ensures nc exits promptly.
	sendCmd := []string{
		"sh", "-c",
		`printf '<134>1 2025-06-15T10:30:00Z testhost testapp 1234 - - Syslog e2e test message\n' | nc -w 1 agent 10514`,
	}

	// The AD-discovered TCP listener may not be accepting connections yet
	// even though the agent reports ready. Retry the send until it succeeds.
	s.EventuallyWithT(func(c *assert.CollectT) {
		stdout, stderr, err := s.Env().Docker.Client.ExecuteCommandStdoutStdErr("syslog-sender", sendCmd...)
		assert.NoError(c, err, "failed to send syslog message: stdout=%s stderr=%s", stdout, stderr)
	}, 1*time.Minute, 5*time.Second, "syslog send never succeeded")

	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := utils.FetchAndFilterLogs(s.Env().FakeIntake, "syslog-e2e", "Syslog e2e test message")
		if !assert.NoError(c, err) {
			return
		}
		if !assert.NotEmpty(c, logs, "no logs matching 'Syslog e2e test message' found") {
			return
		}
		t.Logf("matched %d logs in fakeintake for 'Syslog e2e test message'", len(logs))

		log := logs[0]

		var body map[string]interface{}
		if !assert.NoError(c, json.Unmarshal([]byte(log.Message), &body), "log.Message is not valid JSON: %s", log.Message) {
			return
		}

		assert.Equal(c, "Syslog e2e test message", body["message"], "unexpected message body")
		if !assert.Contains(c, body, "syslog", "missing 'syslog' key in structured output") {
			return
		}

		syslogMap, ok := body["syslog"].(map[string]interface{})
		if !assert.True(c, ok, "'syslog' value is not a map: %T", body["syslog"]) {
			return
		}
		assert.Equal(c, "testapp", syslogMap["appname"], "unexpected appname")
		assert.EqualValues(c, 6, syslogMap["severity"], "unexpected severity")
		assert.EqualValues(c, 16, syslogMap["facility"], "unexpected facility")

		assert.Equal(c, "info", log.Status, "unexpected log status")
		assert.Equal(c, "syslog-e2e", log.Source, "unexpected log source")
	}, 2*time.Minute, 10*time.Second)
}
