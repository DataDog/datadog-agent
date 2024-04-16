// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package updater contains tests for the updater package
package updater

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	confDir        = "/etc/datadog-agent"
	logDir         = "/var/log/datadog"
	locksDir       = "/var/run/datadog-packages"
	packagesDir    = "/opt/datadog-packages"
	bootUpdaterDir = "/opt/datadog-installer"
)

type vmUpdaterSuite struct {
	e2e.BaseSuite[environments.Host]
	packageManager       string
	distro               os.Descriptor
	arch                 os.Architecture
	remoteUpdatesEnabled bool
}

func runTest(t *testing.T, pkgManager string, arch os.Architecture, distro os.Descriptor, remoteUpdatesEnabled bool) {
	reg := regexp.MustCompile(`[^a-zA-Z0-9_\-.]`)
	testName := reg.ReplaceAllString(distro.String()+"-"+string(arch), "_")
	e2e.Run(t, &vmUpdaterSuite{packageManager: pkgManager, distro: distro, arch: arch, remoteUpdatesEnabled: remoteUpdatesEnabled}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(
		awshost.WithUpdater(),
		awshost.WithEC2InstanceOptions(ec2.WithOSArch(distro, arch)),
		WithRemoteUpdatesEnabled(remoteUpdatesEnabled),
	)),
		e2e.WithStackName(testName),
	)
}

func TestCentOSARM(t *testing.T) {
	t.Parallel()
	runTest(t, "rpm", os.AMD64Arch, os.CentOSDefault, false)
}

func TestRedHatARM(t *testing.T) {
	t.Skip("Support for SELinux has not been added yet")
	t.Parallel()
	runTest(t, "rpm", os.ARM64Arch, os.RedHatDefault, false)
}

func TestUbuntuARMRemoteUpdates(t *testing.T) {
	t.Parallel()
	runTest(t, "dpkg", os.ARM64Arch, os.UbuntuDefault, true)
}

func TestUbuntuARM(t *testing.T) {
	t.Parallel()
	runTest(t, "dpkg", os.ARM64Arch, os.UbuntuDefault, false)
}

func TestDebianX86RemoteUpdates(t *testing.T) {
	t.Parallel()
	runTest(t, "dpkg", os.AMD64Arch, os.DebianDefault, true)
}

func TestDebianX86(t *testing.T) {
	t.Parallel()
	runTest(t, "dpkg", os.AMD64Arch, os.DebianDefault, false)
}

func (v *vmUpdaterSuite) TestUserGroupsCreation() {
	// users exist and is a system user
	require.Equal(v.T(), "/usr/sbin/nologin\n", v.Env().RemoteHost.MustExecute(`getent passwd dd-agent | cut -d: -f7`), "unexpected: user does not exist or is not a system user")
	require.Equal(v.T(), "/usr/sbin/nologin\n", v.Env().RemoteHost.MustExecute(`getent passwd dd-installer | cut -d: -f7`), "unexpected: user does not exist or is not a system user")
	require.Equal(v.T(), "dd-installer\n", v.Env().RemoteHost.MustExecute(`getent group dd-installer | cut -d":" -f1`), "unexpected: group does not exist")
	require.Equal(v.T(), "dd-agent\n", v.Env().RemoteHost.MustExecute(`getent group dd-agent | cut -d":" -f1`), "unexpected: group does not exist")
	require.Equal(v.T(), "dd-installer dd-agent\n", v.Env().RemoteHost.MustExecute("id -Gn dd-installer"), "dd-installer not in correct groups")
}

func (v *vmUpdaterSuite) TestSharedAgentDirs() {
	for _, dir := range []string{confDir, logDir} {
		require.Equal(v.T(), "dd-agent\n", v.Env().RemoteHost.MustExecute(`stat -c "%U" `+dir))
		require.Equal(v.T(), "dd-agent\n", v.Env().RemoteHost.MustExecute(`stat -c "%G" `+dir))
		require.Equal(v.T(), "drwxrwxr-x\n", v.Env().RemoteHost.MustExecute(`stat -c "%A" `+dir))
	}
}

