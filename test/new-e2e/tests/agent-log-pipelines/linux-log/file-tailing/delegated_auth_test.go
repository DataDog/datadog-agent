// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package linuxfiletailing

import (
	"bytes"
	_ "embed"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-log-pipelines/utils"
)

const (
	delegatedAuthAPIHost = "api.datadoghq.com"
	delegatedLogsHost    = "agent-http-intake.logs.datadoghq.com"
	delegatedLogFile     = "dela-dual-shipping.log"
	delegatedLogMessage  = "delegated-auth-log-delivered"
	delegatedServerPath  = "/tmp/dela_auth_server.py"
	delegatedLogConfig   = `logs:
  - type: file
    path: /tmp/logs/dela-dual-shipping.log
    service: dela-dual-shipping`
)

//go:embed fixtures/delegated_auth_server.py
var delegatedAuthServer string

var safeHostname = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]*$`)

type delegatedAuthDualShippingSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestDelegatedAuthDualShipping(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &delegatedAuthDualShippingSuite{}, e2e.WithProvisioner(delegatedAuthProvisioner("")))
}

func delegatedAuthProvisioner(agentConfig string) provisioners.TypedProvisioner[environments.Host] {
	options := []func(*agentparams.Params) error{
		agentparams.WithLogs(),
		agentparams.WithIntegration("custom_logs.d", delegatedLogConfig),
		agentparams.WithFile(delegatedServerPath, delegatedAuthServer, false),
	}
	if agentConfig != "" {
		options = append(options, agentparams.WithAgentConfig(agentConfig))
	}
	return awshost.Provisioner(awshost.WithRunOptions(scenec2.WithAgentOptions(options...)))
}

func (s *delegatedAuthDualShippingSuite) TearDownSuite() {
	defer s.BaseSuite.TearDownSuite()

	_, stopErr := s.Env().RemoteHost.Execute("sudo service datadog-agent stop")
	s.Env().RemoteHost.Execute("test ! -f /tmp/dela-auth-server.pid || sudo kill $(cat /tmp/dela-auth-server.pid)")
	if assert.NoError(s.T(), stopErr, "stop the Agent before removing the delegated auth hostname pin") {
		s.Env().RemoteHost.Execute("sudo sed -i '/# dela-dual-shipping-e2e$/d' /etc/hosts")
	}
}

func (s *delegatedAuthDualShippingSuite) TestResolvedCredentialShipsLogs() {
	t := s.T()
	fakeIntakeURL, err := url.Parse(s.Env().FakeIntake.Client().URL())
	require.NoError(t, err)
	require.NotEmpty(t, fakeIntakeURL.Hostname())

	port := fakeIntakeURL.Port()
	if port == "" {
		if fakeIntakeURL.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	useSSL := fakeIntakeURL.Scheme == "https"

	s.setupDelegatedAuthServer(fakeIntakeURL.Hostname())
	config := fmt.Sprintf(`skip_ssl_validation: true
delegated_auth:
  startup_timeout_secs: 30
logs_config:
  use_http: true
  force_use_http: true
  additional_endpoints:
    - api_key: "DELA(00000000-0000-0000-0000-000000000001, aws, region=us-east-1)"
      host: %s
      port: %s
      use_ssl: %t
      use_compression: false
`, delegatedLogsHost, port, useSSL)
	s.UpdateEnv(delegatedAuthProvisioner(config))
	require.NoError(t, s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	logPath := utils.LinuxLogsFolderPath + "/" + delegatedLogFile
	s.Env().RemoteHost.MustExecute("sudo mkdir -p " + utils.LinuxLogsFolderPath)
	s.Env().RemoteHost.MustExecute("sudo touch " + logPath)
	s.Env().RemoteHost.MustExecute("sudo chmod +r " + logPath)
	utils.AppendLog(s, delegatedLogFile, delegatedLogMessage, 1)

	s.EventuallyWithT(func(c *assert.CollectT) {
		exchange, exchangeErr := s.Env().RemoteHost.Execute("test -s /tmp/dela-auth-requests.log")
		assert.NoError(c, exchangeErr, "delegated credential exchange did not reach the local test server: %s", exchange)

		payloads, payloadsErr := s.Env().FakeIntake.Client().GetRawPayloads("/api/v2/logs")
		require.NoError(c, payloadsErr)
		matched := false
		for _, payload := range payloads {
			if payload.APIKey == "dela-e2e-api-key" && bytes.Contains(payload.Data, []byte(delegatedLogMessage)) {
				matched = true
				break
			}
		}
		assert.True(c, matched, "resolved delegated credential did not deliver logs")
	}, 5*time.Minute, 10*time.Second)
}

func (s *delegatedAuthDualShippingSuite) setupDelegatedAuthServer(fakeIntakeHost string) {
	host := s.Env().RemoteHost
	require.True(s.T(), safeHostname.MatchString(fakeIntakeHost), "unsafe Fake Intake hostname %q", fakeIntakeHost)
	fakeIntakeIP := strings.TrimSpace(host.MustExecute(fmt.Sprintf("getent ahostsv4 %s | awk 'NR == 1 { print $1 }'", fakeIntakeHost)))
	require.NotNil(s.T(), net.ParseIP(fakeIntakeIP), "invalid Fake Intake address %q", fakeIntakeIP)
	host.MustExecute(fmt.Sprintf("grep -Fqx '127.0.0.1 %s # dela-dual-shipping-e2e' /etc/hosts || echo '127.0.0.1 %s # dela-dual-shipping-e2e' | sudo tee -a /etc/hosts", delegatedAuthAPIHost, delegatedAuthAPIHost))
	host.MustExecute(fmt.Sprintf("grep -Fqx '%s %s # dela-dual-shipping-e2e' /etc/hosts || echo '%s %s # dela-dual-shipping-e2e' | sudo tee -a /etc/hosts", fakeIntakeIP, delegatedLogsHost, fakeIntakeIP, delegatedLogsHost))
	host.MustExecute("sudo openssl req -x509 -newkey rsa:2048 -nodes -keyout /tmp/dela-auth.key -out /tmp/dela-auth.crt -days 1 -subj /CN=api.datadoghq.com")
	host.MustExecute("test ! -f /tmp/dela-auth-server.pid || sudo kill $(cat /tmp/dela-auth-server.pid) || true")
	host.MustExecute("sudo rm -f /tmp/dela-auth-requests.log")
	host.MustExecute("sudo sh -c 'nohup python3 " + delegatedServerPath + " >/tmp/dela-auth-server.log 2>&1 & echo $! >/tmp/dela-auth-server.pid'")

	s.EventuallyWithT(func(c *assert.CollectT) {
		_, err := host.Execute("curl -ksS -o /dev/null https://api.datadoghq.com/api/v2/intake-key -X POST")
		assert.NoError(c, err)
	}, time.Minute, 2*time.Second)
	host.MustExecute("sudo rm -f /tmp/dela-auth-requests.log")
}
