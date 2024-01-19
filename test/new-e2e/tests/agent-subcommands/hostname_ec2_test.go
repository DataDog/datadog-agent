// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentsubcommands

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"

	"github.com/stretchr/testify/assert"
)

type agentHostnameSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestAgentHostnameEC2Suite(t *testing.T) {
	e2e.Run(t, &agentHostnameSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake()))
}

// https://github.com/DataDog/datadog-agent/blob/main/pkg/util/hostname/README.md#the-current-logic
func (v *agentHostnameSuite) TestAgentHostnameDefaultsToResourceId() {
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithAgentOptions(agentparams.WithAgentConfig(""))))

	metadata := client.NewEC2Metadata(v.Env().RemoteHost)
	hostname := v.Env().Agent.Client.Hostname()

	// Default configuration of hostname for EC2 instances is the resource-id
	resourceID := metadata.Get("instance-id")
	assert.Equal(v.T(), hostname, resourceID)
}

func (v *agentHostnameSuite) TestAgentConfigHostnameVarOverride() {
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithAgentOptions(agentparams.WithAgentConfig("hostname: hostname.from.var"))))

	hostname := v.Env().Agent.Client.Hostname()
	assert.Equal(v.T(), hostname, "hostname.from.var")
}

func (v *agentHostnameSuite) TestAgentConfigHostnameFileOverride() {
	fileContent := "hostname.from.file"
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithAgentOptions(agentparams.WithFile("/tmp/var/hostname", fileContent, false), agentparams.WithAgentConfig("hostname_file: /tmp/var/hostname"))))

	hostname := v.Env().Agent.Client.Hostname()
	assert.Equal(v.T(), hostname, fileContent)
}

// hostname_force_config_as_canonical stops throwing a warning but doesn't change behavior
func (v *agentHostnameSuite) TestAgentConfigHostnameForceAsCanonical() {
	config := `hostname: ip-172-29-113-35.ec2.internal
hostname_force_config_as_canonical: true`

	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithAgentOptions(agentparams.WithAgentConfig(config))))

	hostname := v.Env().Agent.Client.Hostname()
	assert.Equal(v.T(), hostname, "ip-172-29-113-35.ec2.internal")
}

func (v *agentHostnameSuite) TestAgentConfigPrioritizeEC2Id() {
	// ec2_prioritize_instance_id_as_hostname doesn't override higher priority providers
	config := `hostname: hostname.from.var
ec2_prioritize_instance_id_as_hostname: true`
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithAgentOptions(agentparams.WithAgentConfig(config))))

	hostname := v.Env().Agent.Client.Hostname()
	assert.Equal(v.T(), hostname, "hostname.from.var")
}

func (v *agentHostnameSuite) TestAgentConfigPreferImdsv2() {
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithAgentOptions(agentparams.WithAgentConfig("ec2_prefer_imdsv2: true"))))
	// e2e metadata provider already uses IMDSv2
	metadata := client.NewEC2Metadata(v.Env().RemoteHost)

	hostname := v.Env().Agent.Client.Hostname()
	resourceID := metadata.Get("instance-id")
	assert.Equal(v.T(), hostname, resourceID)
}
