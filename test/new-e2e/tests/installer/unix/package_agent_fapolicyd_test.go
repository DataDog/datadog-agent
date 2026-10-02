// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"os"
	"testing"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

// packageAgentFapolicydSuite runs the same install assertions as packageBaseSuite.testInstall,
// on the standard RedHat9 image, which has fapolicyd pre-baked (provision-e2e-rhel-centos.sh).
// It embeds packageBaseSuite directly rather than packageAgentSuite so that it doesn't promote
// packageAgentSuite's other Test* methods (experiment lifecycle, NSS-user, FIPS, ...) into its own
// run. It has its own stack/suite type per test/new-e2e/AGENTS.md so it doesn't share a Pulumi
// stack with packageAgentSuite, and it lives outside the TestPackages flavor matrix since it only
// ever targets RedHat9. The image is shared, but the VM and test state are isolated.
type packageAgentFapolicydSuite struct {
	packageBaseSuite
}

func TestPackageAgentFapolicyd(t *testing.T) {
	if _, ok := os.LookupEnv("E2E_PIPELINE_ID"); !ok {
		t.Log("E2E_PIPELINE_ID env var is not set, this test requires this variable to be set to work")
		t.FailNow()
	}
	t.Parallel()

	method := GetInstallMethodFromEnv(t)
	suite := &packageAgentFapolicydSuite{
		packageBaseSuite: newPackageSuite("agent-fapolicyd", e2eos.RedHat9, e2eos.AMD64Arch, method, awshost.WithRunOptions(ec2.WithoutFakeIntake())),
	}

	opts := []awshost.ProvisionerOption{
		awshost.WithRunOptions(
			ec2.WithEC2InstanceOptions(ec2.WithOSArch(e2eos.RedHat9, e2eos.AMD64Arch), ec2.WithInternetAccess()),
			ec2.WithoutAgent(),
		),
	}
	opts = append(opts, suite.ProvisionerOptions()...)
	e2e.Run(t, suite,
		e2e.WithProvisioner(awshost.Provisioner(opts...)),
		e2e.WithStackName(suite.Name()),
	)
}

func (s *packageAgentFapolicydSuite) TestInstallWithFapolicyd() {
	s.host.Run("rpm -q fapolicyd")
	s.testInstall()
}
