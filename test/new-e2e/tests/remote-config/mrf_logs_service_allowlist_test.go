// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	fakeintakecomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

const (
	agentFailoverProduct = "AGENT_FAILOVER"
	// The two AGENT_FAILOVER configs published in production: the DDR switch, and the HAMR worker's allowlist
	logsSwitchConfigID    = "ddr-logs-switch"
	logsAllowlistConfigID = "hamr-logs-service-allowlist"
	failoverConfigName    = "config"

	failoverLogsSetting         = "multi_region_failover.failover_logs"
	logsServiceAllowlistSetting = "multi_region_failover.logs_service_allowlist"

	allowedService  = "mrf-allowed"
	excludedService = "mrf-excluded"

	// failoverAPIKey is sent to the failover fakeintake, which does not check it
	failoverAPIKey = "00000000000000000000000000failover"

	mrfWaitFor = 3 * time.Minute
	mrfTick    = 5 * time.Second
	// noFailoverDelay is how long to wait, once a log reached the main fakeintake, before asserting that it did
	// not reach the failover one: a log forwarded to the failover region goes out in the same payload as to the
	// main region, so this only covers the time between the two sends.
	noFailoverDelay = 15 * time.Second
)

var logFiles = map[string]string{
	allowedService:  "/tmp/mrf-allowed.log",
	excludedService: "/tmp/mrf-excluded.log",
}

var mrfLogsIntegrationConfig = fmt.Sprintf(`logs:
  - type: file
    path: %s
    service: %s
    source: mrf-e2e
  - type: file
    path: %s
    service: %s
    source: mrf-e2e
`, logFiles[allowedService], allowedService, logFiles[excludedService], excludedService)

// mrfEnv is a host Agent with two fakeintakes: one for the main region, and one for the failover region, which
// receives the logs forwarded by Multi-Region Failover and serves the AGENT_FAILOVER Remote Config.
type mrfEnv struct {
	Host           *components.RemoteHost
	Agent          *components.RemoteHostAgent
	FakeIntake     *components.FakeIntake
	FailoverIntake *components.FakeIntake
}

func (e *mrfEnv) Init(ctx common.Context) error {
	if e.Agent != nil {
		agentClient, err := client.NewHostAgentClient(ctx, e.Host.HostOutput, true)
		if err != nil {
			return err
		}
		e.Agent.Client = agentClient
	}
	return nil
}

func mrfProvisioner() provisioners.Provisioner {
	return provisioners.NewTypedPulumiProvisioner("aws-mrf-logs-service-allowlist", func(ctx *pulumi.Context, env *mrfEnv) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		host, err := ec2.NewVM(awsEnv, "mrflogs")
		if err != nil {
			return err
		}
		host.Export(ctx, &env.Host.HostOutput)

		mainIntake, err := fakeintake.NewECSFargateInstance(awsEnv, "main")
		if err != nil {
			return err
		}
		mainIntake.Export(ctx, &env.FakeIntake.FakeintakeOutput)

		failoverIntake, err := fakeintake.NewECSFargateInstance(awsEnv, "failover")
		if err != nil {
			return err
		}
		failoverIntake.Export(ctx, &env.FailoverIntake.FakeintakeOutput)

		hostAgent, err := agent.NewHostAgent(&awsEnv, host,
			agentparams.WithFakeintake(mainIntake),
			agentparams.WithLogs(),
			agentparams.WithIntegration("mrf_logs.d", mrfLogsIntegrationConfig),
			withMultiRegionFailover(failoverIntake),
		)
		if err != nil {
			return err
		}
		hostAgent.Export(ctx, &env.Agent.HostAgentOutput)

		return nil
	}, nil)
}