func (v *vmUpdaterSuite) TestUpdaterDirs() {
	for _, dir := range []string{locksDir, packagesDir, bootUpdaterDir} {
		require.Equal(v.T(), "dd-installer\n", v.Env().RemoteHost.MustExecute(`stat -c "%U" `+dir))
		require.Equal(v.T(), "dd-installer\n", v.Env().RemoteHost.MustExecute(`stat -c "%G" `+dir))
	}
	require.Equal(v.T(), "drwxrwxrwx\n", v.Env().RemoteHost.MustExecute(`stat -c "%A" `+locksDir))
	require.Equal(v.T(), "drwxr-xr-x\n", v.Env().RemoteHost.MustExecute(`stat -c "%A" `+packagesDir))
}

func (v *vmUpdaterSuite) TestInstallerUnitLoaded() {
	t := v.T()
	if v.packageManager == "rpm" {
		t.Skip("FIXME(Paul): installer unit files disappear after bootstrap")
	}
	if v.remoteUpdatesEnabled {
		require.Equal(v.T(), "enabled\n", v.Env().RemoteHost.MustExecute(`systemctl is-enabled datadog-installer.service`))
	} else {
		output, err := v.Env().RemoteHost.Execute(`systemctl is-enabled datadog-installer.service`)
		require.Error(t, err)
		require.Contains(t, output, "Failed to get unit file state for datadog-installer.service: No such file or directory")
	}
}

func (v *vmUpdaterSuite) TestAgentUnitsLoaded() {
	t := v.T()
	stableUnits := []string{
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-process.service",
		"datadog-agent-sysprobe.service",
		"datadog-agent-security.service",
	}
	addEcrConfig(v.Env().RemoteHost)
	v.Env().RemoteHost.MustExecute(fmt.Sprintf(`sudo %v/bin/installer/installer bootstrap --url "oci://669783387624.dkr.ecr.us-east-1.amazonaws.com/dockerhub/datadog/agent-package-dev@sha256:d86138d88b407cf5ef75bccb3e0bc492ce6e3e3dfa9d3a64d2387d3b350fe5c4"`, bootUpdaterDir))
	for _, unit := range stableUnits {
		require.Equal(t, "enabled\n", v.Env().RemoteHost.MustExecute(fmt.Sprintf(`systemctl is-enabled %s`, unit)))
	}
}

func (v *vmUpdaterSuite) TestExperimentCrash() {
	t := v.T()
	host := v.Env().RemoteHost
	addEcrConfig(host)
	startTime := getMonotonicTimestamp(t, host)
	host.MustExecute(fmt.Sprintf(`sudo %v/bin/installer/installer bootstrap --url "oci://669783387624.dkr.ecr.us-east-1.amazonaws.com/dockerhub/datadog/agent-package-dev@sha256:d86138d88b407cf5ef75bccb3e0bc492ce6e3e3dfa9d3a64d2387d3b350fe5c4"`, bootUpdaterDir))
	v.Env().RemoteHost.MustExecute(`sudo systemctl start datadog-agent-exp --no-block`)
	res := getJournalDOnCondition(t, host, startTime, stopCondition([]JournaldLog{
		{Unit: "datadog-agent.service", Message: "Started"},
	}))
	require.True(t, verifyLogs(res, []JournaldLog{
		{Unit: "datadog-agent.service", Message: "Stopping"},
		{Unit: "datadog-agent.service", Message: "Stopped"},
		{Unit: "datadog-agent-exp.service", Message: "Starting"},
		{Unit: "datadog-agent-exp.service", Message: "Failed"},
		{Unit: "datadog-agent.service", Message: "Started"},
	}), fmt.Sprintf("unexpected logs: %v", res))
}

