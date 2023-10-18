package wkit

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/params"
	"github.com/DataDog/test-infra-definitions/common/utils"
	commonos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type wkitVMSuite struct {
	e2e.Suite[e2e.VMEnv]
}

// you can use an environment variable or a file
const isLocalRun = false

var localVM client.VM

// GetVM returns a vm client that can execute commands
func (s *wkitVMSuite) GetVM() client.VM {
	if isLocalRun {
		return localVM
	}
	return s.Env().VM
}

func TestWKITVMSuite(t *testing.T) {
	e2e.Run(t, &wkitVMSuite{}, e2e.EC2VMStackDef(ec2params.WithOS(ec2os.WindowsOS)), params.WithLazyEnvironment())
}

func (s *wkitVMSuite) SetupSuite() {
	t := s.T()
	if isLocalRun {
		// init local vm here
		sshConnection := utils.Connection{
			Host: "localhost",
			User: "Admin",
		}
		vm, err := client.NewVMClient(t, &sshConnection, commonos.WindowsType)
		require.NoError(t, err)
		localVM = vm
	}
	s.Suite.SetupSuite()
}

func (s *wkitVMSuite) TestDir() {
	t := s.T()
	vm := s.GetVM()
	output := vm.Execute("dir")
	assert.NotEmpty(t, output)
}
