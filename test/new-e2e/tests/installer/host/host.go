// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package host provides a way to interact with an e2e remote host and capture its state.
package host

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
)

// Host is a remote host environment.
type Host struct {
	t              func() *testing.T
	remote         *components.RemoteHost
	os             e2eos.Descriptor
	arch           e2eos.Architecture
	systemdVersion int
	pkgManager     string
}

// Option is an option to configure a Host.
type Option func(func() *testing.T, *Host)

// New creates a new Host.
func New(t func() *testing.T, remote *components.RemoteHost, os e2eos.Descriptor, arch e2eos.Architecture, opts ...Option) *Host {
	host := &Host{
		t:      t,
		remote: remote,
		os:     os,
		arch:   arch,
	}
	for _, opt := range opts {
		opt(t, host)
	}
	if os.Family() == e2eos.LinuxFamily {
		host.uploadFixtures()
		host.setSystemdVersion()
		if _, err := host.remote.Execute("command -v dpkg-query"); err == nil {
			host.pkgManager = "apt"
		} else if _, err := host.remote.Execute("command -v zypper"); err == nil {
			host.pkgManager = "zypper"
		} else if _, err := host.remote.Execute("command -v yum"); err == nil {
			host.pkgManager = "yum"
		} else {
			t().Fatal("no package manager found")
		}
	}
	return host
}

// GetPkgManager returns the package manager of the host.
func (h *Host) GetPkgManager() string {
	return h.pkgManager
}

// Procmgr enabled returns true if the procmgr is enabled on the host, ie if the folder processes.d exists
func (h *Host) ProcmgrEnabled() bool {
	_, err := h.remote.ReadDir("/opt/datadog-packages/datadog-agent/stable/processes.d")
	if errors.Is(err, fs.ErrNotExist) {
		return false
	}
	require.NoError(h.t(), err)
	return true
}

// procmgr COAT telemetry gauge names, reported via `datadog-agent diagnose show-metadata agent-full-telemetry`.
const (
	metricProcmgrDaemonReachable        = "runtime__procmgr_daemon_reachable"
	metricProcmgrDaemonReady            = "runtime__procmgr_daemon_ready"
	metricProcmgrProcessRunning         = "runtime__procmgr_process_running"
	metricAgentServiceInstalled         = "runtime__agent_service_installed"
	metricAgentServiceProcmgrConfigured = "runtime__agent_service_procmgr_configured"
	metricAgentServiceManagementMode    = "runtime__agent_service_management_mode"
	procmgrManagementModeProcmgr        = "procmgr"
)

// AssertProcmgrTelemetry verifies the agent's COAT gauges report serviceID/processName as managed
// by dd-procmgrd. Call this after the process is confirmed running so procmgr is reachable.
func (h *Host) AssertProcmgrTelemetry(t *testing.T, serviceID, processName string) {
	t.Helper()

	// The procmgr reporter refreshes every 5 minutes; poll until gauges reflect the current state.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		out, err := h.remote.Execute("sudo datadog-agent diagnose show-metadata agent-full-telemetry")
		require.NoError(c, err)

		assertTelemetryGaugeTrue(c, out, metricProcmgrDaemonReachable, nil)
		assertTelemetryGaugeTrue(c, out, metricProcmgrDaemonReady, nil)
		assertTelemetryGaugeTrue(c, out, metricProcmgrProcessRunning, map[string]string{
			"process": processName,
		})
		assertTelemetryGaugeTrue(c, out, metricAgentServiceInstalled, map[string]string{
			"service": serviceID,
		})
		assertTelemetryGaugeTrue(c, out, metricAgentServiceProcmgrConfigured, map[string]string{
			"service": serviceID,
		})
		assertTelemetryGaugeTrue(c, out, metricAgentServiceManagementMode, map[string]string{
			"service": serviceID,
			"mode":    procmgrManagementModeProcmgr,
		})
	}, 7*time.Minute, 10*time.Second, "procmgr telemetry gauges should be emitted")
}

func assertTelemetryGaugeTrue(c *assert.CollectT, output, metric string, labels map[string]string) {
	c.Helper()

	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(trimmed, metric) {
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}
		value := fields[len(fields)-1]
		if value != "1" && value != "1.0" {
			continue
		}

		missingLabel := false
		for key, val := range labels {
			if !strings.Contains(trimmed, key+`="`+val+`"`) {
				missingLabel = true
				break
			}
		}
		if missingLabel {
			continue
		}

		return
	}

	if len(labels) == 0 {
		assert.Failf(c, "telemetry gauge not found", "expected %s with value 1", metric)
		return
	}
	assert.Failf(c, "telemetry gauge not found", "expected %s with labels %v and value 1", metric, labels)
}

func (h *Host) setSystemdVersion() {
	output := h.remote.MustExecute("systemctl --version | head -n1 | awk '{print $2}'")
	version, err := parseSystemdVersion(output)
	require.NoError(h.t(), err)
	h.systemdVersion = version
}

func parseSystemdVersion(output string) (int, error) {
	for _, line := range strings.Split(output, "\n") {
		version, err := strconv.Atoi(strings.TrimSpace(line))
		if err == nil {
			return version, nil
		}
	}
	return 0, fmt.Errorf("could not parse systemd version from %q", output)
}

