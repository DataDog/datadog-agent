// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentsubcommands

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e/client"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
)

type agentSuite struct {
	e2e.Suite[e2e.AgentEnv]
}

func TestAgentHostnameEC2Suite(t *testing.T) {
	e2e.Run(t, &agentSuite{}, e2e.AgentStackDef(nil))
}

// https://github.com/DataDog/datadog-agent/blob/main/pkg/util/hostname/README.md#the-current-logic
func (v *agentSuite) TestAgentHostnameDefaultsToResourceId() {
	v.UpdateEnv(e2e.AgentStackDef(nil, agentparams.WithAgentConfig("")))

	metadata := client.NewEC2Metadata(v.Env().VM)
	err := v.Env().Agent.WaitForReady()
	assert.NoError(v.T(), err)

	hostname := v.Env().Agent.Hostname()

	// Default configuration of hostname for EC2 instances is the resource-id
	resourceId := metadata.Get("instance-id")
	assert.Equal(v.T(), hostname, resourceId)
}

func (v *agentSuite) TestAgentConfigHostnameVarOverride() {
	v.UpdateEnv(e2e.AgentStackDef(nil, agentparams.WithAgentConfig("hostname: hostname.from.var")))
	err := v.Env().Agent.WaitForReady()
	assert.NoError(v.T(), err)

	hostname := v.Env().Agent.Hostname()
	assert.Equal(v.T(), hostname, "hostname.from.var")
}

func (v *agentSuite) TestAgentConfigHostnameFileOverride() {
	fileContent := "hostname.from.file"
	v.Env().VM.Execute(fmt.Sprintf(`echo "%s" | tee /tmp/hostname`, fileContent))
	v.UpdateEnv(e2e.AgentStackDef(nil, agentparams.WithAgentConfig("hostname_file: /tmp/hostname")))

	err := v.Env().Agent.WaitForReady()
	assert.NoError(v.T(), err)

	hostname := v.Env().Agent.Hostname()
	assert.Equal(v.T(), hostname, fileContent)
}

// hostname_force_config_as_canonical stops throwing a warning but doesn't change behavior
func (v *agentSuite) TestAgentConfigHostnameForceAsCanonical() {
	config := `hostname: ip-172-29-113-35.ec2.internal
hostname_force_config_as_canonical: true`
	v.UpdateEnv(e2e.AgentStackDef(nil, agentparams.WithAgentConfig(config)))

	err := v.Env().Agent.WaitForReady()
	assert.NoError(v.T(), err)

	hostname := v.Env().Agent.Hostname()
	assert.Equal(v.T(), hostname, "ip-172-29-113-35.ec2.internal")
}

func (v *agentSuite) TestAgentConfigPrioritizeEC2Id() {
	// ec2_prioritize_instance_id_as_hostname doesn't override higher priority providers
	config := `hostname: hostname.from.var
ec2_prioritize_instance_id_as_hostname: true`
	v.UpdateEnv(e2e.AgentStackDef(nil, agentparams.WithAgentConfig(config)))

	err := v.Env().Agent.WaitForReady()
	assert.NoError(v.T(), err)

	hostname := v.Env().Agent.Hostname()
	assert.Equal(v.T(), hostname, "hostname.from.var")
}

func (v *agentSuite) TestAgentConfigPreferImdsv2() {
	// e2e metadata provider already uses IMDSv2
	metadata := client.NewEC2Metadata(v.Env().VM)

	v.UpdateEnv(e2e.AgentStackDef(nil, agentparams.WithAgentConfig("ec2_prefer_imdsv2: true")))

	err := v.Env().Agent.WaitForReady()
	assert.NoError(v.T(), err)

	hostname := v.Env().Agent.Hostname()
	resourceId := metadata.Get("instance-id")
	assert.Equal(v.T(), hostname, resourceId)
}
