// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentplatform

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
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
// It guards against reintroducing the package-owned live config behind incident 61851, where
// upgrades replaced customer edits with the packaged example. Upgrades from affected versions
// (7.77.0 to 7.84.2) still lose the config by design and are not covered.
func (is *persistingIntegrationsSuite) TestPARScriptConfigPersists() {
	if *flavorName != "datadog-agent" && *flavorName != "datadog-fips-agent" {
		is.T().Skipf("flavor %s does not ship the Private Action Runner", *flavorName)
	}

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

	// Root cause check: the package must own the template, never the live file. This also
	// catches a package marking the live file as a conffile/%config, which would keep this
	// upgrade's edit but drop it on a later upgrade or removal.
	is.assertNotOwnedByPackage(VMclient, parScriptConfigPath)
	is.assertOwnedByPackage(VMclient, parScriptConfigExamplePath)
}

// packageOwnerQuery returns the command that names the package owning a path.
func (is *persistingIntegrationsSuite) packageOwnerQuery(path string) string {
	if _, err := is.Env().RemoteHost.Execute("command -v dpkg-query"); err == nil {
		return "dpkg-query -S " + path
	}
	return "rpm -qf " + path
}

func (is *persistingIntegrationsSuite) assertNotOwnedByPackage(VMclient *common.TestClient, path string) {
	out, err := VMclient.Host.Execute(is.packageOwnerQuery(path))
	is.Assert().Error(err, "%s must not be owned by a package, got: %s", path, out)
}

func (is *persistingIntegrationsSuite) assertOwnedByPackage(VMclient *common.TestClient, path string) {
	out, err := VMclient.Host.Execute(is.packageOwnerQuery(path))
	is.Assert().NoError(err, "%s should be owned by the agent package", path)
	is.Assert().Contains(out, *flavorName, "%s should be owned by the agent package", path)
}