// ConfigureAptMirrors hardens apt against package-mirror outages on apt-based hosts.
// It bounds apt's per-request timeout and retries so an unreachable (or merely slow)
// mirror fails within seconds instead of hanging until the CI job's 2h timeout, and on
// Ubuntu rewrites the apt sources to a "mirror+file" list so apt fails over to global
// mirrors when the regional EC2 mirror is down or degraded. us-east-1 is kept as the
// first entry (our infra's region), with archive.ubuntu.com / ports.ubuntu.com and a
// third-party mirror (mirror.leaseweb.net, which mirrors both amd64 and ports) as
// fallbacks, since ports.ubuntu.com alone is shared across every non-x86 architecture
// (arm64, armhf, ppc64el, s390x, riscv64) and is far less provisioned than the
// amd64-only archive.ubuntu.com global CDN. The timeout/retry values mirror
// Dockerfiles/agent/Dockerfile so CI image builds and e2e installs degrade identically.
// The source rewrite is skipped on releases whose apt lacks the "mirror+file" method
// driver (apt < 1.6, e.g. Ubuntu 16.04). See incidents 58780 and 59571.
func (h *Host) ConfigureAptMirrors() {
	if h.pkgManager != "apt" {
		return
	}
	// Fail fast when a mirror is unreachable or slow-but-dribbling instead of retrying
	// with long default TCP timeouts: Acquire::http::Timeout is a per-socket idle timeout,
	// so a short bound turns a stalled mirror into a fast error that triggers failover.
	h.remote.MustExecute(`printf 'Acquire::Retries "1";\nAcquire::http::Timeout "10";\nAcquire::https::Timeout "10";\n' | sudo tee /etc/apt/apt.conf.d/99datadog-e2e-fail-fast`)
	// Ubuntu EC2 AMIs point at a single regional mirror with no fallback; add global mirrors.
	if h.os.Flavor != e2eos.Ubuntu {
		return
	}
	// The "mirror+file" transport used below only exists in apt >= 1.6 (Ubuntu >= 18.04). On
	// older releases (e.g. Ubuntu 16.04, which ships apt 1.2) the method driver is absent, so
	// rewriting the sources to "mirror+file:" makes every subsequent apt operation fail with
	// "The method driver /usr/lib/apt/methods/mirror+file could not be found". Skip the source
	// rewrite there; the Acquire retry/timeout hardening above still applies. See incident 59571.
	if _, err := h.remote.Execute("test -e /usr/lib/apt/methods/mirror+file"); err != nil {
		return
	}
	h.remote.MustExecute(`printf 'http://us-east-1.ec2.archive.ubuntu.com/ubuntu\tpriority:1\nhttp://archive.ubuntu.com/ubuntu\nhttp://mirror.leaseweb.net/ubuntu\n' | sudo tee /etc/apt/mirrorlist.main`)
	h.remote.MustExecute(`printf 'http://us-east-1.ec2.ports.ubuntu.com/ubuntu-ports\tpriority:1\nhttp://ports.ubuntu.com/ubuntu-ports\nhttp://mirror.leaseweb.net/ubuntu-ports\n' | sudo tee /etc/apt/mirrorlist.ports`)
	h.remote.MustExecute(`for f in /etc/apt/sources.list /etc/apt/sources.list.d/ubuntu.sources; do if [ -f "$f" ]; then sudo sed -i -e 's#https\?://[a-z0-9.-]*ec2\.archive\.ubuntu\.com\S*#mirror+file:/etc/apt/mirrorlist.main#g' -e 's#https\?://archive\.ubuntu\.com\S*#mirror+file:/etc/apt/mirrorlist.main#g' -e 's#https\?://security\.ubuntu\.com\S*#mirror+file:/etc/apt/mirrorlist.main#g' -e 's#https\?://[a-z0-9.-]*ec2\.ports\.ubuntu\.com\S*#mirror+file:/etc/apt/mirrorlist.ports#g' -e 's#https\?://ports\.ubuntu\.com\S*#mirror+file:/etc/apt/mirrorlist.ports#g' "$f"; fi; done`)
}

// ConfigureYumMirrors is the yum counterpart to ConfigureAptMirrors. CentOS 7 is EOL and its
// stock /centos/7/ path on vault.centos.org now returns HTTP 403, breaking any "yum install".
// It repoints base/updates/extras at the versioned vault archive (7.9.2009), with the CERN and
// kernel.org vault mirrors as ordered fallbacks (failovermethod=priority) plus skip_if_unavailable
// and a bounded timeout to fail fast. vault's http path matches the agent's production
// kernel-header downloader (pkg/util/kernel/headers/download/rpm/centos.go). See incident 58780.
func (h *Host) ConfigureYumMirrors() {
	if h.pkgManager != "yum" {
		return
	}
	// Only CentOS 7 needs this — its EOL content lives at vault.centos.org/7.9.2009. Other yum
	// distros (RHEL, Amazon Linux) and CentOS 6 (different vault tree) are left untouched.
	if h.os.Flavor != e2eos.CentOS || !strings.HasPrefix(h.os.Version, "7") {
		return
	}
	h.remote.MustExecute(`printf '[base]\nname=CentOS-7 - Base\nbaseurl=http://vault.centos.org/7.9.2009/os/$basearch/\n        https://linuxsoft.cern.ch/centos-vault/7.9.2009/os/$basearch/\n        https://archive.kernel.org/centos-vault/7.9.2009/os/$basearch/\nfailovermethod=priority\ngpgcheck=1\ngpgkey=file:///etc/pki/rpm-gpg/RPM-GPG-KEY-CentOS-7\nskip_if_unavailable=1\ntimeout=30\n\n[updates]\nname=CentOS-7 - Updates\nbaseurl=http://vault.centos.org/7.9.2009/updates/$basearch/\n        https://linuxsoft.cern.ch/centos-vault/7.9.2009/updates/$basearch/\n        https://archive.kernel.org/centos-vault/7.9.2009/updates/$basearch/\nfailovermethod=priority\ngpgcheck=1\ngpgkey=file:///etc/pki/rpm-gpg/RPM-GPG-KEY-CentOS-7\nskip_if_unavailable=1\ntimeout=30\n\n[extras]\nname=CentOS-7 - Extras\nbaseurl=http://vault.centos.org/7.9.2009/extras/$basearch/\n        https://linuxsoft.cern.ch/centos-vault/7.9.2009/extras/$basearch/\n        https://archive.kernel.org/centos-vault/7.9.2009/extras/$basearch/\nfailovermethod=priority\ngpgcheck=1\ngpgkey=file:///etc/pki/rpm-gpg/RPM-GPG-KEY-CentOS-7\nskip_if_unavailable=1\ntimeout=30\n' | sudo tee /etc/yum.repos.d/CentOS-Base.repo > /dev/null`)
	// Drop metadata cached against the previous (403ing) repo config so the next yum call uses the new mirror.
	h.remote.MustExecute("sudo yum clean all")
}

