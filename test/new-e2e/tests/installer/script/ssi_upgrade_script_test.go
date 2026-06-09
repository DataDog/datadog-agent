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
	// productionSSIScriptURL installs the current GA SSI stack. That stack predates
	// the systemd-managed ld.so.preload feature and ships an installer whose
	// `apm` command has no `instrument-start`/`instrument-stop` subcommands.
	productionSSIScriptURL = "https://install.datadoghq.com/scripts/install-ssi.sh"
)

// ssiUpgradeSuite verifies that upgrading a host that already has an older SSI
// stack (whose datadog-installer lacks the `apm instrument-start`/`instrument-stop`
// subcommands) to a newer build keeps host instrumentation working across a
// reboot, and never leaves a broken datadog-apm-inject.service behind. The
// systemd unit is optional: the installer falls back to direct /etc/ld.so.preload
// management when no installer on disk supports the instrument subcommands.
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

	// 1. Install the current GA SSI stack from production. This lands an older
	//    datadog-installer on disk — one whose `apm` command has no
	//    instrument-start/instrument-stop subcommands.
	s.installProductionSSI()
	s.host.AssertPackageInstalledByInstaller("datadog-apm-inject")
	s.assertLDPreloadInstrumented("after fresh GA install")

	// 2. Upgrade to the pipeline build and opt into systemd-managed preload.
	s.RunInstallScript(
		s.scriptURLPrefix+"install-ssi.sh",
		"DD_SITE=datadoghq.com",
		"DD_APM_INSTRUMENTATION_LIBRARIES=python:4",
		"DD_APM_INSTRUMENTATION_ENABLED=host",
		"DD_APM_INSTRUMENTATION_PRELOAD_MODE=systemd",
		"DD_NO_AGENT_INSTALL=true",
		"DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE=installtesting.datad0g.com.internal.dda-testing.com",
	)

	// 3. Host injection must be in place after the upgrade, and no broken
	//    apm-inject service may be left behind. The systemd unit is NOT a required
	//    post-condition: when no installer on disk supports the instrument
	//    subcommands (e.g. an older pinned installer), the installer deliberately
	//    skips the unit and manages /etc/ld.so.preload directly. Either way the
	//    launcher must be present and no unit may be left in a failed state.
	s.assertLDPreloadInstrumented("after upgrade")
	s.assertInjectServiceNotFailed("after upgrade")

	// 4. Reboot — the real guarantee. Host injection must survive, and if a unit
	//    was installed, its boot-time ExecStart must not leave it failed (exactly
	//    what a stale installer baked into ExecStart would cause).
	s.reboot()
	s.assertLDPreloadInstrumented("after reboot")
	s.assertInjectServiceNotFailed("after reboot")
}

// installProductionSSI installs the GA SSI stack directly from production, without
// any pipeline/testing registry overrides, so a genuinely older installer ends up
// on disk.
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

// assertInjectServiceNotFailed asserts the apm-inject systemd unit is not in a
// failed state. The unit is optional — the installer falls back to direct
// /etc/ld.so.preload management when no installer on disk supports the instrument
// subcommands — so its absence is fine. But a unit pointing at an installer that
// lacks `apm instrument-start` would fail its ExecStart and surface here.
func (s *ssiUpgradeSuite) assertInjectServiceNotFailed(when string) {
	// `systemctl is-failed` exits non-zero when the unit is not failed, so ignore
	// the error and inspect the state word. Absent units report "inactive"/"unknown".
	out, _ := s.Env().RemoteHost.Execute("systemctl is-failed " + apmInjectServiceName)
	require.NotEqualf(s.T(), "failed", strings.TrimSpace(out),
		"apm-inject service is in failed state (%s)", when)
}

// assertLDPreloadInstrumented asserts the injector launcher is present in
// /etc/ld.so.preload.
func (s *ssiUpgradeSuite) assertLDPreloadInstrumented(when string) {
	out := s.Env().RemoteHost.MustExecute("cat /etc/ld.so.preload || true")
	require.Containsf(s.T(), out, launcherPreloadPath,
		"injector launcher missing from /etc/ld.so.preload (%s); contents:\n%s", when, out)
}

// reboot reboots the host and waits for it to come back with a new boot id.
func (s *ssiUpgradeSuite) reboot() {
	before := strings.TrimSpace(s.Env().RemoteHost.MustExecute("cat /proc/sys/kernel/random/boot_id"))
	// The connection drops as the host goes down; ignore the error.
	_, _ = s.Env().RemoteHost.Execute("sudo reboot")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		if err := s.Env().RemoteHost.Reconnect(); err != nil {
			assert.NoError(c, err)
			return
		}
		out, err := s.Env().RemoteHost.Execute("cat /proc/sys/kernel/random/boot_id")
		if !assert.NoError(c, err) {
			return
		}
		assert.NotEqualf(c, before, strings.TrimSpace(out), "host has not rebooted yet")
	}, 5*time.Minute, 10*time.Second)
}

func requirePipeline(t *testing.T) {
	t.Helper()
	_, hasPipeline := os.LookupEnv("E2E_PIPELINE_ID")
	_, hasSHA := os.LookupEnv("CI_COMMIT_SHA")
	if !hasPipeline && !hasSHA {
		t.Skip("E2E_PIPELINE_ID / CI_COMMIT_SHA not set; this test upgrades to the current pipeline build")
	}
}
