// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package sbom

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

// hostInstallUsageAgentConfig turns on the host SBOM and its runtime usage
// enrichment, which system-probe runs with CWS off.
const hostInstallUsageAgentConfig = `sbom:
  enabled: true
  host:
    enabled: true
  enrichment:
    usage:
      enabled: true
`

// hostInstallUsageSystemProbeConfig shortens the enrichment interval, the time
// a package's usage must move by before system-probe reports it again, and the
// forward interval, so that usage reaches the core agent within the test.
const hostInstallUsageSystemProbeConfig = `runtime_security_config:
  sbom:
    enrichment_interval: 10s
    forward_interval: 20s
`

// hostInstallUsageSBOMCheck scans the host every minute, since a usage report
// of the host rides the next host scan, hourly by default.
const hostInstallUsageSBOMCheck = `ad_identifiers:
  - _sbom
init_config:
instances:
  - host_periodic_refresh_seconds: 60
`

type hostInstallUsageSuite struct {
	baseSuite[environments.Host]
}

// TestSBOMHostInstallUsageSuite installs the Agent package on an Ubuntu 22.04
// host with the host SBOM and its runtime usage enrichment on and CWS off. dpkg
// lists the files of gzip and libc6 under /bin and /lib, which the merged /usr
// of Ubuntu turns into aliases of the paths the kernel reports.
func TestSBOMHostInstallUsageSuite(t *testing.T) {
	e2e.Run(t, &hostInstallUsageSuite{}, e2e.WithProvisioner(awshost.Provisioner(
		awshost.WithRunOptions(
			scenec2.WithEC2InstanceOptions(scenec2.WithOS(e2eos.Ubuntu2204E2E)),
			scenec2.WithFakeIntakeOptions(fakeintake.WithRetentionPeriod(sbomHostRetentionPeriod)),
			scenec2.WithPreAgentInstallHook(startCron),
			scenec2.WithPreAgentInstallHook(startScriptDaemon),
			scenec2.WithPreAgentInstallHook(startExplicitDaemon),
			scenec2.WithPreAgentInstallHook(startReplacedDaemon),
			scenec2.WithAgentOptions(
				agentparams.WithAgentConfig(hostInstallUsageAgentConfig),
				agentparams.WithSystemProbeConfig(hostInstallUsageSystemProbeConfig),
				agentparams.WithIntegration("sbom.d", hostInstallUsageSBOMCheck),
			),
		),
	)))
}

// startCron starts cron before the Agent is installed, so the probe of
// system-probe finds it running when it starts.
func startCron(_ *aws.Environment, host *remote.Host) (pulumi.Resource, error) {
	return host.OS.Runner().Command("start-cron", &command.Args{
		Create: pulumi.String("sudo systemctl enable --now cron"),
	})
}

// scriptDaemonPackage is the dpkg package startScriptDaemon installs. Its
// /usr/bin/sbom-script-daemon is a shell script whose shebang argument puts
// the script third among the arguments of the shell, and whose name, longer
// than 15 bytes, is cut in the name of its process.
const scriptDaemonPackage = "sbom-script-daemon"

// startScriptDaemonScript builds scriptDaemonPackage with dpkg-deb, installs
// it, and runs its script as a transient service. The shell forks sleep and
// waits for it, so the shell running the script stays the main process.
const startScriptDaemonScript = `set -e
umask 022
cloud-init status --wait >/dev/null 2>&1 || true
d=$(mktemp -d)
mkdir -p "$d/pkg/DEBIAN" "$d/pkg/usr/bin"
printf 'Package: sbom-script-daemon\nVersion: 1.0\nArchitecture: all\nMaintainer: Datadog <package@datadoghq.com>\nDescription: package of the SBOM e2e tests\n' >"$d/pkg/DEBIAN/control"
printf '#!/bin/sh -e\nsleep infinity\n' >"$d/pkg/usr/bin/sbom-script-daemon"
chmod 755 "$d/pkg/usr/bin/sbom-script-daemon"
dpkg-deb --root-owner-group --build "$d/pkg" "$d/sbom-script-daemon.deb" >/dev/null
n=0
until sudo dpkg -i "$d/sbom-script-daemon.deb"; do
	n=$((n + 1))
	[ "$n" -lt 30 ]
	sleep 10
done
sudo systemd-run --unit=sbom-script-daemon /usr/bin/sbom-script-daemon`

