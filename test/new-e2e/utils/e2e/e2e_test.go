package e2e

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e/client"
	ec2vm "github.com/DataDog/test-infra-definitions/aws/scenarios/vm/ec2VM"
	"github.com/DataDog/test-infra-definitions/aws/scenarios/vm/os"
	commonos "github.com/DataDog/test-infra-definitions/common/os"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type MyEnv struct {
	VM *client.VM
}

type vmSuite struct {
	*Suite[MyEnv]
}

func TestE2ESuite(t *testing.T) {
	suite.Run(t, &vmSuite{Suite: NewSuite("my-test", &StackDefinition[MyEnv]{
		EnvCloudName: "aws/sandbox",
		EnvFactory: func(ctx *pulumi.Context) (*MyEnv, error) {
			vm, err := ec2vm.NewUnixLikeEc2VM(ctx, ec2vm.WithOS(os.AmazonLinuxOS, commonos.AMD64Arch))
			if err != nil {
				return nil, err
			}
			return &MyEnv{
				VM: client.NewVM(vm),
			}, nil
		},
	})})
}

func (v *vmSuite) Test1() {
	_, err := v.Env.VM.Execute("ls")
	require.NoError(v.T(), err)
}
