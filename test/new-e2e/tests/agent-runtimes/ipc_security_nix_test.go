// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentruntimes

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclientparams"
	ipchelpers "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-runtimes/ipc"
)

type ipcSecurityLinuxSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestIPCSecurityLinuxSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &ipcSecurityLinuxSuite{}, e2e.WithProvisioner(awshost.Provisioner()))
}

func (v *ipcSecurityLinuxSuite) TestServersideIPCCertUsage() {
	rootDir := "/tmp/" + v.T().Name()
	v.Env().RemoteHost.MkdirAll(rootDir)

	ipcCertFilePath := "/etc/datadog-agent/ipc_cert.pem"

	// fill the config template
	templateVars := map[string]interface{}{
		"IPCCertFilePath": ipcCertFilePath,
		"AgentCMDPort":    ipchelpers.CoreCMDPort,
		"ApmCmdPort":      ipchelpers.ApmCmdPort,
		"AgentIpcPort":    ipchelpers.CoreIPCPort,
		"ProcessCmdPort":  ipchelpers.ProcessCmdPort,
		"SecurityCmdPort": ipchelpers.SecurityCmdPort,
	}
	coreconfig := ipchelpers.FillTmplConfig(v.T(), ipchelpers.CoreConfigTmpl, templateVars)

	// start the agent with that configuration
	v.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(
			ec2.WithAgentOptions(
				agentparams.WithAgentConfig(coreconfig),
				agentparams.WithSecurityAgentConfig(ipchelpers.SecurityAgentConfig),
			),
			ec2.WithAgentClientOptions(
				agentclientparams.WithTraceAgentOnPort(ipchelpers.ApmReceiverPort),
				agentclientparams.WithProcessAgentOnPort(ipchelpers.ProcessCmdPort),
				agentclientparams.WithSecurityAgentOnPort(ipchelpers.SecurityCmdPort),
			),
		),
	))

	// get auth token
	v.T().Log("Getting the IPC cert")
	ipcCertContent := v.Env().RemoteHost.MustExecute("sudo cat " + ipcCertFilePath)

	// check that the Agent API server use the IPC cert
	require.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		ipchelpers.AssertAgentUseCert(t, v.Env().RemoteHost, []byte(strings.TrimSpace(ipcCertContent)))
	}, 2*ipchelpers.ConfigRefreshIntervalSec*time.Second, 1*time.Second)
}