// startScriptDaemon starts the script of scriptDaemonPackage before the Agent
// is installed, so the probe of system-probe finds it running when it starts.
// dpkg retries while the boot of the host holds its lock.
func startScriptDaemon(_ *aws.Environment, host *remote.Host) (pulumi.Resource, error) {
	return host.OS.Runner().Command("start-script-daemon", &command.Args{
		Create: pulumi.String(startScriptDaemonScript),
	})
}

// explicitDaemonPackage is the dpkg package startExplicitDaemon installs. Its
// /usr/bin/sbom-explicit-daemon is a Python script of mode 0644, which systemd
// runs as the operand of python3, as it runs waagent, so its process takes the
// name of python3.
const explicitDaemonPackage = "sbom-explicit-daemon"

// startExplicitDaemonScript builds explicitDaemonPackage with dpkg-deb,
// installs it, and runs its script as the operand of python3 in a transient
// service.
const startExplicitDaemonScript = `set -e
umask 022
cloud-init status --wait >/dev/null 2>&1 || true
d=$(mktemp -d)
mkdir -p "$d/pkg/DEBIAN" "$d/pkg/usr/bin"
printf 'Package: sbom-explicit-daemon\nVersion: 1.0\nArchitecture: all\nMaintainer: Datadog <package@datadoghq.com>\nDescription: package of the SBOM e2e tests\n' >"$d/pkg/DEBIAN/control"
printf 'import time\nwhile True:\n    time.sleep(3600)\n' >"$d/pkg/usr/bin/sbom-explicit-daemon"
chmod 644 "$d/pkg/usr/bin/sbom-explicit-daemon"
dpkg-deb --root-owner-group --build "$d/pkg" "$d/sbom-explicit-daemon.deb" >/dev/null
n=0
until sudo dpkg -i "$d/sbom-explicit-daemon.deb"; do
	n=$((n + 1))
	[ "$n" -lt 30 ]
	sleep 10
done
sudo systemd-run --unit=sbom-explicit-daemon /usr/bin/python3 -u /usr/bin/sbom-explicit-daemon`

// startExplicitDaemon starts the script of explicitDaemonPackage before the
// Agent is installed, so the probe of system-probe finds it running when it
// starts.
func startExplicitDaemon(_ *aws.Environment, host *remote.Host) (pulumi.Resource, error) {
	return host.OS.Runner().Command("start-explicit-daemon", &command.Args{
		Create: pulumi.String(startExplicitDaemonScript),
	})
}

// replacedDaemonPackage and replacedLibPackage are the dpkg packages
// startReplacedDaemon installs. The binary of the first, a copy of sleep, runs
// as a daemon that preloads replacedLib, the library of the second, a copy of
// libutil.so.1, a stub glibc keeps for compatibility. Copies of both replace
// them once the daemon runs, as an upgrade renames a new build over a file.
const (
	replacedDaemonPackage = "sbom-replaced-daemon"
	replacedLibPackage    = "sbom-replaced-lib"
	replacedLib           = "/usr/lib/sbom-replaced-lib/libsbomreplaced.so"
)

