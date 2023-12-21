// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package installscript

import (
	"flag"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	windows "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/agent"

	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
)

var devMode = flag.Bool("devmode", false, "enable devmode")

type agentMSISuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestMSI(t *testing.T) {
	opts := []e2e.SuiteOption{e2e.WithProvisioner(awshost.Provisioner(awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault))))}
	if *devMode {
		opts = append(opts, e2e.WithDevMode())
	}

	e2e.Run(t, &agentMSISuite{}, opts...)
}

func (is *agentMSISuite) TestInstallAgent() {
	vm := is.Env().RemoteHost

	msi := `https://s3.amazonaws.com/ddagent-windows-stable/datadog-agent-7-latest.amd64.msi`
	is.Run("install the agent", func() {
		err := windows.InstallMSI(vm, msi, "", "install.log")
		is.Require().NoError(err, "should install the agent")
	})
	is.Run("uninstall the agent", func() {
		err := windowsAgent.UninstallAgent(vm, "uninstall.log")
		is.Require().NoError(err, "should uninstall the agent")
	})
}