// dockerImage returns the ECR pull-through URL for ecrPath when a registry is configured,
// or publicFallback otherwise.
func (h *Host) dockerImage(ecrPath, publicFallback string) string {
	reg, _ := runner.GetProfile().ParamStore().GetWithDefault(parameters.ImagePullRegistry, "")
	if reg == "" {
		return publicFallback
	}
	return strings.SplitN(reg, ",", 2)[0] + "/" + ecrPath
}

// configureDockerECRCredentialHelper writes credsStore: ecr-login into root's Docker config,
// enabling automatic IAM-role-based authentication for ECR registries (including pull-through caches).
// The amazon-ecr-credential-helper binary must already be on PATH.
func (h *Host) configureDockerECRCredentialHelper() {
	h.remote.MustExecute(`sudo mkdir -p /root/.docker && printf '{"credsStore":"ecr-login"}\n' | sudo tee /root/.docker/config.json > /dev/null`)
}

// installECRCredentialHelper installs the amazon-ecr-credential-helper binary if not already present.
func (h *Host) installECRCredentialHelper() {
	if _, err := h.remote.Execute("command -v docker-credential-ecr-login"); err == nil {
		return
	}
	if h.pkgManager == "apt" {
		h.remote.MustExecute("sudo apt-get install -y amazon-ecr-credential-helper")
	} else {
		// No official amazon-ecr-credential-helper package for non-apt distros (zypper, yum on CentOS, etc.);
		// download the binary directly.
		var helperArch string
		helperVersion := "0.12.0"
		switch h.arch {
		case e2eos.AMD64Arch:
			helperArch = "amd64"
		case e2eos.ARM64Arch:
			helperArch = "arm64"
		default:
			h.t().Fatalf("unsupported architecture for ECR credential helper: %s", h.arch)
		}
		helperURL := fmt.Sprintf("https://amazon-ecr-credential-helper-releases.s3.us-east-2.amazonaws.com/%s/linux-%s/docker-credential-ecr-login", helperVersion, helperArch)
		h.remote.MustExecute(fmt.Sprintf(`sudo curl -fsSL "%s" -o /usr/bin/docker-credential-ecr-login && sudo chmod +x /usr/bin/docker-credential-ecr-login`, helperURL))
	}
}

// TODO[@agent-devx]: Probably move this to the proper docker component defined in components/docker/component.go
// InstallDocker installs Docker on the host if it is not already installed.
func (h *Host) InstallDocker() {
	defer func() {
		// This defer will basically restart docker from a clean state, to avoid any issues in between tests.
		// It will:
		// - 1. Stop docker (if it's running)
		// - 2. Reset failed status
		// - 3. Remove the network directory to avoid network collision
		// - 4. Start docker again
		_, _ = h.remote.Execute("sudo systemctl stop docker")
		_, err := h.remote.Execute("sudo systemctl reset-failed docker")
		if err != nil {
			h.t().Logf("warn: failed to reset-failed for docker.d: %v", err)
		}
		_, err = h.remote.Execute("sudo rm -rf /var/lib/docker/network")
		if err != nil {
			h.t().Logf("warn: failed to remove /var/lib/docker/network: %v", err)
		}
		_, err = h.remote.Execute("sudo systemctl start docker")
		require.NoErrorf(h.t(), err, "failed to start Docker, logs: %s", h.remote.MustExecute("sudo journalctl -xeu docker"))
	}()
	if _, err := h.remote.Execute("command -v docker"); err == nil {
		h.installECRCredentialHelper()
		h.configureDockerECRCredentialHelper()
		return
	}

	switch h.pkgManager {
	case "apt":
		h.remote.MustExecute("sudo apt-get update -qq")
		h.remote.MustExecute("sudo apt-get install -y docker.io")
	case "yum":
		h.remote.MustExecute("sudo yum install -y docker")
	case "zypper":
		h.remote.MustExecute("sudo zypper install -y docker")
	default:
		h.t().Fatalf("unsupported package manager: %s", h.pkgManager)
	}

	h.installECRCredentialHelper()
	h.configureDockerECRCredentialHelper()
}

// GetDockerRuntimePath returns the runtime path of a docker runtime
func (h *Host) GetDockerRuntimePath(runtime string) string {
	if _, err := h.remote.Execute("command -v docker"); err != nil {
		return ""
	}

	cmd := "sudo docker system info --format '{{ (index .Runtimes \"%s\").Path }}'"
	return strings.TrimSpace(h.remote.MustExecute(fmt.Sprintf(cmd, runtime)))
}

// Run executes a command on the host.
func (h *Host) Run(command string, env ...string) string {
	envVars := make(map[string]string)
	for _, e := range env {
		parts := strings.Split(e, "=")
		envVars[parts[0]] = parts[1]
	}
	return h.remote.MustExecute(command, client.WithEnvVariables(envVars))
}

// UserExists checks if a user exists on the host.
func (h *Host) UserExists(username string) bool {
	_, err := h.remote.Execute("id -u " + username)
	return err == nil
}

// GroupExists checks if a group exists on the host.
func (h *Host) GroupExists(groupname string) bool {
	_, err := h.remote.Execute("id -g " + groupname)
	return err == nil
}

// FileExists checks if a file exists on the host.
func (h *Host) FileExists(path string) (bool, error) {
	return h.remote.FileExists(path)
}

// ReadFile reads a file from the host.
func (h *Host) ReadFile(path string) ([]byte, error) {
	return h.remote.ReadFile(path)
}

// WriteFile writes a file to the host.
func (h *Host) WriteFile(path string, content []byte) error {
	_, err := h.remote.WriteFile(path, content)
	return err
}

// DeletePath deletes a path on the host.
func (h *Host) DeletePath(path string) {
	h.remote.MustExecute("sudo ls " + path)
	h.remote.MustExecute("sudo rm -rf " + path)
}

// WaitForUnitActive waits for a systemd unit to be active
func (h *Host) WaitForUnitActive(t *testing.T, units ...string) {
	for _, unit := range units {
		assert.Eventually(t, func() bool {
			_, err := h.remote.Execute("systemctl is-active --quiet " + unit)

			return err == nil
		}, time.Second*90, time.Second*2, "unit %s did not become active. logs: %s", unit, h.remote.MustExecute("sudo journalctl -xeu "+unit))
	}
}