// startReplacedDaemonScript builds replacedDaemonPackage and
// replacedLibPackage with dpkg-deb, installs them, runs the daemon as a
// transient service that preloads the library, then renames a copy of the
// binary and of the library over them, so procfs names both with the suffix
// of a replaced file. The library comes from the directory of the libc of
// sleep, as the loader ignores a library of another ABI, such as the x32 one
// ldconfig lists first on a multilib host. Type=exec makes systemd-run return
// once the daemon runs its binary, and the script waits up to 5 s for the
// loader to map the library before the renames. The replaced subtest checks
// the mapping. A retry first stops the daemon of an earlier attempt.
// needrestart, which restarts the services running a replaced file once apt
// installs the Agent, is told to leave the service alone.
const startReplacedDaemonScript = `set -e
umask 022
cloud-init status --wait >/dev/null 2>&1 || true
sudo systemctl stop sbom-replaced-daemon 2>/dev/null || true
sudo systemctl reset-failed sbom-replaced-daemon 2>/dev/null || true
d=$(mktemp -d)
mkdir -p "$d/daemon/DEBIAN" "$d/daemon/usr/bin" "$d/lib/DEBIAN" "$d/lib/usr/lib/sbom-replaced-lib"
printf 'Package: sbom-replaced-daemon\nVersion: 1.0\nArchitecture: all\nMaintainer: Datadog <package@datadoghq.com>\nDescription: package of the SBOM e2e tests\n' >"$d/daemon/DEBIAN/control"
printf 'Package: sbom-replaced-lib\nVersion: 1.0\nArchitecture: all\nMaintainer: Datadog <package@datadoghq.com>\nDescription: package of the SBOM e2e tests\n' >"$d/lib/DEBIAN/control"
cp /usr/bin/sleep "$d/daemon/usr/bin/sbom-replaced-daemon"
libc=$(ldd /usr/bin/sleep | awk '$1 == "libc.so.6" {print $3}')
cp "${libc%/*}/libutil.so.1" "$d/lib/usr/lib/sbom-replaced-lib/libsbomreplaced.so"
dpkg-deb --root-owner-group --build "$d/daemon" "$d/sbom-replaced-daemon.deb" >/dev/null
dpkg-deb --root-owner-group --build "$d/lib" "$d/sbom-replaced-lib.deb" >/dev/null
n=0
until sudo dpkg -i "$d/sbom-replaced-daemon.deb" "$d/sbom-replaced-lib.deb"; do
	n=$((n + 1))
	[ "$n" -lt 30 ]
	sleep 10
done
if [ -d /etc/needrestart/conf.d ]; then
	echo '$nrconf{override_rc}{qr(^sbom-replaced-daemon)} = 0;' | sudo tee /etc/needrestart/conf.d/sbom-replaced-daemon.conf >/dev/null
fi
sudo systemd-run --unit=sbom-replaced-daemon --property=Type=exec --setenv=LD_PRELOAD=/usr/lib/sbom-replaced-lib/libsbomreplaced.so /usr/bin/sbom-replaced-daemon infinity
pid=$(systemctl show -p MainPID --value sbom-replaced-daemon)
n=0
until sudo grep -qF /usr/lib/sbom-replaced-lib/libsbomreplaced.so "/proc/$pid/maps" || [ "$n" -ge 50 ]; do
	n=$((n + 1))
	sleep 0.1
done
for f in /usr/bin/sbom-replaced-daemon /usr/lib/sbom-replaced-lib/libsbomreplaced.so; do
	sudo cp -p "$f" "$f.new"
	sudo mv -f "$f.new" "$f"
done`

// startReplacedDaemon starts the daemon of replacedDaemonPackage and replaces
// its binary and its library before the Agent is installed, so the probe of
// system-probe finds it running replaced files when it starts.
func startReplacedDaemon(_ *aws.Environment, host *remote.Host) (pulumi.Resource, error) {
	return host.OS.Runner().Command("start-replaced-daemon", &command.Args{
		Create: pulumi.String(startReplacedDaemonScript),
	})
}

// mapsExecutable reports whether maps, the content of /proc/<pid>/maps, maps
// the file at path with execute permission.
func mapsExecutable(maps, path string) bool {
	for _, line := range strings.Split(maps, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 6 && strings.Contains(fields[1], "x") && strings.HasSuffix(line, " "+path) {
			return true
		}
	}
	return false
}

// hostInt runs cmd on the host and returns the integer it prints.
func (s *hostInstallUsageSuite) hostInt(cmd string) int64 {
	out := strings.TrimSpace(s.Env().RemoteHost.MustExecute(cmd))
	n, err := strconv.ParseInt(out, 10, 64)
	s.Require().NoErrorf(err, "%s printed %q", cmd, out)
	return n
}

