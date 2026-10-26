// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentplatform

import (
	"fmt"
	stdos "os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install/installparams"
)

// macosUpgradeSuite upgrades the latest released Agent to this pipeline's build, on a host of its
// own so the released Agent is installed on a clean machine.
type macosUpgradeSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestMacosInstallScriptUpgradeFromStable is named after TestMacosInstallScript so the macOS
// install e2e job, which selects tests with `--run TestMacosInstallScript`, runs it too.
func TestMacosInstallScriptUpgradeFromStable(t *testing.T) {
	if stdos.Getenv("E2E_PIPELINE_ID") == "" {
		t.Skip("E2E_PIPELINE_ID is not set: there is no pipeline build to upgrade the released Agent to")
	}
	extraConfigMap := runner.ConfigMap{}
	// Pulumi needs to pick a smaller subnet subset on macOS; only settable via the configmap.
	extraConfigMap.Set("ddinfra:aws/useMacosCompatibleSubnets", "true", false)
	e2e.Run(t, &macosUpgradeSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(ec2.WithEC2InstanceOptions(ec2.WithOS(os.MacOSDefault), ec2.WithInternetAccess()), ec2.WithoutAgent()),
			awshost.WithExtraConfigParams(extraConfigMap),
		)),
	)
}

func (m *macosUpgradeSuite) SetupSuite() {
	m.BaseSuite.SetupSuite()
	removePreInstalledAgent(m.T(), common.NewMacOSTestClient(m.Env().RemoteHost))
}

// TestUpgradeFromStable installs the latest released Agent with the public install script, then
// upgrades it to this pipeline's build with the pipeline's install script, as a customer would.
// The released Agent installs its LaunchDaemons from the plist examples in etc/; after the upgrade
// they must be the installer's:
//   - the installer postinst succeeded, with no fallback;
//   - each stable job definition is the one the package ships, root:wheel 644, and loaded;
//   - the Agent is healthy, and runs the upgraded version;
//   - the configuration in etc/ is kept;
//   - neither the installer daemon nor any experiment job is installed.
func (m *macosUpgradeSuite) TestUpgradeFromStable() {
	t := m.T()
	macosTestClient := common.NewMacOSTestClient(m.Env().RemoteHost)
	confFilePath := macosConfDefaultConfPath + "/datadog.yaml"
	const marker = "# added by e2e TestUpgradeFromStable"
	macosLogDiagnosticsOnFailure(t, macosTestClient)

	// An empty pipeline ID makes install.MacOS use the public install script and .dmg.
	install.MacOS(t, macosTestClient, installparams.WithPipelineID(""), installparams.WithUsername(m.Env().RemoteHost.Username), installparams.WithArch("x64"))
	require.False(t, t.Failed(), "installing the released Agent failed")
	waitForHealthyMacosAgent(t, macosTestClient, 60*time.Second)

	stableVersion := macosTestClient.MustExecuteOn(t, macosAgentBinary+" version")
	t.Logf("released Agent: %s", stableVersion)
	if _, err := macosTestClient.Execute("sudo test -e " + macosInstallerBinary); err == nil {
		// Not a failure: once a release ships the installer, this still tests an upgrade, but no
		// longer the one from LaunchDaemons installed from the plist examples.
		t.Logf("the released Agent already ships %s: this upgrade does not start from the legacy LaunchDaemons", macosInstallerBinary)
	}
	legacyDefinitions := map[string]string{}
	for _, label := range macosStableLaunchDaemons {
		if out, err := macosTestClient.Execute("sudo cat " + macosLaunchDaemonPath(label)); err == nil {
			legacyDefinitions[label] = out
		} else {
			t.Logf("the released Agent does not install %s", label)
		}
	}
	macosTestClient.MustExecuteOn(t, fmt.Sprintf("echo %q | sudo tee -a %s >/dev/null", marker, confFilePath))

	install.MacOS(t, macosTestClient, installparams.WithUsername(m.Env().RemoteHost.Username), installparams.WithArch("x64"))
	require.False(t, t.Failed(), "upgrading to the pipeline build failed")

	upgradedVersion := macosTestClient.MustExecuteOn(t, macosAgentBinary+" version")
	t.Logf("upgraded Agent: %s", upgradedVersion)
	assert.NotEqual(t, stableVersion, upgradedVersion, "the Agent should run the pipeline build after the upgrade")

	_, err := macosTestClient.Execute("sudo test -x " + macosInstallerBinary)
	require.NoError(t, err, "the installer binary is not shipped in the package")
	assertMacosInstallerPostinstSucceeded(t, macosTestClient)
	assertMacosStableLaunchDaemons(t, macosTestClient)
	for label, legacy := range legacyDefinitions {
		if legacy != macosReadRemoteFile(t, macosTestClient, macosLaunchDaemonPath(label)) {
			t.Logf("%s was replaced with the installer's definition", label)
		} else {
			t.Logf("%s is unchanged: the released definition already matched the installer's", label)
		}
	}
	waitForHealthyMacosAgent(t, macosTestClient, 60*time.Second)
	assert.Contains(t, macosReadRemoteFile(t, macosTestClient, confFilePath), marker, "the upgrade should keep datadog.yaml")
	assertNoMacosUnexpectedLaunchDaemons(t, macosTestClient)
}
