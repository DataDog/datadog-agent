// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package installscript

import (
	"flag"

	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/params"
	windows "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/agent"

	"testing"
)

var (
	devMode = flag.Bool("devmode", false, "enable devmode")
)

type agentMSISuite struct {
	e2e.Suite[e2e.VMEnv]
}

func TestMSI(t *testing.T) {
	var opts []func(*params.Params)

	if *devMode {
		opts = append(opts, params.WithDevMode())
	}

	e2e.Run(t,
		&agentMSISuite{},
		e2e.EC2VMStackDef(ec2params.WithOS(ec2os.WindowsOS)),
		opts...)
}

func (is *agentMSISuite) TestInstallAgent() {
	vm := is.Env().VM

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
