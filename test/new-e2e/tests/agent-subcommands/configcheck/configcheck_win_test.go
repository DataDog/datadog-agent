// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package configcheck

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
)

type windowsConfigCheckSuite struct {
	baseConfigCheckSuite
}

func TestWindowsConfigCheckSuite(t *testing.T) {
	e2e.Run(t, &windowsConfigCheckSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)))))
}

// cpu, disk, file_handle, io, memory, network, ntp, uptime, winproc
func (v *windowsConfigCheckSuite) TestDefaultInstalledChecks() {
	testChecks := []CheckConfigOutput{
		{
			CheckName:  "cpu",
			Filepath:   "file:C:\\ProgramData\\Datadog\\conf.d\\cpu.d\\conf.yaml.default",
			InstanceID: "cpu:",
			Settings:   "{}",
		},
		{
			CheckName:  "disk",
			Filepath:   "file:C:\\ProgramData\\Datadog\\conf.d\\disk.d\\conf.yaml.default",
			InstanceID: "disk:",
			Settings:   "use_mount: false",
		},
		{
			CheckName:  "file_handle",
			Filepath:   "file:C:\\ProgramData\\Datadog\\conf.d\\file_handle.d\\conf.yaml.default",
			InstanceID: "file_handle:",
			Settings:   "{}",
		},
		{
			CheckName:  "io",
			Filepath:   "file:C:\\ProgramData\\Datadog\\conf.d\\io.d\\conf.yaml.default",
			InstanceID: "io:",
			Settings:   "{}",
		},
		{
			CheckName:  "memory",
			Filepath:   "file:C:\\ProgramData\\Datadog\\conf.d\\memory.d\\conf.yaml.default",
			InstanceID: "memory:",
			Settings:   "{}",
		},
		{
			CheckName:  "network",
			Filepath:   "file:C:\\ProgramData\\Datadog\\conf.d\\network.d\\conf.yaml.default",
			InstanceID: "network:",
			Settings:   "{}",
		},
		{
			CheckName:  "ntp",
			Filepath:   "file:C:\\ProgramData\\Datadog\\conf.d\\ntp.d\\conf.yaml.default",
			InstanceID: "ntp:",
			Settings:   "{}",
		},
		{
			CheckName:  "uptime",
			Filepath:   "file:C:\\ProgramData\\Datadog\\conf.d\\uptime.d\\conf.yaml.default",
			InstanceID: "uptime:",
			Settings:   "{}",
		},
		{
			CheckName:  "winproc",
			Filepath:   "file:C:\\ProgramData\\Datadog\\conf.d\\winproc.d\\conf.yaml.default",
			InstanceID: "winproc:",
			Settings:   "{}",
		},
	}

	output := v.Env().Agent.Client.ConfigCheck()
	VerifyDefaultInstalledCheck(v.T(), output, testChecks)
}

func (v *windowsConfigCheckSuite) TestWithBadConfigCheck() {
	// invalid config because of tabspace
	config := `instances:
	- name: bad yaml formatting via tab
`
	integration := agentparams.WithIntegration("http_check.d", config)
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)), awshost.WithAgentOptions(integration)))

	output := v.Env().Agent.Client.ConfigCheck()

	assert.Contains(v.T(), output, "http_check: yaml: line 2: found character that cannot start any token")
}

func (v *windowsConfigCheckSuite) TestWithAddedIntegrationsCheck() {
	config := `instances:
  - name: My First Service
    url: http://some.url.example.com
`
	integration := agentparams.WithIntegration("http_check.d", config)
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)), awshost.WithAgentOptions(integration)))

	output := v.Env().Agent.Client.ConfigCheck()

	result, err := MatchCheckToTemplate("http_check", output)
	require.NoError(v.T(), err)
	assert.Contains(v.T(), result.Filepath, "file:C:\\ProgramData\\Datadog\\conf.d\\http_check.d\\conf.yaml")
	assert.Contains(v.T(), result.InstanceID, "http_check:")
	assert.Contains(v.T(), result.Settings, "name: My First Service")
	assert.Contains(v.T(), result.Settings, "url: http://some.url.example.com")
}
