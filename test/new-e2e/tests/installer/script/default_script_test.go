// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installscript

import (
	"fmt"
	"os"
	"strings"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
	installer "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/unix"
)

type installScriptDefaultSuite struct {
	installerScriptBaseSuite
	url string
}

func testDefaultScript(os e2eos.Descriptor, arch e2eos.Architecture) installerScriptSuite {
	s := &installScriptDefaultSuite{
		installerScriptBaseSuite: newInstallerScriptSuite("installer-default", os, arch, awshost.WithRunOptions(scenec2.WithoutFakeIntake()), awshost.WithRunOptions(scenec2.WithoutAgent())),
	}
	s.url = s.scriptURLPrefix + "install.sh"

	return s
}

func (s *installScriptDefaultSuite) RunInstallScript(url string, params ...string) {
	params = append(params, "DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE=installtesting.datad0g.com.internal.dda-testing.com")
	s.installerScriptBaseSuite.RunInstallScript(url, params...)
}

func (s *installScriptDefaultSuite) TestInstall() {
	defer s.Purge()

	s.RunInstallScript(
		s.url,
		"DD_SITE=datadoghq.com",
		"DD_APM_INSTRUMENTATION_LIBRARIES=java:1,python:4,js:5,dotnet:3",
		"DD_APM_INSTRUMENTATION_ENABLED=host",
		"DD_RUNTIME_SECURITY_CONFIG_ENABLED=true",
		"DD_SBOM_CONTAINER_IMAGE_ENABLED=true",
		"DD_SBOM_HOST_ENABLED=true",
		"DD_REMOTE_UPDATES=true",
		"DD_ENV=env",
		"DD_HOSTNAME=hostname",
	)

	state := s.host.State()

	// Packages installed
	s.host.AssertPackageInstalledByInstaller(
		"datadog-agent",
		"datadog-apm-inject",
		"datadog-apm-library-java",
		"datadog-apm-library-python",
		"datadog-apm-library-js",
		"datadog-apm-library-dotnet",
	)

	// Config files exist
	state.AssertFileExists("/etc/datadog-agent/datadog.yaml", 0640, "dd-agent", "dd-agent")
	state.AssertFileExists("/etc/datadog-agent/system-probe.yaml", 0440, "dd-agent", "dd-agent")
	state.AssertFileExists("/etc/datadog-agent/security-agent.yaml", 0440, "dd-agent", "dd-agent")
	state.AssertPathDoesNotExist("/opt/datadog-packages/datadog-apm-library-ruby") // Not in DD_APM_INSTRUMENTATION_LIBRARIES

	// Units started
	state.AssertUnitsRunning(
		"datadog-agent.service",
		// "datadog-agent-installer.service", FIXME: uncomment when an agent+installer is released
		"datadog-agent-trace.service",
		"datadog-agent-sysprobe.service",
		"datadog-agent-security.service",
	)
}

