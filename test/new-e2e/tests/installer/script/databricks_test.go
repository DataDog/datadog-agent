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
	s.Env().RemoteHost.MustExecute(fmt.Sprintf("curl -L %s > install_script; export DD_API_KEY=test; export DD_INSTALLER_REGISTRY_URL_INSTALLER_PACKAGE=installtesting.datad0g.com; sudo -E bash install_script", url))
}

func TestUpdaterSuite(t *testing.T) {
	e2e.Run(t, &vmUpdaterSuite{commitHash: os.Getenv("CI_COMMIT_SHA")}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(
		awshost.WithEC2InstanceOptions(ec2.WithOSArch(osdesc.UbuntuDefault, osdesc.ARM64Arch)),
	)))
}
