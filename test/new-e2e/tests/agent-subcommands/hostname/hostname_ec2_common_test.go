// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostname

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
)

type baseHostnameSuite struct {
	e2e.BaseSuite[environments.Host]
	osOption awshost.ProvisionerOption
}

func (v *baseHostnameSuite) GetOs() awshost.ProvisionerOption {
	return v.osOption
}

func (v *baseHostnameSuite) TestAgentConfigHostnameVarOverride() {
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(v.GetOs(), awshost.WithAgentOptions(agentparams.WithAgentConfig("hostname: hostname.from.var"))))

	hostname := v.Env().Agent.Client.Hostname()
	assert.Equal(v.T(), hostname, "hostname.from.var")
}

// hostname_force_config_as_canonical stops throwing a warning but doesn't change behavior
func (v *baseHostnameSuite) TestAgentConfigHostnameForceAsCanonical() {
	config := `hostname: ip-172-29-113-35.ec2.internal
hostname_force_config_as_canonical: true`

	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(v.GetOs(), awshost.WithAgentOptions(agentparams.WithAgentConfig(config))))

	hostname := v.Env().Agent.Client.Hostname()
	assert.Equal(v.T(), hostname, "ip-172-29-113-35.ec2.internal")
}

func (v *baseHostnameSuite) TestAgentConfigPrioritizeEC2Id() {
	// ec2_prioritize_instance_id_as_hostname doesn't override higher priority providers
	config := `hostname: hostname.from.var
ec2_prioritize_instance_id_as_hostname: true`
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(v.GetOs(), awshost.WithAgentOptions(agentparams.WithAgentConfig(config))))

	hostname := v.Env().Agent.Client.Hostname()
	assert.Equal(v.T(), hostname, "hostname.from.var")
}
