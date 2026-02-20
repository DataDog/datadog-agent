// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package suite contains a base suite for fleet tests
package suite

import (
	"os"
	"regexp"
	"slices"
	"testing"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/agent"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/backend"
	fleethost "github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/installer"
)

var (
	// LinuxPlatforms is the list of supported Linux platforms.
	LinuxPlatforms = []e2eos.Descriptor{
		e2eos.Ubuntu2404,
		e2eos.AmazonLinux2,
		e2eos.Debian12,
		e2eos.RedHat9,
		// e2eos.CentOS7,
		e2eos.Suse15,
	}
	// WindowsPlatforms is the list of supported Windows platforms.
	WindowsPlatforms = []e2eos.Descriptor{
		e2eos.WindowsServer2016,
		e2eos.WindowsServer2019,
		e2eos.WindowsServer2022,
		e2eos.WindowsServer2025,
	}
	// AllPlatforms is the list of all supported platforms.
	AllPlatforms = append(LinuxPlatforms, WindowsPlatforms...)
)

// Platforms returns the list of platforms to test, excluding Windows platforms
// when the SKIP_WINDOWS environment variable is set to "true".
func Platforms() []e2eos.Descriptor {
	if os.Getenv("SKIP_WINDOWS") == "true" {
		return LinuxPlatforms
	}
	return AllPlatforms
}

// FleetSuite is a base suite for fleet tests.
type FleetSuite struct {
	e2e.BaseSuite[environments.Host]

	Agent     *agent.Agent
	Backend   *backend.Backend
	Host      *fleethost.Host
	Installer *installer.Installer
}

// SetupSuite sets up the fleet suite.
func (s *FleetSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer s.CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	s.Agent = agent.New(s.T, s.Env())
	s.Backend = backend.New(s.T, s.Env())
	s.Host = fleethost.New(s.Env())
	s.Installer = installer.New(s.T, s.Env())
}

// Run runs the fleet suite for the given platforms.
func Run(t *testing.T, f func() e2e.Suite[environments.Host], platforms []e2eos.Descriptor, opts ...awshost.ProvisionerOption) {
	for _, platform := range platforms {
		s := f()
		t.Run(platform.String(), func(t *testing.T) {
			t.Parallel()
			name := regexp.MustCompile("[^a-zA-Z0-9]+").ReplaceAllString(t.Name(), "_")
			// clone opts and shadow it to avoid race condition when running in parallel
			opts := append(slices.Clone(opts), awshost.WithRunOptions(ec2.WithEC2InstanceOptions(ec2.WithOS(platform)), ec2.WithoutAgent()))
			e2e.Run(t, s, e2e.WithProvisioner(awshost.Provisioner(opts...)), e2e.WithStackName(name))
		})
	}
}
