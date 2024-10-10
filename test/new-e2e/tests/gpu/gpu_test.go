// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package gpu

import (
	"flag"
	"testing"

	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
)

var devMode = flag.Bool("devmode", false, "enable dev mode")

type gpuSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestGPUSuite runs tests for the VM interface to ensure its implementation is correct.
func TestGPUSuite(t *testing.T) {
	provisioner := awshost.Provisioner(awshost.WithEC2InstanceOptions(ec2.WithInstanceType("g4dn.xlarge")))
	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(provisioner)}
	if *devMode {
		suiteParams = append(suiteParams, e2e.WithDevMode())
	}

	e2e.Run(t, &gpuSuite{}, suiteParams...)
}

func (v *gpuSuite) TestExecute() {
	vm := v.Env().RemoteHost

	out, err := vm.Execute("whoami")
	v.Require().NoError(err)
	v.Require().NotEmpty(out)
}
