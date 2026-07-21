// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installscript

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	installer "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/unix"
)

const (
	apmInjectServiceName = "datadog-apm-inject.service"
	// launcherPreloadPath is the line that must be present in /etc/ld.so.preload
	// for host instrumentation to be effective.
	launcherPreloadPath = "/opt/datadog-packages/datadog-apm-inject/stable/inject/launcher.preload.so"
	// productionSSIScriptURL installs the current GA SSI stack — the "older
	// version" we upgrade from.
	productionSSIScriptURL = "https://install.datadoghq.com/scripts/install-ssi.sh"
)

// ssiUpgradeSuite verifies that upgrading an SSI host from the current GA release
// to the build under test keeps host instrumentation healthy across a reboot.
//
// The datadog-apm-inject systemd unit (#49380) bakes a resolved datadog-installer
// path into its ExecStart (`<installer> apm instrument-start host`) and re-asserts
// /etc/ld.so.preload on every boot. If that path points at an installer that does
// not support the instrument subcommands, the unit fails its ExecStart on every
// boot. The reboot step below is what surfaces it: at install time the failure is
// masked by the direct ld.so.preload write, but on the next boot only the unit
// runs — so a broken unit shows up as a non-active service.
type ssiUpgradeSuite struct {
	installerScriptBaseSuite
}

// TestSSIUpgrade provisions a single host and runs the upgrade suite. The upgrade
// target is the current pipeline build, so a pipeline id (or commit sha) is required.
func TestSSIUpgrade(t *testing.T) {
	requirePipeline(t)

	flavor := e2eos.Ubuntu2404
	flavor.Architecture = e2eos.AMD64Arch

	suite := &ssiUpgradeSuite{
		installerScriptBaseSuite: newInstallerScriptSuite(
			"installer-ssi-upgrade", flavor, flavor.Architecture,
			awshost.WithRunOptions(ec2.WithoutFakeIntake()),
			awshost.WithRunOptions(ec2.WithoutAgent()),
		),
	}

	opts := []awshost.ProvisionerOption{
		awshost.WithRunOptions(
			ec2.WithEC2InstanceOptions(ec2.WithOSArch(flavor, flavor.Architecture)),
			ec2.WithoutAgent(),
		),
	}
	opts = append(opts, suite.ProvisionerOptions()...)

	e2e.Run(t, suite,
		e2e.WithProvisioner(awshost.Provisioner(opts...)),
		e2e.WithStackName(suite.Name()),
	)
}

func (s *ssiUpgradeSuite) TestUpgradePreservesHostInjection() {
	defer s.Purge()

	// 1. Install the current GA SSI stack — the older version we upgrade from.
	s.installProductionSSI()
	s.host.AssertPackageInstalledByInstaller("datadog-apm-inject")
	s.assertLDPreloadInstrumented()

	// 2. Upgrade to the build under test. On main the apm-inject systemd unit is
	//    set up unconditionally for host instrumentation on a systemd host, so no
	//    opt-in env var is needed.
	s.RunInstallScript(
		s.scriptURLPrefix+"install-ssi.sh",
		"DD_SITE=datadoghq.com",
		"DD_APM_INSTRUMENTATION_LIBRARIES=python:4",
		"DD_APM_INSTRUMENTATION_ENABLED=host",
		"DD_NO_AGENT_INSTALL=true",
		"DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE=installtesting.datad0g.com.internal.dda-testing.com",
	)

	// 3. Right after the upgrade, host injection must be in place. We do NOT assert
	//    the unit is active yet: its install-time start is best-effort by design
	//    (the direct ld.so.preload write covers the current boot, and the unit is
	//    expected to start on the next boot), so the after-reboot check below is the
	//    meaningful one.
	s.assertLDPreloadInstrumented()

	// 4. Reboot — the real check. On boot the unit's ExecStart
	//    (`<installer> apm instrument-start host`) is what re-asserts
	//    /etc/ld.so.preload. A unit whose ExecStart references an installer without
	//    `apm instrument-start` fails to start, so a non-active service after reboot
	//    is the regression.
	rebootAndWait(s.T(), s.Env().RemoteHost)
	s.assertInjectServiceActive()
	s.assertLDPreloadInstrumented()
}

