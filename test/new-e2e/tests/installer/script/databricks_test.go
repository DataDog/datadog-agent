// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installscript

import (
	"fmt"
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	osdesc "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
)

type vmUpdaterSuite struct {
	commitHash string
	e2e.BaseSuite[environments.Host]
}

func (s *vmUpdaterSuite) TestInstallScript() {
	url := fmt.Sprintf("https://installtesting.datad0g.com/%s/scripts/install-databricks.sh", s.commitHash)

	// worker
	s.Env().RemoteHost.MustExecute(fmt.Sprintf("curl -L %s > install_script; export DD_API_KEY=test; export DD_INSTALLER_REGISTRY_URL_INSTALLER_PACKAGE=installtesting.datad0g.com; sudo -E bash install_script", url))
	s.Env().RemoteHost.MustExecute("sudo test -d /opt/datadog-packages/datadog-agent/7.57.2-1")

	// driver
	s.Env().RemoteHost.MustExecute(fmt.Sprintf("curl -L %s > install_script; export DB_IS_DRIVER=true; export DD_API_KEY=test; export DD_INSTALLER_REGISTRY_URL_INSTALLER_PACKAGE=installtesting.datad0g.com; sudo -E bash install_script", url))
	s.Env().RemoteHost.MustExecute("sudo test -d /opt/datadog-packages/datadog-agent/7.57.2-1")
	s.Env().RemoteHost.MustExecute("sudo test -d /opt/datadog-packages/datadog-apm-inject/0.21.0")
	s.Env().RemoteHost.MustExecute("sudo test -d /opt/datadog-packages/datadog-apm-library-java/1.41.1")
}

func TestUpdaterSuite(t *testing.T) {
	for _, arch := range []osdesc.Architecture{osdesc.AMD64Arch, osdesc.ARM64Arch} {
		e2e.Run(t, &vmUpdaterSuite{commitHash: os.Getenv("CI_COMMIT_SHA")}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(
			awshost.WithEC2InstanceOptions(ec2.WithOSArch(osdesc.UbuntuDefault, arch)),
		)))
	}
}
