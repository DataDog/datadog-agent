// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package auth

import (
	_ "embed"
	"path"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclientparams"
)

//go:embed fixtures/security-agent.yaml
var securityAgentConfig string

type authArtifactLinux struct {
	authArtifactBase
}

func TestIPCSecurityLinuxSuite(t *testing.T) {
	t.Parallel()

	e2e.Run(t,
		&authArtifactLinux{
			authArtifactBase{
				svcName:            "datadog-agent",
				authTokenPath:      "/etc/datadog-agent/auth_token",
				ipcCertPath:        "/etc/datadog-agent/ipc_cert.pem",
				removeFilesCmdTmpl: "sudo rm -f %s/* %s %s",
				readLogCmdTmpl:     "tail -f -n +1 %v",
				pathJoinFunction:   path.Join,
				agentProcesses:     []string{"agent", "trace-agent", "process-agent", "security-agent"}, // TODO IPC: add system-probe when it will load auth artifacts
			},
		},
		e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(
			awshost.WithRunOptions(
				ec2.WithAgentOptions(
					agentparams.WithAgentConfig(agentConfig),
					agentparams.WithSecurityAgentConfig(securityAgentConfig),
				),
				ec2.WithAgentClientOptions(agentclientparams.WithSkipWaitForAgentReady()),
			)),
		), e2e.WithSkipCoverage(), // Test Suite is not compatible with coverage computation, because auth tokens are removed at the end of the test
	)
}