// installProductionSSI installs the GA SSI stack directly from production, without
// any pipeline/testing registry overrides.
func (s *ssiUpgradeSuite) installProductionSSI() {
	s.Env().RemoteHost.MustExecute(fmt.Sprintf("curl -L %s > install_ssi_prod", productionSSIScriptURL))
	params := []string{
		"DD_API_KEY=" + installer.GetAPIKey(),
		"DD_SITE=datadoghq.com",
		"DD_APM_INSTRUMENTATION_LIBRARIES=python:4",
		"DD_APM_INSTRUMENTATION_ENABLED=host",
		"DD_NO_AGENT_INSTALL=true",
	}
	_, err := s.Env().RemoteHost.Execute(strings.Join(params, " ") + " bash install_ssi_prod")
	require.NoError(s.T(), err, "failed to install production SSI stack")
}

// assertInjectServiceActive asserts the apm-inject systemd unit is active. A unit
// whose ExecStart references an installer that lacks `apm instrument-start` fails
// to start and reports "failed" instead — which is exactly the regression we want
// this test to catch (most visibly after a reboot). On failure it dumps the unit
// status (including the baked ExecStart path) to make the cause unambiguous.
func (s *ssiUpgradeSuite) assertInjectServiceActive() {
	host := s.Env().RemoteHost
	var state string
	// `|| true` keeps the exit status 0 so we read the actual state word
	// ("active"/"failed"/"inactive") instead of an empty string.
	ok := assert.Eventually(s.T(), func() bool {
		state = strings.TrimSpace(host.MustExecute("systemctl is-active " + apmInjectServiceName + " || true"))
		return state == "active"
	}, 2*time.Minute, 5*time.Second)
	if !ok {
		require.Failf(s.T(), "apm-inject service not active",
			"%s is %q, not active.\n%s", apmInjectServiceName, state, s.injectorDiagnostics())
	}
}

// injectorDiagnostics gathers the on-host state needed to explain why the unit is
// not active: the unit file (if any), and for each candidate installer path
// whether it exists and exposes `apm instrument-start`. This distinguishes
// "no unit created" from "unit baked with an old installer".
func (s *ssiUpgradeSuite) injectorDiagnostics() string {
	const script = `
echo "=== unit file ==="
cat /etc/systemd/system/datadog-apm-inject.service 2>&1 || echo "(no unit file)"
echo "=== systemctl status ==="
systemctl status datadog-apm-inject.service --no-pager --full 2>&1 || true
echo "=== installer candidates (instrument-start support) ==="
for p in \
  /opt/datadog-packages/datadog-installer/stable/bin/installer/installer \
  /opt/datadog-packages/run/datadog-installer-ssi \
  /opt/datadog-packages/datadog-agent/stable/embedded/bin/installer \
  /usr/bin/datadog-installer; do
  if [ -x "$p" ]; then
    echo "[present] $p -> instrument-start lines: $("$p" apm --help 2>&1 | grep -c instrument-start)"
  else
    echo "[absent ] $p"
  fi
done
true`
	return s.Env().RemoteHost.MustExecute(script)
}

// assertLDPreloadInstrumented asserts the injector launcher is present in
// /etc/ld.so.preload.
func (s *ssiUpgradeSuite) assertLDPreloadInstrumented() {
	out := s.Env().RemoteHost.MustExecute("cat /etc/ld.so.preload || true")
	require.Containsf(s.T(), out, launcherPreloadPath,
		"injector launcher missing from /etc/ld.so.preload; contents:\n%s", out)
}

func requirePipeline(t *testing.T) {
	t.Helper()
	_, hasPipeline := os.LookupEnv("E2E_PIPELINE_ID")
	_, hasSHA := os.LookupEnv("CI_COMMIT_SHA")
	if !hasPipeline && !hasSHA {
		t.Skip("E2E_PIPELINE_ID / CI_COMMIT_SHA not set; this test upgrades to the current pipeline build")
	}
}