// withMultiRegionFailover enables Multi-Region Failover towards the failover fakeintake: logs failover starts
// disabled, and the Agent polls the failover fakeintake for AGENT_FAILOVER Remote Config.
func withMultiRegionFailover(failoverIntake *fakeintakecomp.Fakeintake) agentparams.Option {
	return func(p *agentparams.Params) error {
		rootJSON, err := fakeintakecomp.RCRootJSON()
		if err != nil {
			return fmt.Errorf("build fakeintake rc root json: %w", err)
		}
		p.ResourceOptions = append(p.ResourceOptions, pulumi.DependsOn([]pulumi.Resource{failoverIntake}))
		p.ExtraAgentConfig = append(p.ExtraAgentConfig, failoverIntake.URL.ApplyT(func(url string) string {
			return fmt.Sprintf(`multi_region_failover.enabled: true
multi_region_failover.failover_logs: false
multi_region_failover.api_key: %s
multi_region_failover.dd_url: %s
multi_region_failover.remote_configuration.rc_dd_url: %s
multi_region_failover.remote_configuration.refresh_interval: 5s
multi_region_failover.remote_configuration.config_root: '%s'
multi_region_failover.remote_configuration.director_root: '%s'
`, failoverAPIKey, url, url, rootJSON, rootJSON)
		}).(pulumi.StringOutput))
		return nil
	}
}

type mrfLogsServiceAllowlistSuite struct {
	e2e.BaseSuite[mrfEnv]
}

func TestMRFLogsServiceAllowlistSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &mrfLogsServiceAllowlistSuite{}, e2e.WithProvisioner(mrfProvisioner()))
}

// TestLogsServiceAllowlist drives logs failover through AGENT_FAILOVER Remote Config, as the DDR switch and the
// HAMR worker do, and checks which services' logs reach the failover region. failover_logs alone turns logs
// forwarding on or off; logs_service_allowlist only narrows it, and an empty or omitted list forwards every log.
func (s *mrfLogsServiceAllowlistSuite) TestLogsServiceAllowlist() {
	if s.Env().Agent.FIPSEnabled {
		s.T().Skip("Remote Config is not supported by the FIPS Agent")
	}

	s.EventuallyWithT(func(c *assert.CollectT) {
		assert.True(c, s.Env().Agent.Client.IsReady())
	}, mrfWaitFor, mrfTick)
	s.EventuallyWithT(func(c *assert.CollectT) {
		stats, err := s.Env().FailoverIntake.Client().RCStats()
		assert.NoError(c, err)
		assert.NotZero(c, stats.Polls, "the Agent did not poll the failover fakeintake for Remote Config")
	}, mrfWaitFor, mrfTick)

	s.Run("logs failover disabled forwards nothing", func() {
		s.assertForwarded()
	})

	s.Run("logs failover enabled without allowlist forwards every service", func() {
		s.addFailoverConfig(logsSwitchConfigID, `{"name": "DDR logs switch", "failover_logs": true}`)
		s.waitForRemoteConfigSetting(failoverLogsSetting, "true")
		s.assertForwarded(allowedService, excludedService)
	})

	s.Run("allowlist forwards only the listed services", func() {
		s.addFailoverConfig(logsAllowlistConfigID, `{"name": "HAMR Logs Service Allowlist", "logs_service_allowlist": ["`+allowedService+`"]}`)
		s.waitForRemoteConfigSetting(logsServiceAllowlistSetting, "[mrf-allowed]")
		s.assertForwarded(allowedService)
	})

	s.Run("allowlist omitted by every config forwards every service", func() {
		s.deleteFailoverConfig(logsAllowlistConfigID)
		s.waitForSettingWithoutRemoteConfig(logsServiceAllowlistSetting)
		s.assertForwarded(allowedService, excludedService)
	})

	s.Run("empty allowlist forwards every service", func() {
		s.addFailoverConfig(logsAllowlistConfigID, `{"name": "HAMR Logs Service Allowlist", "logs_service_allowlist": []}`)
		s.waitForRemoteConfigSetting(logsServiceAllowlistSetting, "[]")
		s.assertForwarded(allowedService, excludedService)
	})

	s.Run("logs failover disabled with an allowlist forwards nothing", func() {
		s.addFailoverConfig(logsAllowlistConfigID, `{"name": "HAMR Logs Service Allowlist", "logs_service_allowlist": ["`+allowedService+`"]}`)
		s.waitForRemoteConfigSetting(logsServiceAllowlistSetting, "[mrf-allowed]")
		s.addFailoverConfig(logsSwitchConfigID, `{"name": "DDR logs switch", "failover_logs": false}`)
		s.waitForRemoteConfigSetting(failoverLogsSetting, "false")
		s.assertForwarded()
	})
}

