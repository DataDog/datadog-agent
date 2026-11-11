// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentplatform

import (
	"encoding/base64"
	"fmt"
	"strings"
)

const (
	parScriptConfigPath        = "/etc/datadog-agent/private-action-runner/script-config.yaml"
	parScriptConfigExamplePath = parScriptConfigPath + ".example"
	// parScriptConfigCustom uses a script name that never appears in the packaged example.
	parScriptConfigCustom = `schemaId: script-credentials-v1
runPredefinedScript:
  e2e-native-upgrade-preserved:
    command:
      - echo
      - preserved
`
)

// TestPARScriptConfigPersists verifies that a customer-edited Private Action Runner script
// config survives a native DEB/RPM upgrade from the latest stable Agent to the pipeline build.
//
// Regression test for incident 61851: from 7.77.0 until 7.84.3 the packages owned the live
// script config, so every upgrade replaced customer edits with the packaged example. Packages
// now ship a `.example` template and only create the live file when it is missing.
func (is *persistingIntegrationsSuite) TestPARScriptConfigPersists() {
	VMclient := is.SetupTestClient()
	host := VMclient.Host

	startAgentVersion := is.SetupAgentStartVersion(VMclient)

	// Fresh install initializes the live config from the template.
	is.Require().NotEmpty(host.MustExecute("sudo cat "+parScriptConfigPath), "fresh install should create the live PAR script config")
	is.Require().NotEmpty(host.MustExecute("sudo cat "+parScriptConfigExamplePath), "package should ship the PAR script config template")

	// tee keeps the existing file's owner and mode.
	encoded := base64.StdEncoding.EncodeToString([]byte(parScriptConfigCustom))
	host.MustExecute(fmt.Sprintf("echo %s | base64 -d | sudo tee %s > /dev/null", encoded, parScriptConfigPath))
	permsBefore := host.MustExecute("sudo stat -c '%U:%G %a' " + parScriptConfigPath)

	upgradedAgentVersion := is.UpgradeAgentVersion(VMclient)
	is.Require().NotEqual(startAgentVersion, upgradedAgentVersion)

	is.Assert().Equal(strings.TrimSpace(parScriptConfigCustom), strings.TrimSpace(host.MustExecute("sudo cat "+parScriptConfigPath)),
		"PAR script config was modified by the upgrade")
	is.Assert().Equal(permsBefore, host.MustExecute("sudo stat -c '%U:%G %a' "+parScriptConfigPath),
		"PAR script config ownership or mode changed during the upgrade")
	is.Assert().NotEmpty(host.MustExecute("sudo cat "+parScriptConfigExamplePath), "upgraded package should still ship the PAR script config template")
}