// WaitForProcessesRunning waits for procmgr-supervised processes to report a Running state.
// Unlike State.AssertProcessesRunning (a static snapshot check with no retry), this polls
// dd-procmgrd directly: becoming Running takes a few extra hops after the hosting systemd unit
// itself is active (daemon init, processes.d read, condition_path_exists check, process spawn), so
// a plain WaitForUnitActive on the procmgr unit is not enough to avoid a race with State().
func (h *Host) WaitForProcessesRunning(t *testing.T, names ...string) {
	const procmgrBin = "/opt/datadog-packages/datadog-agent/stable/embedded/bin/dd-procmgr"
	for _, rawName := range names {
		name := processName(rawName)
		require.EventuallyWithT(t, func(c *assert.CollectT) {
			out, err := h.remote.Execute("sudo -u dd-agent " + procmgrBin + " describe --json " + name)
			if !assert.NoError(c, err, "dd-procmgr describe failed for process %s", name) {
				return
			}
			var detail struct {
				State string `json:"state"`
			}
			if !assert.NoError(c, json.Unmarshal([]byte(out), &detail), "failed to parse dd-procmgr describe output for %s", name) {
				return
			}
			assert.Equal(c, "Running", detail.State, "process %s is not running", name)
		}, 2*time.Minute, 5*time.Second)
	}
}

// WaitForUnitActivating waits for a systemd unit to be activating
func (h *Host) WaitForUnitActivating(t *testing.T, units ...string) {
	for _, unit := range units {
		assert.Eventually(t, func() bool {
			_, err := h.remote.Execute(fmt.Sprintf("grep -q \"Active: activating\" <(sudo systemctl status %s)", unit))
			return err == nil
		},
			time.Second*90,
			time.Second*2,
			"unit %s did not become activating. installer logs:\n%s\n\ninstaller exp logs:\n%sunit %s logs:\n%s",
			unit,
			h.remote.MustExecute("sudo journalctl -xeu datadog-installer"),
			h.remote.MustExecute("sudo journalctl -xeu datadog-installer-exp"),
			unit,
			h.remote.MustExecute("sudo journalctl -xeu "+unit),
		)
	}
}

// WaitForUnitExited waits for a systemd unit to exit with a specific exit code
func (h *Host) WaitForUnitExited(t *testing.T, exitCode int, units ...string) {
	for _, unit := range units {
		assert.Eventually(t, func() bool {
			_, err := h.remote.Execute(fmt.Sprintf("systemctl show -p ExecMainCode -p ExecMainStatus %[2]s | xargs | grep -q 'ExecMainCode=1 ExecMainStatus=%[1]d'", exitCode, unit))
			return err == nil
		}, time.Second*90, time.Second*2, "unit %s did not exit or exit with expected code. logs: %s", unit, h.remote.MustExecute("sudo journalctl -xeu "+unit))
	}
}

// WaitForFileExists waits for a file to exist on the host
func (h *Host) WaitForFileExists(useSudo bool, filePaths ...string) {
	sudo := ""
	if useSudo {
		sudo = "sudo"
	}

	for _, path := range filePaths {
		_, err := h.remote.Execute(fmt.Sprintf("timeout=30; file=%s; while [ ! %s -f $file ] && [ $timeout -gt 0 ]; do sleep 1; ((timeout--)); done; [ $timeout -ne 0 ]", path, sudo))
		require.NoError(h.t(), err, "file %s did not exist", path)
	}
}

// WaitForTraceAgentSocketReady waits for the trace agent to be ready to receive traces
// This is because of a race condition where the trace agent is not ready to receive traces and we send them
// meaning that the traces are lost
func (h *Host) WaitForTraceAgentSocketReady() {
	require.EventuallyWithT(h.t(), func(t *assert.CollectT) {
		// this endpoint is no-op but it will fail if the trace agent is not ready
		_, err := h.remote.Execute("curl -XGET --unix-socket /var/run/datadog/apm.socket http:/services")
		require.NoError(t, err)
	}, 30*time.Second, 1*time.Second, "trace agent did not become ready")
}

// BootstrapperVersion returns the version of the bootstrapper on the host.
func (h *Host) BootstrapperVersion() string {
	return strings.TrimSpace(h.remote.MustExecute("sudo datadog-bootstrap version"))
}

// InstallerVersion returns the version of the installer on the host.
func (h *Host) InstallerVersion() string {
	return strings.TrimSpace(h.remote.MustExecute("sudo datadog-installer version"))
}

// AgentStableVersion returns the stable version of the agent on the host.
func (h *Host) AgentStableVersion() string {
	path := strings.TrimSpace(h.remote.MustExecute(`readlink /opt/datadog-packages/datadog-agent/stable`))
	return filepath.Base(path)
}

// AssertPackageInstalledByInstaller checks if a package is installed by the installer on the host.
func (h *Host) AssertPackageInstalledByInstaller(pkgs ...string) {
	for _, pkg := range pkgs {
		_, err := h.remote.ReadDir(fmt.Sprintf("/opt/datadog-packages/%s/stable/", pkg))
		require.NoErrorf(
			h.t(),
			err,
			"package %s not installed by the installer (err)",
			pkg,
		)
	}
}

// AssertPackageNotInstalledByInstaller checks if a package is not installed by the installer on the host.
func (h *Host) AssertPackageNotInstalledByInstaller(pkgs ...string) {
	for _, pkg := range pkgs {
		_, err := h.remote.ReadDir(fmt.Sprintf("/opt/datadog-packages/%s/stable/", pkg))
		if err == nil {
			installPath := strings.TrimSpace(h.remote.MustExecute(fmt.Sprintf("sudo readlink -f /opt/datadog-packages/%s/stable", pkg)))
			if strings.HasPrefix(installPath, "/opt/datadog-packages/") {
				h.t().Errorf("package %s installed by the installer", pkg)
			}
		}
	}
}

// AgentRuntimeConfig returns the runtime agent config on the host.
func (h *Host) AgentRuntimeConfig() (string, error) {
	return h.remote.Execute("sudo -u dd-agent datadog-agent config --all")
}

// AssertPackageVersion checks if a package is installed with the correct version
func (h *Host) AssertPackageVersion(pkg string, version string) {
	state := h.State()
	state.AssertDirExists(filepath.Join("/opt/datadog-packages/", pkg, version), 0755, "root", "root")
}

