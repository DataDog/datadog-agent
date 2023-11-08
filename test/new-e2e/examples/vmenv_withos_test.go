// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/test"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components"
	"github.com/DataDog/test-infra-definitions/components/vm"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2vm"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
)

type Remote vm.ERemote

var _ components.Importable = &Remote{}

func (r *Remote) GetClient(t *testing.T) (*client.VMClient, error) {
	return client.NewVMClient(t, &utils.Connection{
		Host: r.Host,
		User: r.Username,
	}, r.OSType)
}

type myVMEnv struct {
	Local *Remote
	VM    *Remote
}

type myVMSuite struct {
	test.BaseSuite[myVMEnv]
}

func TestMyVMSuite(t *testing.T) {
	var provisionerOpt test.Option

	if os.Getenv("MYLOCALTEST") == "true" {
		provisionerOpt = test.WithProvisioner(test.NewFileProvisioner("mydir", os.DirFS("mydir")))
	} else {
		provisionerOpt = test.WithTypedPulumiProvisioner[myVMEnv](func(ctx *pulumi.Context, env *myVMEnv) error {
			myVM, err := ec2vm.NewEc2VM(ctx)
			if err != nil {
				return err
			}

			return myVM.Export(ctx, myVM, env.VM)
		}, nil)
	}

	test.Run(t, &myVMSuite{}, provisionerOpt, test.WithDevMode())
}

func (v *myVMSuite) TestItIsUbuntu() {
	env := v.Env()
	client, err := env.VM.GetClient(v.T())
	assert.NoError(v.T(), err)

	res := client.Execute("cat /etc/os-release")
	assert.Contains(v.T(), res, "Ubuntu")
}