// TestInstallParity tests that the installer install script with full options
// has the same behaviour as the agent 7 install script in terms of config & units started
func (s *installScriptDefaultSuite) TestInstallParity() {
	if _, ok := os.LookupEnv("E2E_PIPELINE_ID"); !ok {
		s.T().Skip("Skipping test due to missing E2E_PIPELINE_ID variable")
	}

	defer s.Purge()

	// Full supported option set
	params := []string{
		"DD_SITE=datadoghq.com",
		"DD_APM_INSTRUMENTATION_LIBRARIES=java:1,python:4,js:5,dotnet:3",
		"DD_APM_INSTRUMENTATION_ENABLED=host",
		"DD_RUNTIME_SECURITY_CONFIG_ENABLED=true",
		"DD_REMOTE_UPDATES=true",
		"DD_ENV=env",
		"DD_HOSTNAME=hostname",
	}

	s.RunInstallScript(s.url, params...)

	installerScriptConfigsRaw := map[string]string{
		"datadog.yaml":        s.Env().RemoteHost.MustExecute("sudo cat /etc/datadog-agent/datadog.yaml"),
		"system-probe.yaml":   s.Env().RemoteHost.MustExecute("sudo cat /etc/datadog-agent/system-probe.yaml"),
		"security-agent.yaml": s.Env().RemoteHost.MustExecute("sudo cat /etc/datadog-agent/security-agent.yaml"),
	}

	// Purge the agent & install using the agent 7 install script
	s.Purge()
	defer func() {
		s.Env().RemoteHost.Execute("sudo apt-get remove -y --purge datadog-installer || sudo yum remove -y datadog-installer || sudo zypper remove -y datadog-installer")
	}()
	if s.os.Flavor == e2eos.CentOS && s.os.Version == e2eos.CentOS7.Version {
		s.Env().RemoteHost.MustExecute("sudo systemctl daemon-reexec")
	}
	_, err := s.Env().RemoteHost.Execute(strings.Join(params, " ")+" bash -c \"$(curl -L https://dd-agent.s3.amazonaws.com/scripts/install_script_agent7.sh)\"", client.WithEnvVariables(map[string]string{
		"DD_API_KEY":               installer.GetAPIKey(),
		"TESTING_KEYS_URL":         "apttesting.datad0g.com/test-keys",
		"TESTING_APT_URL":          fmt.Sprintf("s3.amazonaws.com/apttesting.datad0g.com/datadog-agent/pipeline-%s-a7", os.Getenv("E2E_PIPELINE_ID")),
		"TESTING_APT_REPO_VERSION": fmt.Sprintf("stable-%s 7", s.arch),
		"TESTING_YUM_URL":          "s3.amazonaws.com/yumtesting.datad0g.com",
		"TESTING_YUM_VERSION_PATH": fmt.Sprintf("testing/pipeline-%s-a7/7", os.Getenv("E2E_PIPELINE_ID")),
	}))
	require.NoErrorf(s.T(), err, "installer not properly installed through install script")

	agent7ScriptConfigsRaw := map[string]string{
		"datadog.yaml":        s.Env().RemoteHost.MustExecute("sudo cat /etc/datadog-agent/datadog.yaml"),
		"system-probe.yaml":   s.Env().RemoteHost.MustExecute("sudo cat /etc/datadog-agent/system-probe.yaml"),
		"security-agent.yaml": s.Env().RemoteHost.MustExecute("sudo cat /etc/datadog-agent/security-agent.yaml"),
	}

	// Enforce that both sets of generated configs are the same
	for file, installerScriptConfigRaw := range installerScriptConfigsRaw {
		installerScriptConfig := map[string]interface{}{}
		require.NoError(s.T(), yaml.Unmarshal([]byte(installerScriptConfigRaw), &installerScriptConfig))

		agent7ConfigRaw := agent7ScriptConfigsRaw[file]
		agent7Config := map[string]interface{}{}
		require.NoError(s.T(), yaml.Unmarshal([]byte(agent7ConfigRaw), &agent7Config))

		for key, value := range agent7Config {
			if key != "api_key" {
				require.Equal(s.T(), value, installerScriptConfig[key], "config key %s in file %s differs", key, file)
			} else if installerScriptConfig[key] != value {
				s.T().Fatalf("config key api_key differs in file %s (not logging values)", file)
			}
		}
		require.Equal(s.T(), len(installerScriptConfig), len(agent7Config), "config lengths in file %s differs", file)
	}
}

// TestInstallIgnoreMajorMinor tests that the installer install script properly ignores
// the major / minor version when installing the agent
func (s *installScriptDefaultSuite) TestInstallIgnoreMajorMinor() {
	params := []string{
		"DD_API_KEY=" + installer.GetAPIKey(),
		"DD_REMOTE_UPDATES=true",
		"DD_AGENT_MAJOR_VERSION=7",
		"DD_AGENT_MINOR_VERSION=65.0",
	}
	defer s.Purge()
	s.RunInstallScript(s.url, params...)

	// Check the agent version is the latest one
	installedVersion := s.host.AgentStableVersion()
	assert.NotEqual(s.T(), "7.65.0", installedVersion, "agent version should not be 7.65.0")
}
