// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installscript

import (
	"strings"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/host"
)

const (
	agentPackagePath   = "/opt/datadog-packages/datadog-agent"
	cTracerPackagePath = "/opt/datadog-packages/datadog-apm-library-c"
	// pinned versions must be published OCI package versions
	pinnedAgentMinorVersion = "86.0"
	pinnedAgentVersion      = "7.86.0-1"
	pinnedCTracerVersion    = "0.39.0"
	// 0.73.0 is the first injector that loads the C tracer from datadog-apm-library-c
	databricksGPUApmInjectVersion = "0.74.1"
)

type installScriptDatabricksGPUSuite struct {
	installerScriptBaseSuite
	url string
}

func testDatabricksGPUScript(os e2eos.Descriptor, arch e2eos.Architecture) installerScriptSuite {
	s := &installScriptDatabricksGPUSuite{
		installerScriptBaseSuite: newInstallerScriptSuite("installer-databricks-gpu", os, arch, awshost.WithRunOptions(scenec2.WithoutFakeIntake()), awshost.WithRunOptions(scenec2.WithoutAgent())),
	}
	s.url = s.scriptURLPrefix + "install-databricks-gpu.sh"

	return s
}

func (s *installScriptDatabricksGPUSuite) TestDatabricksGPUDriverInstallScript() {
	s.RunInstallScript(s.url, "DB_IS_DRIVER=TRUE")
	state := s.host.State()
	injectPath := "/opt/datadog-packages/datadog-apm-inject/" + databricksGPUApmInjectVersion

	// The Agent and the C tracer are not pinned: the script installs the
	// latest Agent 7 and the latest 0.x C tracer.
	agentStable := s.assertStableSymlink(state, agentPackagePath, "7.")
	state.AssertDirExists(agentStable, 0755, "dd-agent", "dd-agent")
	state.AssertDirExists(injectPath, 0755, "root", "root")
	state.AssertSymlinkExists("/opt/datadog-packages/datadog-apm-inject/stable", injectPath, "root", "root")
	cTracerStable := s.assertStableSymlink(state, cTracerPackagePath, "0.")
	state.AssertDirExists(cTracerStable, 0755, "root", "root")

	state.AssertFileExists("/etc/datadog-agent/datadog.yaml", 0640, "dd-agent", "dd-agent")
	state.AssertFileExists("/etc/datadog-agent/application_monitoring.yaml", 0644, "dd-agent", "dd-agent")
	appMonitoring, err := s.host.ReadFile("/etc/datadog-agent/application_monitoring.yaml")
	s.Require().NoError(err)
	s.Contains(string(appMonitoring), "DD_INJECT_NATIVE: always")
	s.Contains(string(appMonitoring), "DD_GPU_ENABLED: true")

	// GPU Monitoring only: no Data Jobs Monitoring setup
	state.AssertPathDoesNotExist("/opt/datadog-packages/datadog-apm-library-java")
	state.AssertPathDoesNotExist("/etc/datadog-agent/conf.d/spark.d/databricks.yaml")
}

func (s *installScriptDatabricksGPUSuite) TestDatabricksGPUPinnedVersionsInstallScript() {
	s.RunInstallScript(s.url, "DD_AGENT_MINOR_VERSION="+pinnedAgentMinorVersion, "DD_C_TRACER_VERSION="+pinnedCTracerVersion)
	state := s.host.State()
	agentPath := agentPackagePath + "/" + pinnedAgentVersion
	state.AssertDirExists(agentPath, 0755, "dd-agent", "dd-agent")
	state.AssertSymlinkExists(agentPackagePath+"/stable", agentPath, "root", "root")
	cPath := cTracerPackagePath + "/" + pinnedCTracerVersion
	state.AssertDirExists(cPath, 0755, "root", "root")
	state.AssertSymlinkExists(cTracerPackagePath+"/stable", cPath, "root", "root")
}

// assertStableSymlink checks that the package's stable symlink points to a
// version starting with versionPrefix, and returns the symlink target.
func (s *installScriptDatabricksGPUSuite) assertStableSymlink(state host.State, packagePath, versionPrefix string) string {
	stable, ok := state.FS[packagePath+"/stable"]
	s.Require().True(ok, "%s/stable does not exist", packagePath)
	s.Require().True(stable.IsSymlink, "%s/stable is not a symlink", packagePath)
	s.Require().True(strings.HasPrefix(stable.Link, packagePath+"/"+versionPrefix), "unexpected %s/stable target %s", packagePath, stable.Link)
	return stable.Link
}
