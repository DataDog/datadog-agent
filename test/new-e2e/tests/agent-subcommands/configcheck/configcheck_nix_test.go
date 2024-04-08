// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package configcheck

import (
	"testing"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
)

type linuxConfigCheckSuite struct {
	baseConfigCheckSuite
}

func TestLinuxConfigCheckSuite(t *testing.T) {
	e2e.Run(t, &linuxConfigCheckSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake()))
}

// cpu, disk, file_handle, io, load, memory, network, ntp, uptime
func (v *linuxConfigCheckSuite) TestDefaultInstalledChecks() {
	testChecks := []CheckConfigOutput{
		{
			CheckName:  "cpu",
			Filepath:   "file:/etc/datadog-agent/conf.d/cpu.d/conf.yaml.default",
			InstanceID: "cpu:",
			Settings:   "{}",
		},
		{
			CheckName:  "disk",
			Filepath:   "file:/etc/datadog-agent/conf.d/disk.d/conf.yaml.default",
			InstanceID: "disk:",
			Settings:   "use_mount: false",
		},
		{
			CheckName:  "file_handle",
			Filepath:   "file:/etc/datadog-agent/conf.d/file_handle.d/conf.yaml.default",
			InstanceID: "file_handle:",
			Settings:   "{}",
		},
		{
			CheckName:  "io",
			Filepath:   "file:/etc/datadog-agent/conf.d/io.d/conf.yaml.default",
			InstanceID: "io:",
			Settings:   "{}",
		},
		{
			CheckName:  "load",
			Filepath:   "file:/etc/datadog-agent/conf.d/load.d/conf.yaml.default",
			InstanceID: "load:",
			Settings:   "{}",
		},
		{
			CheckName:  "memory",
			Filepath:   "file:/etc/datadog-agent/conf.d/memory.d/conf.yaml.default",
			InstanceID: "memory:",
			Settings:   "{}",
		},
		{
			CheckName:  "network",
			Filepath:   "file:/etc/datadog-agent/conf.d/network.d/conf.yaml.default",
			InstanceID: "network:",
			Settings:   "{}",
		},
		{
			CheckName:  "ntp",
			Filepath:   "file:/etc/datadog-agent/conf.d/ntp.d/conf.yaml.default",
			InstanceID: "ntp:",
			Settings:   "{}",
		},
		{
			CheckName:  "uptime",
			Filepath:   "file:/etc/datadog-agent/conf.d/uptime.d/conf.yaml.default",
			InstanceID: "uptime:",
			Settings:   "{}",
		},
	}

	output := v.Env().Agent.Client.ConfigCheck()
	VerifyDefaultInstalledCheck(v.T(), output, testChecks)
}

func (v *linuxConfigCheckSuite) TestWithBadConfigCheck() {
	// invalid config because of tabspace
	config := `instances:
	- name: bad yaml formatting via tab
`
	integration := agentparams.WithIntegration("http_check.d", config)
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithAgentOptions(integration)))

	output := v.Env().Agent.Client.ConfigCheck()

	assert.Contains(v.T(), output, "http_check: yaml: line 2: found character that cannot start any token")
}

func (v *linuxConfigCheckSuite) TestWithAddedIntegrationsCheck() {
	config := `instances:
  - name: My First Service
    url: http://some.url.example.com
`
	integration := agentparams.WithIntegration("http_check.d", config)
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithAgentOptions(integration)))

	output := v.Env().Agent.Client.ConfigCheck()

	result, err := MatchCheckToTemplate("http_check", output)
	require.NoError(v.T(), err)
	assert.Contains(v.T(), result.Filepath, "file:/etc/datadog-agent/conf.d/http_check.d/conf.yaml")
	assert.Contains(v.T(), result.InstanceID, "http_check:")
	assert.Contains(v.T(), result.Settings, "name: My First Service")
	assert.Contains(v.T(), result.Settings, "url: http://some.url.example.com")
}
