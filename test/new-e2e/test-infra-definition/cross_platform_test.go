// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testinfradefinition

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/require"
)

type crossPlatformSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestCrossPlatformSuite(t *testing.T) {
	e2e.Run(t, &crossPlatformSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoAgentNoFakeIntake()))
}

func (s *crossPlatformSuite) TestUbuntuOS() {
	os := os.UbuntuDefault
	s.subTestInstallAgentVersion(os)
	s.subTestInstallAgent(os)
}

func (s *crossPlatformSuite) TestWindows() {
	// As the Windows installer cannot downgrade an agent version,
	// the order of tests matter. New tests must be added at
	// the end of the function.
	os := os.WindowsDefault
	s.subTestInstallAgentVersion(os)
	s.subTestInstallAgent(os)
}

func (s *crossPlatformSuite) TestOtherOSES() {
	// TODO: There is a bug for CentOS and RockyLinux
	for _, os := range []os.Descriptor{os.AmazonLinuxDefault, os.DebianDefault, os.FedoraDefault, os.RedHatDefault, os.SuseDefault} {
		s.subTestInstallAgent(os)
	}
}

func (s *crossPlatformSuite) subTestInstallAgent(os os.Descriptor) {
	s.T().Run(fmt.Sprintf("Test install agent %v", os), func(t *testing.T) {
		s.UpdateEnv(awshost.Provisioner(awshost.WithEC2InstanceOptions(ec2.WithOS(os))))
		output := s.Env().Agent.Client.Status().Content
		require.Contains(s.T(), output, "Agent start")
	})
}

func (s *crossPlatformSuite) subTestInstallAgentVersion(os os.Descriptor) {
	for _, data := range []struct {
		version      string
		agentVersion string
	}{
		{"6.45.0", "Agent 6.45.0"},
		{"7.46.0~rc.2-1", "Agent 7.46.0-rc.2"},
	} {
		s.T().Run("Test install agent version "+data.version, func(t *testing.T) {
			s.UpdateEnv(awshost.Provisioner(awshost.WithEC2InstanceOptions(ec2.WithOS(os)), awshost.WithAgentOptions(agentparams.WithVersion(data.version))))

			version := s.Env().Agent.Client.Version()
			require.Contains(s.T(), version, data.agentVersion)
		})
	}
}
