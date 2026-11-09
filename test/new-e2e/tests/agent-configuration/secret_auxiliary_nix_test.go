// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentconfiguration

import (
	"strings"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-configuration/secretsutils"
)

// TestAuxiliarySecretRefresh checks the running processes, including their real
// secret-backend timers. No restart occurs between the initial and rotated keys.
func (v *secretLinuxRuntimeSuite) TestAuxiliarySecretRefresh() {
	const directory = "/tmp/fa-auxiliary-secret-runtime"
	const backend = directory + "/resolver.py"
	require.NoError(v.T(), v.Env().RemoteHost.MkdirAll(directory))
	client := secretsutils.NewClient(v.T(), v.Env().RemoteHost, directory)
	client.SetSecret("system_probe_key", strings.Repeat("1", 32))
	client.SetSecret("security_agent_key", strings.Repeat("2", 32))
	v.T().Cleanup(func() {
		v.UpdateEnv(awshost.Provisioner())
		_, err := v.Env().RemoteHost.Execute("sudo rm -rf " + directory)
		assert.NoError(v.T(), err)
	})
	v.UpdateEnv(awshost.Provisioner(awshost.WithRunOptions(scenec2.WithAgentOptions(
		secretsutils.WithUnixSetupScript(backend, true),
		agentparams.WithAgentConfig(`secret_backend_command: `+backend+`
secret_backend_arguments: [`+directory+`]
secret_backend_command_allow_group_exec_perm: true
secret_refresh_interval: 5
secret_refresh_scatter: false
`),
		agentparams.WithSystemProbeConfig(`network_config:
  enabled: true
system_probe_config:
  internal_profiling:
    enabled: false
    api_key: ENC[system_probe_key]
`),
		agentparams.WithSecurityAgentConfig(`compliance_config:
  enabled: true
  run_in_system_probe: false
security_agent:
  internal_profiling:
    enabled: false
    api_key: ENC[security_agent_key]
`),
	))))
	assertKeys := func(systemProbe, securityAgent string) {
		require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
			for _, process := range []struct{ binary, section, suffix string }{
				{"system-probe", "system_probe_config", systemProbe},
				{"security-agent", "security_agent", securityAgent},
			} {
				output := v.Env().RemoteHost.MustExecuteOn(c, "sudo /opt/datadog-agent/embedded/bin/"+process.binary+" config")
				var config map[string]any
				require.NoError(c, yaml.Unmarshal([]byte(output), &config))
				section, ok := config[process.section].(map[string]any)
				require.True(c, ok, "%s config section missing", process.binary)
				profile, ok := section["internal_profiling"].(map[string]any)
				require.True(c, ok, "%s profiling section missing", process.binary)
				key, ok := profile["api_key"].(string)
				require.True(c, ok)
				require.GreaterOrEqual(c, len(key), 4)
				assert.Equal(c, process.suffix, key[len(key)-4:], "%s runtime key", process.binary)
				assert.NotContains(c, key, "ENC[")
			}
		}, 2*time.Minute, 5*time.Second)
	}
	assertKeys("1111", "2222")
	client.SetSecret("system_probe_key", strings.Repeat("3", 32))
	client.SetSecret("security_agent_key", strings.Repeat("4", 32))
	assertKeys("3333", "4444")
}
