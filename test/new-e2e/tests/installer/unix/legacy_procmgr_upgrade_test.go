// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
)

const (
	legacyProcmgrUnit        = "datadog-agent-procmgrd.service"
	legacyProcmgrBaselineVer = "7.79.2"
)

type legacyProcmgrUpgradeSuite struct {
	packageBaseSuite
}

// TestLegacyProcmgrUpgradeContinuesOnRetirementFailure verifies RPM postinst warns and
// continues when legacy procmgrd units cannot be stopped during upgrade.
//
// The baseline agent (7.79.2) ships datadog-agent-procmgrd.service. Upgrade uses rpm
// --noscripts so prerm does not stop it before postinst. A systemctl wrapper makes stop
// fail with Access denied, matching the production failure mode.
func TestLegacyProcmgrUpgradeContinuesOnRetirementFailure(t *testing.T) {
	if _, ok := os.LookupEnv("E2E_PIPELINE_ID"); !ok {
		t.Skip("E2E_PIPELINE_ID env var is not set, this test requires a pipeline build")
	}
	if GetInstallMethodFromEnv(t) != InstallMethodInstallScript {
		t.Skip("install-script only scenario")
	}

	flavor := e2eos.RedHat9
	flavor.Architecture = e2eos.AMD64Arch
	suite := &legacyProcmgrUpgradeSuite{
		packageBaseSuite: newPackageSuite(
			"legacy_procmgr_upgrade",
			flavor,
			e2eos.AMD64Arch,
			InstallMethodInstallScript,
			awshost.WithRunOptions(scenec2.WithoutFakeIntake()),
		),
	}

	opts := []awshost.ProvisionerOption{
		awshost.WithRunOptions(
			scenec2.WithEC2InstanceOptions(scenec2.WithOSArch(flavor, flavor.Architecture)),
			scenec2.WithoutAgent(),
		),
	}
	opts = append(opts, suite.ProvisionerOptions()...)

	e2e.Run(t, suite,
		e2e.WithProvisioner(awshost.Provisioner(opts...)),
		e2e.WithStackName(suite.Name()),
	)
}

func (s *legacyProcmgrUpgradeSuite) TestLegacyProcmgrUpgradeContinuesOnRetirementFailure() {
	defer s.Purge()

	s.installLegacyProcmgrBaseline()

	_, err := s.Env().RemoteHost.Execute("systemctl cat " + legacyProcmgrUnit)
	require.NoErrorf(s.T(), err, "baseline agent %s must ship %s", legacyProcmgrBaselineVersion(), legacyProcmgrUnit)

	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit, legacyProcmgrUnit)

	postinstOutput := s.upgradeToPipelineRPM()
	require.Contains(s.T(), postinstOutput, "failed to retire legacy procmgr units")
	require.Contains(s.T(), postinstOutput, "Access denied")

	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit, legacyProcmgrUnit)
	state := s.host.State()
	state.AssertUnitsLoaded(legacyProcmgrUnit)
	s.host.Run("sudo datadog-agent status")
}

func legacyProcmgrBaselineVersion() string {
	if version := os.Getenv("STABLE_AGENT_ASSERT_VERSION"); version != "" {
		return version
	}
	return legacyProcmgrBaselineVer
}

func legacyProcmgrBaselineMinorVersion() string {
	return strings.TrimPrefix(legacyProcmgrBaselineVersion(), "7.")
}

func (s *legacyProcmgrUpgradeSuite) installLegacyProcmgrBaseline() {
	// Install pre-rename agent from production RPM repos (DD_AGENT_*_VERSION pins the release).
	// Do not use InstallScriptEnv: the pipeline installer would ship procmgr.service instead.
	env := map[string]string{
		"DD_API_KEY": GetAPIKey(),
		"DD_SITE":    "datadoghq.com",
	}
	cmd := fmt.Sprintf(
		`DD_AGENT_MAJOR_VERSION=7 DD_AGENT_MINOR_VERSION=%s bash -c "$(curl -L https://install.datadoghq.com/scripts/install_script_agent7.sh)"`,
		legacyProcmgrBaselineMinorVersion(),
	)
	_, err := s.Env().RemoteHost.Execute(cmd, client.WithEnvVariables(env))
	require.NoErrorf(s.T(), err, "failed to install datadog-agent %s baseline", legacyProcmgrBaselineVersion())
}

func (s *legacyProcmgrUpgradeSuite) upgradeToPipelineRPM() string {
	env := map[string]string{}
	installScriptPackageManagerEnv(env, s.arch)
	upgradeScript := `
set -e
ARCH=$(uname -m)
case "$ARCH" in
  aarch64) ARCHI=aarch64 ;;
  i686|i386|x86) ARCHI=i386 ;;
  *) ARCHI=x86_64 ;;
esac
sudo tee /etc/yum.repos.d/datadog.repo >/dev/null <<EOF
[datadog]
name = Datadog, Inc.
baseurl = https://${TESTING_YUM_URL}/${TESTING_YUM_VERSION_PATH}/${ARCHI}/
enabled = 1
gpgcheck = 1
repo_gpgcheck = 1
priority = 1
gpgkey = https://${TESTING_KEYS_URL}/DATADOG_RPM_KEY_CURRENT.public
       https://${TESTING_KEYS_URL}/DATADOG_RPM_KEY_E09422B3.public
       https://${TESTING_KEYS_URL}/DATADOG_RPM_KEY_FD4BF915.public
EOF
sudo yum -y clean metadata
cd /tmp
sudo yum -y --disablerepo='*' --enablerepo='datadog' download datadog-agent
sudo rpm -Uvh --noscripts /tmp/datadog-agent-*.rpm
sudo mkdir -p /tmp/e2e-systemctl-bin
sudo tee /tmp/e2e-systemctl-bin/systemctl >/dev/null <<'EOF'
#!/bin/bash
if [ "$1" = "stop" ]; then
  case "$2" in
  datadog-agent-procmgrd.service|datadog-agent-procmgrd-exp.service)
    echo "Failed to stop $2: Access denied" >&2
    exit 1
    ;;
  esac
fi
exec /usr/bin/systemctl "$@"
EOF
sudo chmod +x /tmp/e2e-systemctl-bin/systemctl
sudo runcon system_u:system_r:rpm_script_t:s0 /bin/bash -c \
  'export PATH=/tmp/e2e-systemctl-bin:$PATH; /opt/datadog-agent/embedded/bin/installer postinst datadog-agent rpm'
`
	output, err := s.Env().RemoteHost.Execute(upgradeScript, client.WithEnvVariables(env))
	require.NoError(s.T(), err, "pipeline RPM upgrade + postinst failed")
	return output
}