// unitStart returns the monotonic time, in microseconds, at which the main
// process of the systemd unit started.
func (s *hostInstallUsageSuite) unitStart(unit string) int64 {
	return s.hostInt("systemctl show -p ExecMainStartTimestampMonotonic --value " + unit)
}

// mainPID returns the main process of the systemd unit, which must run.
func (s *hostInstallUsageSuite) mainPID(unit string) string {
	pid := s.hostInt("systemctl show -p MainPID --value " + unit)
	s.Require().NotZerof(pid, "the service %s has no main process", unit)
	return strconv.FormatInt(pid, 10)
}

func (s *hostInstallUsageSuite) SetupSuite() {
	s.baseSuite.SetupSuite()
	s.Fakeintake = s.Env().FakeIntake.Client()
}

// Test00UpAndRunning checks the premise of the suite: system-probe brings up
// the usage consumer alone.
func (s *hostInstallUsageSuite) Test00UpAndRunning() {
	host := s.Env().RemoteHost
	if !s.EventuallyWithTf(func(c *assert.CollectT) {
		out, err := host.Execute("sudo grep -h 'event monitoring' /var/log/datadog/system-probe.log")
		require.NoError(c, err, "system-probe logged no event monitoring consumer yet")
		assert.Contains(c, out, "event monitoring usage consumer initialized")
		assert.NotContains(c, out, "event monitoring cws consumer initialized")
	}, 5*time.Minute, 10*time.Second, "system-probe never ran the usage enrichment alone") {
		out, _ := host.Execute("sudo systemctl status datadog-agent-sysprobe --no-pager; sudo journalctl -u datadog-agent-sysprobe -n 50 --no-pager")
		s.T().Logf("system-probe service:\n%s", out)
	}
}