func (v *vmUpdaterSuite) TestPurgeAndInstallAgent() {
	// disable Debian due to flaky failures
	host := v.Env().RemoteHost
	if v.distro == os.DebianDefault {
		v.T().Skip("Skipping Debian as it fails")
	}

	addEcrConfig(host)
	host.MustExecute(fmt.Sprintf("sudo %v/bin/installer/installer purge", bootUpdaterDir))
	stableUnits := []string{
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-process.service",
		"datadog-agent-sysprobe.service",
		"datadog-agent-security.service",
	}
	for _, unit := range stableUnits {
		_, err := host.Execute(fmt.Sprintf(`systemctl is-enabled %s`, unit))
		require.Equal(
			v.T(),
			fmt.Sprintf("Failed to get unit file state for %s: No such file or directory\n: Process exited with status 1", unit),
			err.Error(),
		)
	}

	// dir exists
	host.MustExecute(`test -d /opt/datadog-packages`)
	// dir does not exist
	_, err := host.Execute(`test -d /opt/datadog-packages/datadog-agent`)
	require.NotNil(v.T(), err)

	// agent symlink does not exist
	_, err = host.Execute(`test -L /usr/bin/datadog-agent`)
	require.NotNil(v.T(), err)

	// install info files do not exist
	for _, file := range []string{"install_info", "install.json"} {
		exists, _ := host.FileExists(filepath.Join(confDir, file))
		assert.False(v.T(), exists)
	}

	// bootstrap
	host.MustExecute(fmt.Sprintf(`sudo %v/bin/installer/installer bootstrap --url "oci://669783387624.dkr.ecr.us-east-1.amazonaws.com/dockerhub/datadog/agent-package-dev@sha256:d86138d88b407cf5ef75bccb3e0bc492ce6e3e3dfa9d3a64d2387d3b350fe5c4"`, bootUpdaterDir))

	// assert agent symlink
	_ = host.MustExecute(`test -L /usr/bin/datadog-agent`)
	require.Equal(v.T(), "/usr/bin/datadog-agent\n", host.MustExecute("which datadog-agent"))
	binPath := host.MustExecute("readlink -f $(which datadog-agent)")
	assert.True(v.T(), strings.HasPrefix(binPath, "/opt/datadog-packages/datadog-agent/7."))
	assert.True(v.T(), strings.HasSuffix(binPath, "/bin/agent/agent\n"))

	// assert install info files
	for _, file := range []string{"install_info", "install.json"} {
		exists, _ := host.FileExists(filepath.Join(confDir, file))
		assert.True(v.T(), exists)
	}
	assertInstallMethod(v, v.T(), host)

	// assert file ownerships
	agentDir := "/opt/datadog-packages/datadog-agent"
	require.Equal(v.T(), "dd-installer\n", host.MustExecute(`stat -c "%U" `+agentDir))
	require.Equal(v.T(), "dd-installer\n", host.MustExecute(`stat -c "%G" `+agentDir))
	require.Equal(v.T(), "drwxr-xr-x\n", host.MustExecute(`stat -c "%A" `+agentDir))
	require.Equal(v.T(), "1\n", host.MustExecute(`sudo ls -l /opt/datadog-packages/datadog-agent | awk '$9 != "stable" && $3 == "dd-agent" && $4 == "dd-agent"' | wc -l`))

	// assert units
	for _, unit := range stableUnits {
		require.Equal(v.T(), "enabled\n", v.Env().RemoteHost.MustExecute(fmt.Sprintf(`systemctl is-enabled %s`, unit)))
	}
}

