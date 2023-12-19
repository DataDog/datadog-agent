// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2vm"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type multiVMEnv struct {
	MainVM client.VM
	AppVM  client.VM
}

func multiEC2VMStackDef() *e2e.StackDefinition[multiVMEnv] {
	return e2e.EnvFactoryStackDef(func(ctx *pulumi.Context) (*multiVMEnv, error) {
		mainVM, err := ec2vm.NewEc2VM(ctx, ec2params.WithOS(ec2os.UbuntuOS), ec2params.WithName("main"))
		if err != nil {
			return nil, err
		}
		sharedAWSEnv := mainVM.GetAwsEnvironment()
		appVM, err := ec2vm.NewEC2VMWithEnv(sharedAWSEnv, ec2params.WithOS(ec2os.AmazonLinuxOS), ec2params.WithName("app"))
		if err != nil {
			return nil, err
		}
		return &multiVMEnv{
			MainVM: client.NewPulumiStackVM(mainVM),
			AppVM:  client.NewPulumiStackVM(appVM),
		}, nil
	})
}

type multiVMSuite struct {
	e2e.Suite[multiVMEnv]
}

func TestMultiVMSuite(t *testing.T) {
	e2e.Run(t, &multiVMSuite{}, multiEC2VMStackDef())
}

func (v *multiVMSuite) TestItIsExpectedOS() {
	res := v.Env().MainVM.Execute("cat /etc/os-release")
	v.Assert().Contains(res, "Ubuntu")
	res = v.Env().AppVM.Execute("cat /etc/os-release")
	v.Assert().Contains(res, "Amazon Linux")
}
