// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installscript

import (
	"fmt"

	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	e2eos "github.com/DataDog/test-infra-definitions/components/os"
)

const (
	databricksAgentVersion          = "7.58.2-1"
	databricksApmInjectVersion      = "0.26.0"
	databricksApmLibraryJavaVersion = "1.42.2"
)

type installScriptDatabricksSuite struct {
	installerScriptBaseSuite
	url string
}

func testDatabricksScript(os e2eos.Descriptor, arch e2eos.Architecture) installerScriptSuite {
	s := &installScriptDatabricksSuite{
		installerScriptBaseSuite: newInstallerScriptSuite("installer", os, arch, awshost.WithoutFakeIntake(), awshost.WithoutAgent()),
	}
	s.url = fmt.Sprintf("https://installtesting.datad0g.com/%s/scripts/install-databricks.sh", s.commitHash)

	return s
}

func (s *installScriptDatabricksSuite) TestDatabricksWorkerInstallScript() {
	s.RunInstallScript(s.url)
	state := s.host.State()
	agentPath := fmt.Sprintf("/opt/datadog-packages/datadog-agent/%s", databricksAgentVersion)
	state.AssertDirExists(agentPath, 0755, "dd-agent", "dd-agent")
	state.AssertSymlinkExists("/opt/datadog-packages/datadog-agent/stable", agentPath, "root", "root")

	state.AssertFileExists("/etc/datadog-agent/datadog.yaml", 0640, "dd-agent", "dd-agent")
}

func (s *installScriptDatabricksSuite) TestDatabricksDriverInstallScript() {
	s.RunInstallScript(s.url, "DB_IS_DRIVER=TRUE")
	state := s.host.State()
	agentPath := fmt.Sprintf("/opt/datadog-packages/datadog-agent/%s", databricksAgentVersion)
	javaPath := fmt.Sprintf("/opt/datadog-packages/datadog-apm-library-java/%s", databricksApmLibraryJavaVersion)
	injectPath := fmt.Sprintf("/opt/datadog-packages/datadog-apm-inject/%s", databricksApmInjectVersion)

	state.AssertDirExists(agentPath, 0755, "dd-agent", "dd-agent")
	state.AssertSymlinkExists("/opt/datadog-packages/datadog-agent/stable", agentPath, "root", "root")
	state.AssertDirExists(injectPath, 0755, "root", "root")
	state.AssertSymlinkExists("/opt/datadog-packages/datadog-apm-inject/stable", injectPath, "root", "root")
	state.AssertDirExists(javaPath, 0755, "root", "root")
	state.AssertSymlinkExists("/opt/datadog-packages/datadog-apm-library-java/stable", javaPath, "root", "root")

	state.AssertFileExists("/etc/datadog-agent/datadog.yaml", 0640, "dd-agent", "dd-agent")
	state.AssertFileExists("/etc/datadog-agent/conf.d/spark.d/databricks.yaml", 0644, "dd-agent", "dd-agent")
	state.AssertFileExists("/etc/datadog-agent/inject/tracer.yaml", 0644, "root", "root")
}