func (v *vmUpdaterSuite) TestPurgeAndInstallAPMInjector() {
	// Temporarily disable CentOS & Redhat, as there is a bug in the APM injector
	// disable Debian as well as it fails
	if v.distro == os.CentOSDefault {
		v.T().Skip("APM injector not available for CentOS yet")
	}
	if v.distro == os.DebianDefault {
		v.T().Skip("Skipping Debian as it fails")
	}

	host := v.Env().RemoteHost

	///////////////////
	// Setup machine //
	///////////////////
	addEcrConfig(host)
	host.MustExecute(fmt.Sprintf("sudo %v/bin/installer/installer purge", bootUpdaterDir))
	// Install docker
	installDocker(v.distro, v.arch, v.T(), host)
	defer func() {
		// Best effort to stop any running container at the end of the test
		host.Execute(`sudo docker ps -aq | xargs sudo docker stop | xargs sudo docker rm`)
	}()

	/////////////////////////
	// Check initial state //
	/////////////////////////

	// packages dir exists; but there are no packages installed
	host.MustExecute(`test -d /opt/datadog-packages`)
	_, err := host.Execute(`test -d /opt/datadog-packages/datadog-apm-inject`)
	require.NotNil(v.T(), err)
	_, err = host.Execute(`test -d /opt/datadog-packages/datadog-agent`)
	require.NotNil(v.T(), err)
	_, err = host.Execute(`test -d /opt/datadog-packages/datadog-apm-library-java`)
	require.NotNil(v.T(), err)

	// /etc/ld.so.preload does not contain the injector
	_, err = host.Execute(`grep "/opt/datadog-packages/datadog-apm-inject" /etc/ld.so.preload`)
	require.NotNil(v.T(), err)

	// docker daemon does not contain the injector
	_, err = host.Execute(`grep "/opt/datadog-packages/datadog-apm-inject" /etc/docker/daemon.json`)
	require.NotNil(v.T(), err)

	////////////////////////
	// Bootstrap packages //
	////////////////////////

	host.MustExecute(fmt.Sprintf(`sudo %v/bin/installer/installer bootstrap --url "oci://669783387624.dkr.ecr.us-east-1.amazonaws.com/dockerhub/datadog/agent-package-dev:7.54.0-devel.git.247.f92fbc1.pipeline.31778392-1"`, bootUpdaterDir))
	host.MustExecute(fmt.Sprintf(`sudo %v/bin/installer/installer bootstrap --url "oci://669783387624.dkr.ecr.us-east-1.amazonaws.com/dockerhub/datadog/apm-library-java-package-dev:1.32.0-SNAPSHOT-8708864e8e-pipeline.30373268.beta.8708864e-1"`, bootUpdaterDir))
	host.MustExecute(fmt.Sprintf(`sudo %v/bin/installer/installer bootstrap --url "oci://669783387624.dkr.ecr.us-east-1.amazonaws.com/dockerhub/datadog/apm-inject-package-dev:0.12.3-dev.bddec85.glci481808135.g8acdc698-1"`, bootUpdaterDir))

	////////////////////////////////
	// Check post-bootstrap state //
	////////////////////////////////

	// assert packages dir exist
	host.MustExecute(`test -L /opt/datadog-packages/datadog-agent/stable`)
	host.MustExecute(`test -L /opt/datadog-packages/datadog-apm-library-java/stable`)
	host.MustExecute(`test -L /opt/datadog-packages/datadog-apm-inject/stable`)

	// assert /etc/ld.so.preload contains the injector
	res, err := host.Execute(`grep "/opt/datadog-packages/datadog-apm-inject" /etc/ld.so.preload`)
	require.Nil(v.T(), err)
	require.Equal(v.T(), "/opt/datadog-packages/datadog-apm-inject/stable/inject/launcher.preload.so\n", res)

	// assert docker daemon contains the injector (removing blank spaces for easier comparison)
	res, err = host.Execute(`grep "/opt/datadog-packages/datadog-apm-inject" /etc/docker/daemon.json | sed -re 's/^[[:blank:]]+|[[:blank:]]+$//g' -e 's/[[:blank:]]+/ /g'`)
	require.Nil(v.T(), err)
	require.Equal(v.T(), "\"path\": \"/opt/datadog-packages/datadog-apm-inject/stable/inject/auto_inject_runc\"\n", res)

	// assert agent config has been changed
	raw, err := host.ReadFile("/etc/datadog-agent/datadog.yaml")
	require.Nil(v.T(), err)
	require.True(v.T(), strings.Contains(string(raw), "# BEGIN LD PRELOAD CONFIG"), "missing LD_PRELOAD config, config:\n%s", string(raw))

	// assert agent is running
	host.MustExecute("sudo systemctl status datadog-agent.service")

	_, err = host.Execute("sudo systemctl status datadog-agent-trace.service")
	require.Nil(v.T(), err)

	// assert required files exist
	requiredFiles := []string{
		"auto_inject_runc",
		"launcher.preload.so",
		"ld.so.preload",
		"musl-launcher.preload.so",
		"process",
	}
	for _, file := range requiredFiles {
		host.MustExecute(fmt.Sprintf("test -f /opt/datadog-packages/datadog-apm-inject/stable/inject/%s", file))
	}

	// assert file ownerships
	injectorDir := "/opt/datadog-packages/datadog-apm-inject"
	require.Equal(v.T(), "dd-installer\n", host.MustExecute(`stat -c "%U" `+injectorDir))
	require.Equal(v.T(), "dd-installer\n", host.MustExecute(`stat -c "%G" `+injectorDir))
	require.Equal(v.T(), "drwxr-xr-x\n", host.MustExecute(`stat -c "%A" `+injectorDir))
	require.Equal(v.T(), "1\n", host.MustExecute(`sudo ls -l /opt/datadog-packages/datadog-apm-inject | awk '$9 != "stable" && $3 == "dd-installer" && $4 == "dd-installer"' | wc -l`))

	/////////////////////////////////////
	// Check injection with a real app //
	/////////////////////////////////////

	launchJavaDockerContainer(v.T(), host)

	// check "Dropping Payload due to non-retryable error" in trace agent logs
	// as we don't have an API key the payloads can't be flushed successfully,
	// but this log indicates that the trace agent managed to receive the payload
	require.Eventually(v.T(), func() bool {
		_, err := host.Execute(`cat /var/log/datadog/trace-agent.log | grep "Dropping Payload due to non-retryable error"`)
		return err == nil
	}, 30*time.Second, 100*time.Millisecond)

	///////////////////////
	// Check purge state //
	///////////////////////

	host.MustExecute(fmt.Sprintf("sudo %v/bin/installer/installer purge", bootUpdaterDir))

	_, err = host.Execute(`test -d /opt/datadog-packages/datadog-apm-inject`)
	require.NotNil(v.T(), err)
	_, err = host.Execute(`test -d /opt/datadog-packages/datadog-agent`)
	require.NotNil(v.T(), err)
	_, err = host.Execute(`test -d /opt/datadog-packages/datadog-apm-library-java`)
	require.NotNil(v.T(), err)
	_, err = host.Execute(`grep "/opt/datadog-packages/datadog-apm-inject" /etc/ld.so.preload`)
	require.NotNil(v.T(), err)
	_, err = host.Execute(`grep "/opt/datadog-packages/datadog-apm-inject" /etc/docker/daemon.json`)
	require.NotNil(v.T(), err)
	res, err = host.Execute("grep \"LD PRELOAD CONFIG\" /etc/datadog-agent/datadog.yaml")
	require.NotNil(v.T(), err, "expected no LD PRELOAD CONFIG in agent config, got:\n%s", res)
}