// AssertPackagePrefix checks if a package is installed with a version with the prefix
func (h *Host) AssertPackagePrefix(pkg string, semver string) {
	state := h.State()
	packageDir := filepath.Join("/opt/datadog-packages/", pkg, "")
	list := state.ListDirectory(packageDir)
	for _, entry := range list {
		version, _ := strings.CutPrefix(entry.Name, packageDir)
		if strings.HasPrefix(version, semver) {
			return
		}
	}
	h.t().Errorf("Semver compatible version %v not found among list of installed package %v", semver, list)
}

// AssertPackageInstalledByPackageManager checks if a package is installed by the package manager on the host.
func (h *Host) AssertPackageInstalledByPackageManager(pkgs ...string) {
	for _, pkg := range pkgs {
		switch h.pkgManager {
		case "apt":
			h.remote.MustExecute("dpkg-query -l " + pkg)
		case "yum", "zypper":
			h.remote.MustExecute("rpm -q " + pkg)
		default:
			h.t().Fatal("unsupported package manager")
		}
	}
}

// AssertPackageNotInstalledByPackageManager checks if a package is not installed by the package manager on the host.
func (h *Host) AssertPackageNotInstalledByPackageManager(pkgs ...string) {
	for _, pkg := range pkgs {
		switch h.pkgManager {
		case "apt":
			// If a package is removed but not purged, it will be in the "rc" state (opposed to "ii" for installed)
			// if it's been purged, the command will return an error
			h.remote.MustExecute(fmt.Sprintf("dpkg-query -l %[1]s | grep '^rc' || ! dpkg-query -l %[1]s", pkg))
		case "yum", "zypper":
			h.remote.MustExecute("! rpm -q " + pkg)
		default:
			h.t().Fatal("unsupported package manager")
		}
	}
}

// State returns the state of the host.
func (h *Host) State() State {
	return State{
		t:         h.t(),
		Users:     h.users(),
		Groups:    h.groups(),
		FS:        h.fs(),
		Units:     h.getSystemdUnitInfo(),
		Processes: h.getProcessesUnitInfo(),
	}
}

func (h *Host) users() []user.User {
	output := h.remote.MustExecute("sudo getent passwd")
	lines := strings.Split(output, "\n")
	var users []user.User
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, ":")
		assert.Len(h.t(), parts, 7)
		users = append(users, user.User{
			Username: parts[0],
			Uid:      parts[2],
			Gid:      parts[3],
			Name:     parts[4],
			HomeDir:  parts[5],
		})
	}
	sort.Slice(users, func(i, j int) bool {
		return users[i].Uid < users[j].Uid
	})
	return users
}

func (h *Host) groups() []user.Group {
	output := h.remote.MustExecute("sudo getent group")
	lines := strings.Split(output, "\n")
	var groups []user.Group
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, ":")
		assert.Len(h.t(), parts, 4)
		groups = append(groups, user.Group{
			Name: parts[0],
			Gid:  parts[2],
		})
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Gid < groups[j].Gid
	})
	return groups
}

func (h *Host) fs() map[string]FileInfo {
	ignoreDirs := []string{
		"/proc",
		"/sys",
		"/dev",
		"/run/utmp",
		"/tmp",
	}
	var cmdBuilder strings.Builder
	cmdBuilder.WriteString("sudo find / ")
	for _, dir := range ignoreDirs {
		fmt.Fprintf(&cmdBuilder, "-path '%s' -prune -o ", dir)
	}
	cmd := cmdBuilder.String()
	cmd += `-printf '%p\\|//%s\\|//%TY-%Tm-%Td %TH:%TM:%TS\\|//%f\\|//%m\\|//%u\\|//%g\\|//%y\\|//%l\n' 2>/dev/null`
	output := h.remote.MustExecute(cmd + " || true")
	lines := strings.Split(output, "\n")

	fileInfos := make(map[string]FileInfo)
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\\|//")
		assert.Len(h.t(), parts, 9)

		path := parts[0]
		size, _ := strconv.ParseInt(parts[1], 10, 64)
		modTime, _ := time.Parse("2006-01-02 15:04:05", parts[2])
		name := parts[3]
		mode, _ := strconv.ParseUint(parts[4], 8, 32)
		user := parts[5]
		group := parts[6]
		fileType := parts[7]
		isDir := fileType == "d"
		isSymlink := fileType == "l"
		link := parts[8]

		fileInfos[path] = FileInfo{
			Name:      name,
			Size:      size,
			Perms:     fs.FileMode(mode).Perm(),
			ModTime:   modTime,
			IsDir:     isDir,
			IsSymlink: isSymlink,
			Link:      link,
			User:      user,
			Group:     group,
		}
	}
	return fileInfos
}

func (h *Host) getSystemdUnitInfo() map[string]SystemdUnitInfo {
	// Retrieve the status of all units
	output := h.remote.MustExecute("sudo systemctl list-units --all --no-legend --no-pager")
	output = strings.ReplaceAll(output, "●", "") // Remove the bullet point
	unitsOutput := strings.Split(string(output), "\n")
	units := make(map[string]SystemdUnitInfo)

	// Retrieve the enabled state of unit files
	enabledOutput := h.remote.MustExecute("sudo systemctl list-unit-files --no-legend --no-pager")
	enabledOutput = strings.ReplaceAll(enabledOutput, "●", "") // Remove the bullet point
	enabledLines := strings.Split(string(enabledOutput), "\n")
	enabledMap := make(map[string]string)
	for _, line := range enabledLines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		enabledMap[fields[0]] = fields[1] // Map full unit name to enabled status
	}

	// Parse active state and match with enabled state
	for _, line := range unitsOutput {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		name := fields[0] // Full unit name with extension
		loadState := LoadState(fields[1])
		active := fields[2]
		subState := SubState(fields[3])

		enabled, exists := enabledMap[name]
		if !exists {
			enabled = "unknown" // Handle cases where the unit file is not listed
		}

		units[name] = SystemdUnitInfo{
			Name:      name,
			Active:    active,
			SubState:  subState,
			LoadState: loadState,
			Enabled:   enabled,
		}
	}

	return units
}