// addFailoverConfig publishes or replaces an AGENT_FAILOVER config on the failover fakeintake.
func (s *mrfLogsServiceAllowlistSuite) addFailoverConfig(configID, config string) {
	require.NoError(s.T(), s.Env().FailoverIntake.Client().RCAddConfig("", agentFailoverProduct, configID, failoverConfigName, []byte(config)))
}

// deleteFailoverConfig removes an AGENT_FAILOVER config from the failover fakeintake.
func (s *mrfLogsServiceAllowlistSuite) deleteFailoverConfig(configID string) {
	failoverIntake := s.Env().FailoverIntake.Client()
	configs, err := failoverIntake.RCListConfigs()
	require.NoError(s.T(), err)
	for _, config := range configs {
		if config.Product == agentFailoverProduct && config.ConfigID == configID {
			key := strings.Join([]string{config.OrgID, config.Product, config.ConfigID, config.ConfigName}, "/")
			require.NoError(s.T(), failoverIntake.RCDeleteConfig(key))
			return
		}
	}
	require.Failf(s.T(), "AGENT_FAILOVER config not found", "config %s", configID)
}

// waitForRemoteConfigSetting waits for the Agent to apply a value of the setting from Remote Config.
func (s *mrfLogsServiceAllowlistSuite) waitForRemoteConfigSetting(setting, value string) {
	s.EventuallyWithT(func(c *assert.CollectT) {
		output := s.settingWithSources(c, setting)
		assert.Contains(c, output, fmt.Sprintf("%s is set to: %s\n", setting, value))
		assert.Contains(c, output, fmt.Sprintf("remote-config: %s\n", value))
	}, mrfWaitFor, mrfTick)
}

// waitForSettingWithoutRemoteConfig waits for the Agent to remove the Remote Config value of the setting.
func (s *mrfLogsServiceAllowlistSuite) waitForSettingWithoutRemoteConfig(setting string) {
	s.EventuallyWithT(func(c *assert.CollectT) {
		assert.Contains(c, s.settingWithSources(c, setting), "remote-config: <nil>\n")
	}, mrfWaitFor, mrfTick)
}

func (s *mrfLogsServiceAllowlistSuite) settingWithSources(c *assert.CollectT, setting string) string {
	output, err := s.Env().Agent.Client.ConfigWithError(agentclient.WithArgs([]string{"get", setting, "--source"}))
	assert.NoError(c, err)
	return output
}

// assertForwarded writes a log line for each service, and checks that every line reaches the main region while
// only the lines of the given services reach the failover region. Whether a log is forwarded is decided when the
// Agent processes it, so the lines are written after the failover configuration was applied.
func (s *mrfLogsServiceAllowlistSuite) assertForwarded(forwarded ...string) {
	token := fmt.Sprintf("mrf-e2e-%d", time.Now().UnixNano())
	services := []string{allowedService, excludedService}
	for _, service := range services {
		s.Env().Host.MustExecute(fmt.Sprintf("echo '%s %s' >> %s", token, service, logFiles[service]))
	}

	mainIntake := s.Env().FakeIntake.Client()
	failoverIntake := s.Env().FailoverIntake.Client()
	s.EventuallyWithT(func(c *assert.CollectT) {
		for _, service := range services {
			logs, err := mainIntake.FilterLogs(service, fakeintakeclient.WithMessageContaining(token))
			assert.NoError(c, err)
			assert.NotEmpty(c, logs, "log of %s did not reach the main region", service)
		}
		for _, service := range forwarded {
			logs, err := failoverIntake.FilterLogs(service, fakeintakeclient.WithMessageContaining(token))
			assert.NoError(c, err)
			assert.NotEmpty(c, logs, "log of %s did not reach the failover region", service)
		}
	}, mrfWaitFor, mrfTick)

	time.Sleep(noFailoverDelay)
	for _, service := range services {
		if slices.Contains(forwarded, service) {
			continue
		}
		logs, err := failoverIntake.FilterLogs(service, fakeintakeclient.WithMessageContaining(token))
		require.NoError(s.T(), err)
		assert.Empty(s.T(), logs, "log of %s reached the failover region", service)
	}
}