// TestHostUsage checks the usage of the host packages: a daemon and two scripts
// started before the Agent, one through its shebang and one as the operand of
// python3, a daemon and its library replaced on disk before the Agent, a binary
// and its library run by root, and a setuid binary.
func (s *hostInstallUsageSuite) TestHostUsage() {
	host := s.Env().RemoteHost
	defer func() {
		if s.T().Failed() {
			for _, log := range []string{"system-probe", "agent"} {
				out, _ := host.Execute("sudo grep -iE 'sbom|usage consumer' /var/log/datadog/" + log + ".log | tail -n 100")
				s.T().Logf("PKG-IN-USE[host] SBOM lines of the %s log:\n%s", log, out)
			}
		}
	}()

	// cron starts before the Agent, so its use comes from the processes the
	// probe finds running when it starts.
	s.Run("daemon", func() {
		s.EventuallyWithTf(func(c *assert.CollectT) {
			cron := findComponent(newestHostSBOM(c, s.Fakeintake, time.Time{}), "cron")
			require.NotNilf(c, cron, "no cron in the host SBOM")
			ts, _ := lastSeenRunning(cron)
			s.T().Logf("PKG-IN-USE[host] daemon: cron LastSeenRunning=%d", ts)
			assert.Positivef(c, ts, "cron is unused, with its daemon started before the Agent")
			assert.Equalf(c, []string{"true"}, propertyValues(cron.GetProperties(), propRunningAsRoot), "cron %s, cron runs as root", propRunningAsRoot)
			// 10m: run on its own, the subtest also waits out the Agent start and
			// its first host scans.
		}, 10*time.Minute, 15*time.Second, "the host SBOM never reported cron in use")
	})

	// The script of scriptDaemonPackage starts before the Agent too, and procfs
	// gives the shell running it as the executable of its process, so its use
	// comes from the script the probe finds in the arguments of the process.
	s.Run("script", func() {
		pid := s.mainPID(scriptDaemonPackage)
		exe := strings.TrimSpace(host.MustExecute("sudo readlink /proc/" + pid + "/exe"))
		s.Require().Equalf("/usr/bin/dash", exe, "the main process of %s runs %s", scriptDaemonPackage, exe)
		s.Require().Lessf(s.unitStart(scriptDaemonPackage), s.unitStart("datadog-agent-sysprobe"), "%s started after system-probe", scriptDaemonPackage)

		s.EventuallyWithTf(func(c *assert.CollectT) {
			script := findComponent(newestHostSBOM(c, s.Fakeintake, time.Time{}), scriptDaemonPackage)
			require.NotNilf(c, script, "no %s in the host SBOM", scriptDaemonPackage)
			ts, _ := lastSeenRunning(script)
			s.T().Logf("PKG-IN-USE[host] script: %s LastSeenRunning=%d", scriptDaemonPackage, ts)
			assert.Positivef(c, ts, "%s is unused, with its script running since before the Agent", scriptDaemonPackage)
			assert.Equalf(c, []string{"true"}, propertyValues(script.GetProperties(), propRunningAsRoot), "%s %s, systemd runs it as root", scriptDaemonPackage, propRunningAsRoot)
		}, 10*time.Minute, 15*time.Second, "the host SBOM never reported %s in use", scriptDaemonPackage)
	})

	// The script of explicitDaemonPackage runs as the operand of python3 from
	// before the Agent, so its process takes the name of python3, and its use
	// comes from the script the probe finds past the options of python3.
	s.Run("explicit", func() {
		pid := s.mainPID(explicitDaemonPackage)
		exe := strings.TrimSpace(host.MustExecute("sudo readlink /proc/" + pid + "/exe"))
		s.Require().Regexpf(`^/usr/bin/python3\.[0-9]+$`, exe, "the main process of %s runs %s", explicitDaemonPackage, exe)
		comm := strings.TrimSpace(host.MustExecute("cat /proc/" + pid + "/comm"))
		s.Require().Equalf("python3", comm, "the main process of %s is named %s", explicitDaemonPackage, comm)
		s.Require().Lessf(s.unitStart(explicitDaemonPackage), s.unitStart("datadog-agent-sysprobe"), "%s started after system-probe", explicitDaemonPackage)

		s.EventuallyWithTf(func(c *assert.CollectT) {
			explicit := findComponent(newestHostSBOM(c, s.Fakeintake, time.Time{}), explicitDaemonPackage)
			require.NotNilf(c, explicit, "no %s in the host SBOM", explicitDaemonPackage)
			ts, _ := lastSeenRunning(explicit)
			s.T().Logf("PKG-IN-USE[host] explicit: %s LastSeenRunning=%d", explicitDaemonPackage, ts)
			assert.Positivef(c, ts, "%s is unused, with its script running as the operand of python3 since before the Agent", explicitDaemonPackage)
			assert.Equalf(c, []string{"true"}, propertyValues(explicit.GetProperties(), propRunningAsRoot), "%s %s, systemd runs it as root", explicitDaemonPackage, propRunningAsRoot)
		}, 10*time.Minute, 15*time.Second, "the host SBOM never reported %s in use", explicitDaemonPackage)
	})

	// The binary of replacedDaemonPackage and replacedLib, which the daemon
	// preloads, are replaced on disk once the daemon runs, before the Agent, so
	// procfs names both with the suffix of a replaced file, and their use comes
	// from the replay of those paths: the executable of the daemon and its
	// mappings.
	s.Run("replaced", func() {
		pid := s.mainPID(replacedDaemonPackage)
		exe := strings.TrimSpace(host.MustExecute("sudo readlink /proc/" + pid + "/exe"))
		s.Require().Truef(strings.HasSuffix(exe, " (deleted)"), "the daemon of %s runs %s, a binary in place", replacedDaemonPackage, exe)
		maps := host.MustExecute("sudo cat /proc/" + pid + "/maps")
		if !mapsExecutable(maps, replacedLib+" (deleted)") {
			journal := host.MustExecute("sudo journalctl --no-pager -u " + replacedDaemonPackage)
			s.Require().Failf("premise", "the daemon of %s maps %s in place, or not at all:\n%s\n%s", replacedDaemonPackage, replacedLib, maps, journal)
		}
		sysprobeStart := s.hostInt(`date -d "$(systemctl show -p ExecMainStartTimestamp --value datadog-agent-sysprobe)" +%s`)
		for _, file := range []string{"/usr/bin/" + replacedDaemonPackage, replacedLib} {
			s.Require().Lessf(s.hostInt("stat -c %Z "+file), sysprobeStart, "%s was replaced after system-probe started", file)
		}
		s.Require().Lessf(s.unitStart(replacedDaemonPackage), s.unitStart("datadog-agent-sysprobe"), "%s started after system-probe", replacedDaemonPackage)

		s.EventuallyWithTf(func(c *assert.CollectT) {
			comps := newestHostSBOM(c, s.Fakeintake, time.Time{})
			for _, name := range []string{replacedDaemonPackage, replacedLibPackage} {
				comp := findComponent(comps, name)
				require.NotNilf(c, comp, "no %s in the host SBOM", name)
				ts, _ := lastSeenRunning(comp)
				s.T().Logf("PKG-IN-USE[host] replaced: %s LastSeenRunning=%d", name, ts)
				assert.Positivef(c, ts, "%s is unused, with its replaced file running since before the Agent", name)
				assert.Equalf(c, []string{"true"}, propertyValues(comp.GetProperties(), propRunningAsRoot), "%s %s, systemd runs it as root", name, propRunningAsRoot)
			}
		}, 10*time.Minute, 15*time.Second, "the host SBOM never reported %s and %s in use", replacedDaemonPackage, replacedLibPackage)
	})

	// sudo runs gzip as root from its setuid binary, and the loader opens
	// libc.so.6 for every process of the run. The probe samples those opens,
	// and dpkg lists libc.so.6 under /lib, the alias of the path they open.
	s.Run("in-use", func() {
		startedAt, err := strconv.ParseInt(strings.TrimSpace(host.MustExecute("date +%s")), 10, 64)
		s.Require().NoError(err, "date")

		s.EventuallyWithTf(func(c *assert.CollectT) {
			_, err := host.Execute("sudo gzip --version")
			require.NoError(c, err, "sudo gzip --version")

			comps := newestHostSBOM(c, s.Fakeintake, time.Time{})
			gzip := findComponent(comps, "gzip")
			require.NotNilf(c, gzip, "no gzip in the host SBOM")
			libc := findComponent(comps, "libc6")
			require.NotNilf(c, libc, "no libc6 in the host SBOM")
			sudo := findComponent(comps, "sudo")
			require.NotNilf(c, sudo, "no sudo in the host SBOM")

			gzipTS, _ := lastSeenRunning(gzip)
			libcTS, _ := lastSeenRunning(libc)
			suid := propertyValues(sudo.GetProperties(), propHasSetSuidBit)
			s.T().Logf("PKG-IN-USE[host] in-use: gzip LastSeenRunning=%d, libc6 LastSeenRunning=%d, sudo HasSetSuidBit=%v, startedAt=%d", gzipTS, libcTS, suid, startedAt)

			assert.GreaterOrEqualf(c, gzipTS, startedAt, "gzip LastSeenRunning %d predates the run at %d", gzipTS, startedAt)
			assert.Equalf(c, []string{"true"}, propertyValues(gzip.GetProperties(), propRunningAsRoot), "gzip %s, sudo runs it as root", propRunningAsRoot)
			assert.GreaterOrEqualf(c, libcTS, startedAt, "libc6 LastSeenRunning %d predates the run at %d", libcTS, startedAt)
			assert.Equalf(c, []string{"true"}, suid, "sudo %s, its setuid binary ran", propHasSetSuidBit)
			osComp := findOSComponent(comps)
			if osComp == nil {
				osComp = findComponent(comps, "ubuntu")
			}
			require.NotNilf(c, osComp, "no OS component in the host SBOM")
			assertNoRuntimeProperties(c, "host", osComp)
		}, 10*time.Minute, 15*time.Second, "the host SBOM never reported gzip, libc6 and sudo in use")
	})
}