func (h *Host) getProcessesUnitInfo() map[string]ProcessesUnitInfo {
	processes := make(map[string]ProcessesUnitInfo)
	if !h.ProcmgrEnabled() {
		return processes
	}
	// Return early if procmgr is not running
	if _, err := h.remote.Execute("systemctl is-active --quiet datadog-agent-procmgr.service"); err != nil {
		return processes
	}

	const procmgrBin = "/opt/datadog-packages/datadog-agent/stable/embedded/bin/dd-procmgr"
	listOutput := h.remote.MustExecute("sudo -u dd-agent " + procmgrBin + " list --json")

	var entries []struct {
		Name         string   `json:"name"`
		UUID         string   `json:"uuid"`
		State        string   `json:"state"`
		PID          int      `json:"pid"`
		Command      string   `json:"command"`
		Args         []string `json:"args"`
		RestartCount int      `json:"restart_count"`
		LastExitCode *int     `json:"last_exit_code"`
		LastSignal   *int     `json:"last_signal"`
	}
	require.NoError(h.t(), json.Unmarshal([]byte(listOutput), &entries))

	for _, e := range entries {
		info := ProcessesUnitInfo{
			Name:         e.Name,
			UUID:         e.UUID,
			State:        e.State,
			PID:          e.PID,
			Command:      e.Command,
			Args:         e.Args,
			RestartCount: e.RestartCount,
			LastExitCode: e.LastExitCode,
			LastSignal:   e.LastSignal,
		}

		describeOutput := h.remote.MustExecute("sudo -u dd-agent " + procmgrBin + " describe --json " + e.Name)
		var detail struct {
			AutoStart bool `json:"auto_start"`
		}
		require.NoError(h.t(), json.Unmarshal([]byte(describeOutput), &detail))
		info.AutoStart = detail.AutoStart

		processes[e.Name] = info
	}

	return processes
}

// SetUmask set the default umask for commands
func (h *Host) SetUmask(mask string) (oldmask string) {
	oldmask = strings.TrimSpace(h.remote.MustExecute("umask"))
	if _, err := h.remote.Execute("cat ~/.bashrc | grep umask"); err != nil {
		// There are different default bashrc files for different distros. In some cases
		// the umask must be at the first instruction as other instructions are skipped for non-interactive sessions
		// and in others the umask must be at the bottom as it is overridden somewhere in the bashrc file.
		// Thus we set it in both places.
		h.remote.MustExecute(fmt.Sprintf("echo 'umask %s' | cat - ~/.bashrc > temp && mv temp ~/.bashrc", mask))
		h.remote.MustExecute(fmt.Sprintf("echo 'umask %s' | tee -a ~/.bashrc", mask))
	} else {
		h.remote.MustExecute(fmt.Sprintf("sed -i -E 's/umask %s/umask %s/g' ~/.bashrc", oldmask, mask))
	}
	h.remote.MustExecute("umask | grep -q " + mask) // Correctness check
	return oldmask
}

// SetupProxy sets up a Squid Proxy with Docker & adds iptables/nftables rules to redirect block all traffic
// except for the proxy
func (h *Host) SetupProxy() {
	// Install Docker & the Squid Proxy
	h.InstallDocker()
	h.remote.MustExecute("sudo docker run -d --name squid-proxy -v /opt/fixtures/squid.conf:/etc/squid/squid.conf -p 3128:3128 " +
		h.dockerImage("ecr-public/ubuntu/squid:4.10-20.04_beta", "public.ecr.aws/ubuntu/squid:4.10-20.04_beta"))

	squidIP := strings.TrimSpace(h.remote.MustExecute("sudo docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' squid-proxy"))

	// Block all traffic except for the proxy
	// Allow squid proxy
	h.remote.MustExecute(fmt.Sprintf("sudo iptables -A OUTPUT -d 0.0.0.0/0 -p tcp -s \"%s\" --dport 80 -j ACCEPT", squidIP))
	h.remote.MustExecute(fmt.Sprintf("sudo iptables -A OUTPUT -d 0.0.0.0/0 -p tcp -s \"%s\" --dport 443 -j ACCEPT", squidIP))
	// Block all traffic
	h.remote.MustExecute("sudo iptables -A OUTPUT -p tcp --dport 80 -j REJECT")
	h.remote.MustExecute("sudo iptables -A OUTPUT -p tcp --dport 443 -j REJECT")

	// Check proxy works
	_, err := h.remote.Execute("curl https://google.com")
	require.Error(h.t(), err)
}

// RemoveProxy removes the Squid Proxy & iptables/nftables rules
func (h *Host) RemoveProxy() {
	squidIP := strings.TrimSpace(h.remote.MustExecute("sudo docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' squid-proxy"))

	// Remove traffic block
	// Remove squid proxy rules
	h.remote.MustExecute(fmt.Sprintf("sudo iptables -D OUTPUT -p tcp -s \"%s\" --dport 80 -j ACCEPT", squidIP))
	h.remote.MustExecute(fmt.Sprintf("sudo iptables -D OUTPUT -p tcp -s \"%s\" --dport 443 -j ACCEPT", squidIP))
	// Remove block rules
	h.remote.MustExecute("sudo iptables -D OUTPUT -p tcp --dport 80 -j REJECT")
	h.remote.MustExecute("sudo iptables -D OUTPUT -p tcp --dport 443 -j REJECT")

	// Check proxy removed
	_, err := h.remote.Execute("curl https://google.com")
	require.NoError(h.t(), err)

	// Remove Docker container
	_, err = h.remote.Execute("sudo docker rm -f squid-proxy")
	if err != nil {
		h.t().Logf("warn: failed to remove Docker container: %v", err)
	}
}

// LoadState is the load state of a systemd unit.
type LoadState string

// SubState is the sub state of a systemd unit.
type SubState string

const (
	// Loaded is the load state of a systemd unit.
	Loaded LoadState = "loaded"
	// NotLoaded is the load state of a systemd unit.
	NotLoaded LoadState = "not-found"
	// Masked is the load state of a systemd unit.
	Masked LoadState = "masked"
	// Error is the load state of a systemd unit.
	Error LoadState = "error"

	// Running is the sub state of a systemd unit.
	Running SubState = "running"
	// Dead is the sub state of a systemd unit.
	Dead SubState = "dead"
)