func assertInstallMethod(v *vmUpdaterSuite, t *testing.T, host *components.RemoteHost) {
	rawYaml, err := host.ReadFile(filepath.Join(confDir, "install_info"))
	assert.Nil(t, err)
	var config Config
	require.Nil(t, yaml.Unmarshal(rawYaml, &config))

	assert.Equal(t, "updater_package", config.InstallMethod["installer_version"])
	assert.Equal(t, v.packageManager, config.InstallMethod["tool"])
	assert.True(t, "" != config.InstallMethod["tool_version"])
}

func addEcrConfig(host *components.RemoteHost) {
	host.MustExecute(fmt.Sprintf("cat %s/datadog.yaml | grep registry_auth || echo \"\nupdater:\n  registry_auth: ecr\" | sudo tee -a %s/datadog.yaml", confDir, confDir))
}

// Config yaml struct
type Config struct {
	InstallMethod map[string]string `yaml:"install_method"`
}

func WithRemoteUpdatesEnabled(enabled bool) awshost.ProvisionerOption {
	if enabled {
		return awshost.WithAgentOptions(func(p *agentparams.Params) error {
			p.ExtraAgentConfig = append(p.ExtraAgentConfig, pulumi.String("updater.remote_updates: true"))
			return nil
		})
	} else {
		return func(*awshost.ProvisionerParams) error { return nil }
	}
}
