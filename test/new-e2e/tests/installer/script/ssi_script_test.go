// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installscript

import (
	e2eos "github.com/DataDog/test-infra-definitions/components/os"

	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
)

type installScriptSSISuite struct {
	installerScriptBaseSuite
	url string
}

func testSSIScript(os e2eos.Descriptor, arch e2eos.Architecture) installerScriptSuite {
	s := &installScriptSSISuite{
		installerScriptBaseSuite: newInstallerScriptSuite("installer-ssi", os, arch, awshost.WithoutFakeIntake(), awshost.WithoutAgent()),
	}
	s.url = s.scriptURLPrefix + "install-ssi.sh"

	return s
}

func (s *installScriptSSISuite) RunInstallScript(url string, params ...string) {
	params = append(params, "DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE=installtesting.datad0g.com.internal.dda-testing.com")
	s.installerScriptBaseSuite.RunInstallScript(url, params...)
}

func (s *installScriptSSISuite) TestInstall() {
	defer s.Purge()

	s.RunInstallScript(
		s.url,
		"DD_SITE=datadoghq.com",
		"DD_APM_INSTRUMENTATION_LIBRARIES=java:1,python:3,js:5,dotnet:3",
		"DD_APM_INSTRUMENTATION_ENABLED=host",
		"DD_PROFILING_ENABLED=true",
		"DD_NO_AGENT_INSTALL=true",
	)

	state := s.host.State()

	// Packages installed
	s.host.AssertPackageInstalledByInstaller(
		"datadog-apm-inject",
		"datadog-apm-library-java",
		"datadog-apm-library-python",
		"datadog-apm-library-js",
		"datadog-apm-library-dotnet",
	)
	s.host.AssertPackageNotInstalledByInstaller("datadog-agent")

	// Check the installer exists in /opt/datadog-packages/run
	state.AssertFileExists("/opt/datadog-packages/run/datadog-installer-ssi", 0755, "root", "root")

	// Config files exist
	state.AssertFileExists("/etc/datadog-agent/application_monitoring.yaml", 0644, "root", "root")
}