// SystemdUnitInfo is the info of a systemd unit.
type SystemdUnitInfo struct {
	Name      string
	Active    string
	Enabled   string
	SubState  SubState
	LoadState LoadState
}

// ProcessesUnitInfo is the info of a process managed by procmgr.
type ProcessesUnitInfo struct {
	Name         string
	UUID         string
	State        string
	PID          int
	Command      string
	Args         []string
	RestartCount int
	LastExitCode *int
	LastSignal   *int
	AutoStart    bool
}

// FileInfo struct mimics os.FileInfo
type FileInfo struct {
	Name      string
	Size      int64
	Perms     fs.FileMode
	ModTime   time.Time
	IsDir     bool
	IsSymlink bool
	Link      string
	User      string
	Group     string
}

// State is the state of a remote host.
type State struct {
	t         *testing.T
	Users     []user.User
	Groups    []user.Group
	FS        map[string]FileInfo
	Units     map[string]SystemdUnitInfo
	Processes map[string]ProcessesUnitInfo
}

// Stat returns the FileInfo of a path on the host.
func (s *State) Stat(path string) (FileInfo, bool) {
	path = evalSymlinkPath(path, s.FS)
	fileInfo, ok := s.FS[path]
	return fileInfo, ok
}

// AssertUserExists asserts that a user exists on the host.
func (s *State) AssertUserExists(userName string) {
	for _, user := range s.Users {
		if user.Username == userName {
			return
		}
	}
	assert.Fail(s.t, "user does not exist", userName)
}

// AssertGroupExists asserts that a group exists on the host.
func (s *State) AssertGroupExists(groupName string) {
	for _, group := range s.Groups {
		if group.Name == groupName {
			return
		}
	}
	assert.Fail(s.t, "group does not exist", groupName)
}

// AssertUserHasGroup asserts that a user has a group on the host.
func (s *State) AssertUserHasGroup(userName, groupName string) {
	for _, user := range s.Users {
		if user.Username == userName {
			for _, group := range s.Groups {
				if group.Name == groupName {
					if user.Gid == group.Gid {
						return
					}
				}
			}
		}
	}
	assert.Fail(s.t, "user does not have group", userName, groupName)
}

// evalSymlinkPath resolves the absolute path, resolving symlinks
func evalSymlinkPath(path string, fs map[string]FileInfo) string {
	// Normalize the path to clean up any .. or .
	path = filepath.Clean(path)

	// Split the path into components
	parts := strings.Split(path, "/")
	resolvedPath := "/"

	for _, part := range parts {
		if part == "" || part == "." {
			// Ignore empty part or current directory marker
			continue
		}

		// Append the current part to the resolved path
		nextPath := filepath.Join(resolvedPath, part)
		nextPath = filepath.Clean(nextPath) // Clean to ensure no trailing slashes

		// Check if the current path component is a symlink
		if fileInfo, exists := fs[nextPath]; exists && fileInfo.IsSymlink {
			// Resolve the symlink
			symlinkTarget := fileInfo.Link
			if !filepath.IsAbs(symlinkTarget) {
				symlinkTarget = filepath.Join(filepath.Dir(nextPath), symlinkTarget)
			}
			// Handle recursive symlink resolution
			symlinkTarget = evalSymlinkPath(symlinkTarget, fs)
			// Update the resolvedPath to be the target of the symlink
			resolvedPath = symlinkTarget
		} else {
			// Not a symlink, or doesn't exist in fs; move to next component
			resolvedPath = nextPath
		}

		// Ensure the path ends correctly for the next iteration
		if !strings.HasSuffix(resolvedPath, "/") && len(resolvedPath) > 1 {
			resolvedPath += "/"
		}
	}
	return filepath.Clean(resolvedPath)
}

// ListDirectory returns a list of entries in the directory and fails the test
// if it doesn't exist
func (s *State) ListDirectory(path string) []FileInfo {
	path = evalSymlinkPath(path, s.FS)
	fileInfo, ok := s.FS[path]
	assert.True(s.t, ok, "dir %v does not exist", path)
	assert.True(s.t, fileInfo.IsDir, "%v is not a directory", path)

	directoryPrefix := path
	if directoryPrefix[len(directoryPrefix)-1] != '/' {
		directoryPrefix += "/"
	}
	entryList := []FileInfo{}
	for p, e := range s.FS {
		if strings.HasPrefix(p, directoryPrefix) {
			entryList = append(entryList, e)
		}
	}
	return entryList
}

// AssertDirExists asserts that a directory exists on the host with the given perms, user, and group.
func (s *State) AssertDirExists(path string, perms fs.FileMode, user string, group string) {
	path = evalSymlinkPath(path, s.FS)
	fileInfo, ok := s.FS[path]
	assert.True(s.t, ok, "dir %v does not exist", path)
	assert.True(s.t, fileInfo.IsDir, "%v is not a directory", path)
	assert.Equal(s.t, perms, fileInfo.Perms, "%v has unexpected perms", path)
	assert.Equal(s.t, user, fileInfo.User, "%v has unexpected user", path)
	assert.Equal(s.t, group, fileInfo.Group, "%v has unexpected group", path)
}

// AssertPathDoesNotExist asserts that a path does not exist on the host.
func (s *State) AssertPathDoesNotExist(path string) {
	path = evalSymlinkPath(path, s.FS)
	_, ok := s.FS[path]
	assert.False(s.t, ok, "something exists at path %s", path)
}

// AssertFileExistsAnyUser asserts that a file exists on the host with the given perms.
func (s *State) AssertFileExistsAnyUser(path string, perms fs.FileMode) {
	path = evalSymlinkPath(path, s.FS)
	fileInfo, ok := s.FS[path]
	assert.True(s.t, ok, "file %v does not exist", path)
	assert.False(s.t, fileInfo.IsDir, "%v is not a file", path)
	assert.Equal(s.t, perms, fileInfo.Perms, "%v has unexpected perms", path)
}

