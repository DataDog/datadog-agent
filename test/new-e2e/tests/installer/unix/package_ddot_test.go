// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"fmt"
	"os"
	"path/filepath"

	e2eos "github.com/DataDog/test-infra-definitions/components/os"

	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/host"
)

type packageDDOTSuite struct {
	packageBaseSuite
}

func testDDOT(os e2eos.Descriptor, arch e2eos.Architecture, method InstallMethodOption) packageSuite {
	return &packageDDOTSuite{
		packageBaseSuite: newPackageSuite("ddot", os, arch, method, awshost.WithoutFakeIntake()),
	}
}

func (s *packageDDOTSuite) TestInstallDDOTWithAgent() {
	// Install agent and DDOT together via environment variable
	s.RunInstallScript("DD_REMOTE_UPDATES=true", "DD_OTEL_COLLECTOR_ENABLED=true", envForceInstall("datadog-agent"))
	defer s.Purge()

	// Verify both packages are installed
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.AssertPackageInstalledByInstaller("datadog-agent-ddot")

	// Wait for services to be active
	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit, ddotUnit)

	state := s.host.State()
	s.assertCoreUnits(state, false)
	s.assertDDOTUnits(state, false)

	// Verify configuration files exist
	state.AssertFileExists("/etc/datadog-agent/datadog.yaml", 0640, "dd-agent", "dd-agent")
	state.AssertFileExists("/etc/datadog-agent/otel-config.yaml", 0644, "dd-agent", "dd-agent")

	// Verify DDOT binary exists
	state.AssertFileExists("/opt/datadog-packages/datadog-agent-ddot/stable/embedded/bin/otel-agent", 0755, "dd-agent", "dd-agent")

	// Verify otelcollector configuration is present in datadog.yaml
	s.host.Run("sudo grep -q 'otelcollector:' /etc/datadog-agent/datadog.yaml")
}

func (s *packageDDOTSuite) TestInstallWithDDOT() {
	// Install datadog-agent (base infrastructure)
	s.RunInstallScript("DD_REMOTE_UPDATES=true", envForceInstall("datadog-agent"))
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit)

	// Install ddot
	s.host.Run(fmt.Sprintf("sudo datadog-installer install oci://installtesting.datad0g.com.internal.dda-testing.com/ddot-package:pipeline-%s", os.Getenv("E2E_PIPELINE_ID")))
	s.host.AssertPackageInstalledByInstaller("datadog-agent-ddot")

	// Check if datadog.yaml exists, if not return an error
	s.host.Run("sudo test -f /etc/datadog-agent/datadog.yaml || { echo 'Error: datadog.yaml does not exist'; exit 1; }")
	// Substitute API & site into otel-config.yaml
	s.host.Run("sudo sh -c \"sed -e 's/\\${env:DD_API_KEY}/XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX/' -e 's/\\${env:DD_SITE}/datadoghq.com/' /etc/datadog-agent/otel-config.yaml.example > /etc/datadog-agent/otel-config.yaml\"")

	s.host.WaitForUnitActive(s.T(), ddotUnit)

	state := s.host.State()
	// Verify running
	s.assertCoreUnits(state, false)
	s.assertDDOTUnits(state, false)

	// Verify files exist
	state.AssertFileExists("/etc/datadog-agent/datadog.yaml", 0640, "dd-agent", "dd-agent")
	state.AssertFileExists("/etc/datadog-agent/otel-config.yaml", 0644, "dd-agent", "dd-agent")

	state.AssertDirExists("/opt/datadog-packages/datadog-agent-ddot/stable", 0755, "dd-agent", "dd-agent")
	state.AssertFileExists("/opt/datadog-packages/datadog-agent-ddot/stable/embedded/bin/otel-agent", 0755, "dd-agent", "dd-agent")

	s.host.Run("sudo grep -q 'otelcollector:' /etc/datadog-agent/datadog.yaml")
}

func (s *packageDDOTSuite) assertCoreUnits(state host.State, oldUnits bool) {
	state.AssertUnitsLoaded(agentUnit, traceUnit, processUnit, probeUnit, securityUnit)
	state.AssertUnitsEnabled(agentUnit)
	state.AssertUnitsRunning(agentUnit, traceUnit) //cannot assert process-agent because it may be running or dead based on timing
	state.AssertUnitsDead(probeUnit, securityUnit)

	systemdPath := "/etc/systemd/system"
	if oldUnits {
		pkgManager := s.host.GetPkgManager()
		switch pkgManager {
		case "apt":
			if s.os.Flavor == e2eos.Ubuntu {
				// Ubuntu 24.04 moved to a new systemd path
				systemdPath = "/usr/lib/systemd/system"
			} else {
				systemdPath = "/lib/systemd/system"
			}
		case "yum", "zypper":
			systemdPath = "/usr/lib/systemd/system"
		default:
			s.T().Fatalf("unsupported package manager: %s", pkgManager)
		}
	}

	for _, unit := range []string{agentUnit, traceUnit, processUnit, probeUnit, securityUnit} {
		s.host.AssertUnitProperty(unit, "FragmentPath", filepath.Join(systemdPath, unit))
	}
}

// Verify ddot service running
func (s *packageDDOTSuite) assertDDOTUnits(state host.State, oldUnits bool) {
	state.AssertUnitsLoaded(ddotUnit)
	state.AssertUnitsRunning(ddotUnit)

	systemdPath := "/etc/systemd/system"
	if oldUnits {
		pkgManager := s.host.GetPkgManager()
		switch pkgManager {
		case "apt":
			if s.os.Flavor == e2eos.Ubuntu {
				systemdPath = "/usr/lib/systemd/system"
			} else {
				systemdPath = "/lib/systemd/system"
			}
		case "yum", "zypper":
			systemdPath = "/usr/lib/systemd/system"
		default:
			s.T().Fatalf("unsupported package manager: %s", pkgManager)
		}
	}

	s.host.AssertUnitProperty(ddotUnit, "FragmentPath", filepath.Join(systemdPath, ddotUnit))
}
