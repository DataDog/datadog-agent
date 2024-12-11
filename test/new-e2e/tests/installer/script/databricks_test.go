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
	state.AssertDirExists("/opt/datadog-packages/datadog-agent/7.57.2-1", 0755, "dd-agent", "dd-agent")
	state.AssertSymlinkExists("/opt/datadog-packages/datadog-agent/stable", "/opt/datadog-packages/datadog-agent/7.57.2-1", "root", "root")
}

func (s *installScriptDatabricksSuite) TestDatabricksDriverInstallScript() {
	s.RunInstallScript(s.url, "DB_IS_DRIVER=true")
	state := s.host.State()
	state.AssertDirExists("/opt/datadog-packages/datadog-agent/7.57.2-1", 0755, "dd-agent", "dd-agent")
	state.AssertSymlinkExists("/opt/datadog-packages/datadog-agent/stable", "/opt/datadog-packages/datadog-agent/7.57.2-1", "root", "root")
	state.AssertDirExists("/opt/datadog-packages/datadog-apm-inject/0.21.0", 0755, "root", "root")
	state.AssertSymlinkExists("/opt/datadog-packages/datadog-apm-inject/stable", "/opt/datadog-packages/datadog-apm-inject/0.21.0", "root", "root")
	state.AssertDirExists("/opt/datadog-packages/datadog-apm-library-java/1.41.1", 0755, "root", "root")
	state.AssertSymlinkExists("/opt/datadog-packages/datadog-apm-library-java/stable", "/opt/datadog-packages/datadog-apm-library-java/1.41.1", "root", "root")
}
