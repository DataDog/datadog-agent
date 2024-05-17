// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostname

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"
)

type linuxHostnameSuite struct {
	baseHostnameSuite
}

func TestLinuxHostnameSuite(t *testing.T) {
	osOption := awshost.WithEC2InstanceOptions(ec2.WithOS(os.UbuntuDefault))
	e2e.Run(t, &linuxHostnameSuite{baseHostnameSuite: baseHostnameSuite{osOption: osOption}}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake()))
}

func (v *linuxHostnameSuite) TestAgentConfigHostnameFileOverride() {
	fileContent := "hostname.from.file"
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(v.GetOs(), awshost.WithAgentOptions(agentparams.WithFile("/tmp/var/hostname", fileContent, false), agentparams.WithAgentConfig("hostname_file: /tmp/var/hostname"))))

	hostname := v.Env().Agent.Client.Hostname()
	assert.Equal(v.T(), fileContent, hostname)
}

func (v *linuxHostnameSuite) TestAgentConfigPreferImdsv2() {
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(v.GetOs(), awshost.WithAgentOptions(agentparams.WithAgentConfig("ec2_prefer_imdsv2: true"))))
	// e2e metadata provider already uses IMDSv2
	metadata := client.NewEC2Metadata(v.T(), v.Env().RemoteHost.Host, v.Env().RemoteHost.OSFamily)

	hostname := v.Env().Agent.Client.Hostname()
	resourceID := metadata.Get("instance-id")
	assert.Equal(v.T(), resourceID, hostname)
}

// https://github.com/DataDog/datadog-agent/blob/main/pkg/util/hostname/README.md#the-current-logic
func (v *linuxHostnameSuite) TestAgentHostnameDefaultsToResourceId() {
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(v.GetOs(), awshost.WithAgentOptions(agentparams.WithAgentConfig(""))))

	metadata := client.NewEC2Metadata(v.T(), v.Env().RemoteHost.Host, v.Env().RemoteHost.OSFamily)
	hostname := v.Env().Agent.Client.Hostname()

	// Default configuration of hostname for EC2 instances is the resource-id
	resourceID := metadata.Get("instance-id")
	assert.Equal(v.T(), resourceID, hostname)
}
