// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentconfiguration

import (
	_ "embed"
	"path/filepath"
	"strings"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-configuration/secretsutils"
)

//go:embed config-refresh/fixtures/systemprobe-secret-config.yaml.tmpl
var systemProbeSecretConfigTmpl string

// Defaults applied by EnableAgentIPCForSystemProbeSecurity in pkg/config/setup/config.go
// when CSPM or CWS ships its payloads from system-probe.
const (
	autoAgentIPCPort                 = 5009
	autoConfigRefreshIntervalSeconds = 60
)

const systemProbeLogPath = "/var/log/datadog/system-probe.log"

// TestSystemProbeResolvesSecretAPIKey covers the path that lets CSPM ship from
// system-probe when the api_key is secret-backed.
//
// system-probe runs with a no-op secrets resolver, so it cannot expand
// ENC[api_key] itself; it can only learn the resolved value from the core agent
// over the IPC config endpoint. Enabling CSPM is expected to open that endpoint
// automatically, because compliance_config.run_in_system_probe now defaults to
// true. If any link in that chain breaks, system-probe builds its intake
// endpoints with the literal "ENC[api_key]" string and silently fails to ship.
func (v *configRefreshLinuxSuite) TestSystemProbeResolvesSecretAPIKey() {
	rootDir := "/tmp/" + v.T().Name()
	v.Env().RemoteHost.MkdirAll(rootDir)

	secretResolverPath := filepath.Join(rootDir, "secret-resolver.py")

	v.T().Log("Setting up the secret resolver and the api key file")
	secretClient := secretsutils.NewClient(v.T(), v.Env().RemoteHost, rootDir)
	secretClient.SetSecret("api_key", apiKey1)

	coreconfig := fillTmplConfig(v.T(), systemProbeSecretConfigTmpl, map[string]interface{}{
		"SecretDirectory": rootDir,
		"SecretResolver":  secretResolverPath,
	})

	v.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(scenec2.WithAgentOptions(
			secretsutils.WithUnixSetupScript(secretResolverPath, true),
			agentparams.WithAgentConfig(coreconfig),
			// Nothing is needed here: the compliance module is enabled from the
			// core config, and that alone is what makes system-probe run.
			agentparams.WithSystemProbeConfig("# intentionally empty\n"),
			agentparams.WithSkipAPIKeyInConfig(), // api_key is already provided in the config
		)),
	))

	v.T().Log("Checking that the core agent opened its IPC config endpoint")
	var runtimeConfig struct {
		AgentIPC struct {
			Port                  int `yaml:"port"`
			ConfigRefreshInterval int `yaml:"config_refresh_interval"`
		} `yaml:"agent_ipc"`
	}
	require.NoError(v.T(), yaml.Unmarshal([]byte(v.Env().Agent.Client.Config()), &runtimeConfig))
	assert.Equal(v.T(), autoAgentIPCPort, runtimeConfig.AgentIPC.Port,
		"enabling CSPM should have opened the agent IPC config endpoint")
	assert.Equal(v.T(), autoConfigRefreshIntervalSeconds, runtimeConfig.AgentIPC.ConfigRefreshInterval,
		"enabling CSPM should have turned on config refresh")

	v.T().Log("Checking that system-probe stayed up")
	// system-probe synchronizes its configuration at init and fails to start if
	// the core agent is not reachable, so this also covers the startup ordering
	// between the two services.
	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		state, _ := v.Env().RemoteHost.Execute("systemctl is-active datadog-agent-sysprobe")
		assert.Equal(c, "active", strings.TrimSpace(state), "system-probe is not running")
	}, 2*time.Minute, 5*time.Second)

	v.T().Log("Checking that system-probe resolved the api key")
	// The logs pipeline redacts the key it is about to use through the scrubber,
	// so reuse it rather than reimplementing how many characters stay visible.
	resolved := scrubber.HideKeyExceptLastChars(apiKey1)
	unresolved := scrubber.HideKeyExceptLastChars("ENC[api_key]")
	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		logs := v.Env().RemoteHost.MustExecuteOn(c, "sudo cat "+systemProbeLogPath)
		assert.Contains(c, logs, "(API Key: "+resolved+")",
			"system-probe did not build its intake endpoints with the resolved api key")
		assert.NotContains(c, logs, unresolved,
			"system-probe used the unresolved ENC[api_key] literal, so config sync did not reach it")
	}, 2*time.Minute, 5*time.Second)

	v.T().Log("Checking that the security-agent stood down")
	// CSPM is the only workload the security-agent had to run, and it now runs
	// in system-probe, so the process is expected to exit cleanly at startup.
	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		state, _ := v.Env().RemoteHost.Execute("systemctl is-active datadog-agent-security")
		// Exiting with ErrAllComponentsDisabled is a clean exit, so systemd
		// parks the unit as inactive rather than failed or restarting.
		assert.Equal(c, "inactive", strings.TrimSpace(state),
			"the security-agent should have stood down when CSPM runs in system-probe")
	}, 2*time.Minute, 5*time.Second)
}
