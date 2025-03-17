// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ipc

import (
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclientparams"
)

type ipcSecurityLinuxSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestIPCSecuirityLinuxSuite(t *testing.T) {
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
		"AgentCMDPort":    coreCMDPort,
		"ApmCmdPort":      apmCmdPort,
		"AgentIpcPort":    coreIPCPort,
		"ProcessCmdPort":  processCmdPort,
		"SecurityCmdPort": securityCmdPort,
	}
	coreconfig := fillTmplConfig(v.T(), coreConfigTmpl, templateVars)

	// start the agent with that configuration
	v.UpdateEnv(awshost.Provisioner(
		awshost.WithAgentOptions(
			agentparams.WithAgentConfig(coreconfig),
			agentparams.WithSecurityAgentConfig(securityAgentConfig),
		),
		awshost.WithAgentClientOptions(
			agentclientparams.WithTraceAgentOnPort(apmReceiverPort),
			agentclientparams.WithProcessAgentOnPort(processCmdPort),
			agentclientparams.WithSecurityAgentOnPort(securityCmdPort),
		),
	))

	// get auth token
	v.T().Log("Getting the IPC cert")
	ipcCertContent := v.Env().RemoteHost.MustExecute("sudo cat " + ipcCertFilePath)

	// Reading and decoding cert and key from file
	var block *pem.Block

	block, _ = pem.Decode([]byte(strings.TrimSpace(ipcCertContent)))
	require.NotNil(v.T(), block)
	require.Equal(v.T(), block.Type, "CERTIFICATE")
	cert := pem.EncodeToMemory(block)

	certPool := x509.NewCertPool()
	ok := certPool.AppendCertsFromPEM(cert)
	require.True(v.T(), ok)

	// check that the Agent API server use the IPC cert
	require.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		assertAgentUseCert(t, v.Env().RemoteHost, certPool)
	}, 2*configRefreshIntervalSec*time.Second, 1*time.Second)
}