// AssertFileExists asserts that a file exists on the host with the given perms, user, and group.
func (s *State) AssertFileExists(path string, perms fs.FileMode, user string, group string) {
	path = evalSymlinkPath(path, s.FS)
	fileInfo, ok := s.FS[path]
	assert.True(s.t, ok, "file %v does not exist", path)
	assert.False(s.t, fileInfo.IsDir, "%v is not a file", path)
	assert.Equal(s.t, perms, fileInfo.Perms, "%v has unexpected perms", path)
	assert.Equal(s.t, user, fileInfo.User, "%v has unexpected user", path)
	assert.Equal(s.t, group, fileInfo.Group, "%v has unexpected group", path)
}

// AssertSymlinkExists asserts that a symlink exists on the host with the given target, user, and group.
func (s *State) AssertSymlinkExists(path string, target string, user string, group string) {
	fileInfo, ok := s.FS[path]
	assert.True(s.t, ok, "symlink %v does not exist", path)
	assert.True(s.t, fileInfo.IsSymlink, "%v is not a symlink", path)
	assert.Equal(s.t, target, fileInfo.Link, "%v has unexpected target", path)
	assert.Equal(s.t, user, fileInfo.User, "%v has unexpected user", path)
	assert.Equal(s.t, group, fileInfo.Group, "%v has unexpected group", path)
}

// AssertUnitsLoaded asserts that units are enabled on the host.
func (s *State) AssertUnitsLoaded(names ...string) {
	for _, name := range names {
		unit, ok := s.Units[name]
		assert.True(s.t, ok, "unit %v is not loaded", name)
		assert.Equal(s.t, Loaded, unit.LoadState, "unit %v is not loaded", name)
	}
}

// AssertUnitsEnabled asserts that a systemd unit is not loaded.
func (s *State) AssertUnitsEnabled(names ...string) {
	for _, name := range names {
		unit, ok := s.Units[name]
		assert.True(s.t, ok, "unit %v is not enabled", name)
		assert.Equal(s.t, "enabled", unit.Enabled, "unit %v is not enabled", name)
	}
}

// AssertUnitsRunning asserts that a systemd unit is running.
func (s *State) AssertUnitsRunning(names ...string) {
	for _, name := range names {
		unit, ok := s.Units[name]
		assert.True(s.t, ok, "unit %v is not running", name)
		assert.Equal(s.t, Running, unit.SubState, "unit %v is not running", name)
	}
}

// AssertUnitsActive asserts that a systemd unit is active (covers oneshot+RemainAfterExit services whose substate is "exited").
func (s *State) AssertUnitsActive(names ...string) {
	for _, name := range names {
		unit, ok := s.Units[name]
		assert.True(s.t, ok, "unit %v is not active", name)
		assert.Equal(s.t, "active", unit.Active, "unit %v is not active", name)
	}
}

// AssertUnitsNotLoaded asserts that a systemd unit is not loaded.
func (s *State) AssertUnitsNotLoaded(names ...string) {
	for _, name := range names {
		unit, ok := s.Units[name]
		assert.True(s.t, !ok || (ok && unit.LoadState != Loaded), "unit %v is loaded", name)
	}
}

// AssertUnitsNotEnabled asserts that a systemd unit is not enabled
func (s *State) AssertUnitsNotEnabled(names ...string) {
	for _, name := range names {
		unit, ok := s.Units[name]
		assert.True(s.t, ok, "unit %v is enabled", name)
		assert.Equal(s.t, "disabled", unit.Enabled, "unit %v is enabled", name)
	}
}

// AssertUnitsDead asserts that a systemd unit is not running.
func (s *State) AssertUnitsDead(names ...string) {
	for _, name := range names {
		unit, ok := s.Units[name]
		assert.True(s.t, ok, "unit %v is not running", name)
		assert.Equal(s.t, Dead, unit.SubState, "unit %v is not running", name)
	}
}

// processName strips a trailing ".yaml" suffix, allowing callers to pass either the process
// name (e.g. "datadog-agent-ddot") or its processes.d config filename (e.g. "datadog-agent-ddot.yaml").
func processName(name string) string {
	return strings.TrimSuffix(name, ".yaml")
}

// AssertProcessesLoaded asserts that processes are loaded (registered) by procmgr.
func (s *State) AssertProcessesLoaded(names ...string) {
	for _, name := range names {
		name := processName(name)
		_, ok := s.Processes[name]
		assert.True(s.t, ok, "process %v is not loaded", name)
	}
}

// AssertProcessesNotLoaded asserts that processes are not loaded (registered) by procmgr.
func (s *State) AssertProcessesNotLoaded(names ...string) {
	for _, name := range names {
		name := processName(name)
		_, ok := s.Processes[name]
		assert.False(s.t, ok, "process %v is loaded", name)
	}
}

// AssertProcessesEnabled asserts that processes are configured to auto-start under procmgr.
func (s *State) AssertProcessesEnabled(names ...string) {
	for _, name := range names {
		name := processName(name)
		process, ok := s.Processes[name]
		assert.True(s.t, ok, "process %v is not loaded", name)
		assert.True(s.t, process.AutoStart, "process %v is not enabled (auto_start)", name)
	}
}

// AssertProcessesNotEnabled asserts that processes are loaded but not configured to auto-start.
func (s *State) AssertProcessesNotEnabled(names ...string) {
	for _, name := range names {
		name := processName(name)
		process, ok := s.Processes[name]
		assert.True(s.t, ok, "process %v is not loaded", name)
		assert.False(s.t, process.AutoStart, "process %v is enabled (auto_start)", name)
	}
}

// AssertProcessesRunning asserts that processes are running under procmgr.
func (s *State) AssertProcessesRunning(names ...string) {
	for _, name := range names {
		name := processName(name)
		process, ok := s.Processes[name]
		assert.True(s.t, ok, "process %v is not loaded", name)
		assert.Equal(s.t, "Running", process.State, "process %v is not running", name)
	}
}

// AssertProcessesDead asserts that processes are not running under procmgr (stopped, exited, crashed or failed).
func (s *State) AssertProcessesDead(names ...string) {
	for _, name := range names {
		name := processName(name)
		process, ok := s.Processes[name]
		assert.True(s.t, ok, "process %v is not loaded", name)
		assert.NotEqual(s.t, "Running", process.State, "process %v is running", name)
	}
}
