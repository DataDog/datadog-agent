// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testinfradefinition

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
	"github.com/stretchr/testify/require"
)

type crossPlatformSuite struct {
	e2e.Suite[e2e.AgentEnv]
}

func TestCrossPlatformSuite(t *testing.T) {
	e2e.Run(t, &crossPlatformSuite{}, e2e.AgentStackDef())
}

func (s *crossPlatformSuite) TestUbuntuOS() {
	os := ec2os.UbuntuOS
	s.subTestInstallAgentVersion(os)
	s.subTestInstallAgent(os)
}

func (s *crossPlatformSuite) TestWindows() {
	// As the Windows installer cannot downgrade an agent version,
	// the order of tests matter. New tests must be added at
	// the end of the function.
	os := ec2os.WindowsOS
	s.subTestInstallAgentVersion(os)
	s.subTestInstallAgent(os)
}

func (s *crossPlatformSuite) TestOtherOSES() {
	for _, os := range ec2os.GetOSTypes() {
		if os == ec2os.WindowsOS || os == ec2os.UbuntuOS {
			// Windows and Ubuntu have their dedicated tests
			continue
		}

		if os == ec2os.CentOS || os == ec2os.RockyLinux {
			// TODO: There is a bug for CentOS and RockyLinux
			continue
		}

		s.subTestInstallAgent(os)
	}
}

func (s *crossPlatformSuite) subTestInstallAgent(os ec2os.Type) {
	s.T().Run(fmt.Sprintf("Test install agent %v", os), func(t *testing.T) {
		s.UpdateEnv(e2e.AgentStackDef(e2e.WithVMParams(ec2params.WithOS(os))))
		output := s.Env().Agent.Status().Content
		require.Contains(s.T(), output, "Agent start")
	})
}

func (s *crossPlatformSuite) subTestInstallAgentVersion(os ec2os.Type) {
	for _, data := range []struct {
		version      string
		agentVersion string
	}{
		{"6.45.0", "Agent 6.45.0"},
		{"7.46.0~rc.2-1", "Agent 7.46.0-rc.2"},
	} {
		s.T().Run("Test install agent version "+data.version, func(t *testing.T) {
			s.UpdateEnv(e2e.AgentStackDef(e2e.WithVMParams(ec2params.WithOS(os)), e2e.WithAgentParams(agentparams.WithVersion(data.version))))

			version := s.Env().Agent.Version()
			require.Contains(s.T(), version, data.agentVersion)
		})
	}
}
