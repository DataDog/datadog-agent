// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installscript

import (
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

const (
	databricksAgentVersion          = "7.74.0-1"
	databricksApmInjectVersion      = "0.53.0"
	databricksApmLibraryJavaVersion = "1.58.0"
)

type installScriptDatabricksSuite struct {
	installerScriptBaseSuite
	url string
}

func testDatabricksScript(os e2eos.Descriptor, arch e2eos.Architecture) installerScriptSuite {
	s := &installScriptDatabricksSuite{
		installerScriptBaseSuite: newInstallerScriptSuite("installer-databricks", os, arch, awshost.WithRunOptions(scenec2.WithoutFakeIntake()), awshost.WithRunOptions(scenec2.WithoutAgent())),
	}
	s.url = s.scriptURLPrefix + "install-databricks.sh"

	return s
}

func (s *installScriptDatabricksSuite) TestDatabricksWorkerInstallScript() {
	s.RunInstallScript(s.url)
	state := s.host.State()
	agentPath := "/opt/datadog-packages/datadog-agent/" + databricksAgentVersion
	state.AssertDirExists(agentPath, 0755, "dd-agent", "dd-agent")
	state.AssertSymlinkExists("/opt/datadog-packages/datadog-agent/stable", agentPath, "root", "root")

	state.AssertFileExists("/etc/datadog-agent/datadog.yaml", 0640, "dd-agent", "dd-agent")
}

func (s *installScriptDatabricksSuite) TestDatabricksDriverInstallScript() {
	s.RunInstallScript(s.url, "DB_IS_DRIVER=TRUE")
	state := s.host.State()
	agentPath := "/opt/datadog-packages/datadog-agent/" + databricksAgentVersion
	javaPath := "/opt/datadog-packages/datadog-apm-library-java/" + databricksApmLibraryJavaVersion
	injectPath := "/opt/datadog-packages/datadog-apm-inject/" + databricksApmInjectVersion

	state.AssertDirExists(agentPath, 0755, "dd-agent", "dd-agent")
	state.AssertSymlinkExists("/opt/datadog-packages/datadog-agent/stable", agentPath, "root", "root")
	state.AssertDirExists(injectPath, 0755, "root", "root")
	state.AssertSymlinkExists("/opt/datadog-packages/datadog-apm-inject/stable", injectPath, "root", "root")
	state.AssertDirExists(javaPath, 0755, "root", "root")
	state.AssertSymlinkExists("/opt/datadog-packages/datadog-apm-library-java/stable", javaPath, "root", "root")

	state.AssertFileExists("/etc/datadog-agent/datadog.yaml", 0640, "dd-agent", "dd-agent")
	state.AssertFileExists("/etc/datadog-agent/conf.d/spark.d/databricks.yaml", 0644, "dd-agent", "dd-agent")
	state.AssertFileExists("/etc/datadog-agent/application_monitoring.yaml", 0644, "dd-agent", "dd-agent")
}
