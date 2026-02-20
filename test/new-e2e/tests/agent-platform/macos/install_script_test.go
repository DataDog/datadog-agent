// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package macos implements tests for the agent install script on MacOS.
package macos

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install/installparams"
)

type macosInstallSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestMacosInstallScript(t *testing.T) {
	extraConfigMap := runner.ConfigMap{}
	// When the environment is initialized Pulumi needs to be aware that it must chose in a smaller subset of subnet on MacOS.
	// Going directly through the configmap is the only way we have for now to let Pulumi know about it.
	extraConfigMap.Set("ddinfra:aws/useMacosCompatibleSubnets", "true", false)
	e2e.Run(t, &macosInstallSuite{}, e2e.WithProvisioner(
		awshost.ProvisionerNoAgentNoFakeIntake(
			awshost.WithRunOptions(ec2.WithEC2InstanceOptions(ec2.WithOS(os.MacOSDefault))),
			awshost.WithExtraConfigParams(extraConfigMap),
		)),
	)
}

func (m *macosInstallSuite) TestInstallAgent() {
	macosTestClient := common.NewMacOSTestClient(m.Env().RemoteHost)

	install.MacOS(m.T(), macosTestClient, installparams.WithUsername(m.Env().RemoteHost.Username), installparams.WithArch("x64"))

	// The agent should start at some point
	m.EventuallyWithT(func(c *assert.CollectT) {
		_, err := macosTestClient.Execute("/usr/local/bin/datadog-agent status")
		assert.NoError(c, err)
	}, 20*time.Second, 1*time.Second)

	// check that there is no world-writable files or directories in /opt/datadog-agent
	// exclude /opt/datadog-agent/run/ipc which is intentionally world-writable for multi-user GUI sockets
	worldWritableFiles, err := macosTestClient.Execute("sudo find /opt/datadog-agent \\( -type f -o -type d \\) -perm -002 ! -path '/opt/datadog-agent/run/ipc' ! -path '/opt/datadog-agent/run/ipc/*'")
	assert.NoError(m.T(), err)
	assert.Empty(m.T(), strings.TrimSpace(worldWritableFiles))
}
