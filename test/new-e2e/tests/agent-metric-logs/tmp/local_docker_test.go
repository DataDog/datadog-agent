package tmp

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	dclocal "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/local"
)

type tmpSuite struct {
	e2e.BaseSuite[environments.DockerLocal]
}

func TestSimpleLocalAgentRun(t *testing.T) {
	e2e.Run(t, &tmpSuite{}, e2e.WithProvisioner(dclocal.Provisioner()))
}

func (d *tmpSuite) TestExecute() {
	d.T().Log("Running test")
	vm := d.Env().RemoteHost

	out, err := vm.Execute("whoami")
	d.Require().NoError(err)
	d.Require().NotEmpty(out)
}
