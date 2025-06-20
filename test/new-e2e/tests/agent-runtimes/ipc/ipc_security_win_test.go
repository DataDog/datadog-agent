// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package ipc

import (
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclientparams"
)

type ipcSecurityWindowsSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestIPCSecurityWindowsSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &ipcSecurityWindowsSuite{}, e2e.WithProvisioner(awshost.Provisioner(awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)))))
}

func (v *ipcSecurityWindowsSuite) TestServersideIPCCertUsage() {
	rootDir := "C:/tmp/" + v.T().Name()
	v.Env().RemoteHost.MkdirAll(rootDir)

	ipcCertFilePath := `C:\ProgramData\Datadog\ipc_cert.pem`

	templateVars := map[string]interface{}{
		"IPCCertFilePath": ipcCertFilePath,
		"AgentCMDPort":    coreCMDPort,
		"AgentIpcPort":    coreIPCPort,
		"ApmCmdPort":      apmCmdPort,
		"ProcessCmdPort":  processCmdPort,
		"SecurityCmdPort": securityCmdPort,
	}
	coreconfig := fillTmplConfig(v.T(), coreConfigTmpl, templateVars)

	agentOptions := []func(*agentparams.Params) error{
		agentparams.WithAgentConfig(coreconfig),
		agentparams.WithSecurityAgentConfig(securityAgentConfig),
	}
	// start the agent with that configuration
	v.UpdateEnv(awshost.Provisioner(
		awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)),
		awshost.WithAgentOptions(agentOptions...),
		awshost.WithAgentClientOptions(
			agentclientparams.WithTraceAgentOnPort(apmReceiverPort),
			agentclientparams.WithProcessAgentOnPort(processCmdPort),
		),
	))

	// Currently the e2e framework does not restart the security agent on Windows so we need to do it manually.
	// When the framework will support it, remove the line below and add `agentclientparams.WithSecurityAgentOnPort(securityCmdPort)` to the agent options.
	v.Env().RemoteHost.MustExecute("Restart-Service datadog-security-agent")

	// get auth token
	v.T().Log("Getting the IPC cert")
	ipcCertContent, err := v.Env().RemoteHost.ReadFile(ipcCertFilePath)
	require.NoError(v.T(), err)

	// check that the Agent API server use the IPC cert
	require.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		assertAgentUseCert(t, v.Env().RemoteHost, ipcCertContent)
	}, 2*configRefreshIntervalSec*time.Second, 1*time.Second)
}
