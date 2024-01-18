// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"

	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type multiVMEnv struct {
	MainVM *components.RemoteHost
	AppVM  *components.RemoteHost
}

func multiVMEnvProvisioner() e2e.PulumiEnvRunFunc[multiVMEnv] {
	return func(ctx *pulumi.Context, env *multiVMEnv) error {
		awsEnv, err := aws.NewEnvironment(ctx)

		main, err := ec2.NewVM(awsEnv, "main", ec2.WithOS(os.UbuntuDefault))
		if err != nil {
			return err
		}
		main.Export(ctx, &env.MainVM.HostOutput)

		app, err := ec2.NewVM(awsEnv, "app", ec2.WithOS(os.AmazonLinuxDefault))
		if err != nil {
			return err
		}
		app.Export(ctx, &env.AppVM.HostOutput)

		return nil
	}
}

type multiVMSuite struct {
	e2e.BaseSuite[multiVMEnv]
}

func TestMultiVMSuite(t *testing.T) {
	e2e.Run(t, &multiVMSuite{}, e2e.WithPulumiProvisioner(multiVMEnvProvisioner(), nil))
}

func (v *multiVMSuite) TestItIsExpectedOS() {
	res := v.Env().MainVM.MustExecute("cat /etc/os-release")
	v.Assert().Contains(res, "Ubuntu")
	res = v.Env().AppVM.MustExecute("cat /etc/os-release")
	v.Assert().Contains(res, "Amazon Linux")
}
