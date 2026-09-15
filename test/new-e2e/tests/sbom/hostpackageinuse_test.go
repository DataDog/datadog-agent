// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package sbom

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/sbom"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hostAgentConfig collects the host SBOM and enriches it with runtime usage.
// sbom.enrichment.usage.enabled is the one enrichment switch: it covers whichever
// dimensions the Agent collects, so pairing it with sbom.host.enabled is what asks
// for the host. It also brings up the system-probe event monitor and derives the
// CWS SBOM resolver's host index, which needs no system-probe.yaml of its own.
//
// The os analyzer alone keeps the host scan inside the test window. The runtime
// enrichment covers the OS packages of the package database, which is what the
// resolver indexes.
const hostAgentConfig = `
sbom:
  enabled: true
  host:
    enabled: true
    analyzers: ["os"]
  enrichment:
    usage:
      enabled: true
`

// hostSystemProbeConfig shortens the resolver's enrichment window so a package's
// in-use timestamp surfaces within the test window instead of the 1m default, and
// its host refresh period so the reset phase observes a recomputed index.
const hostSystemProbeConfig = `
runtime_security_config:
  sbom:
    enrichment_interval: 10s
    forward_interval: 20s
    host:
      refresh_interval: 60s
`

// hostSBOMRetentionPeriod overrides fakeintake's 15m default: the phases below
// span more than that, and the earlier payloads are what the staleness assertion
// compares against.
const hostSBOMRetentionPeriod = "1h"

// pkgInUseHost parameterizes the host package-in-use phases for one distribution.
// The package manager and the path of the package database differ; the enrichment
// behaviour under test does not. Package names are resolved from the host at
// runtime (see pkgOwning) rather than hard-coded, because which package owns a
// given binary drifts between releases: RHEL splits su and lsblk out of
// util-linux into util-linux-core, Debian keeps them in util-linux.
type pkgInUseHost struct {
	name string // subtest name + log tag
	os   e2eos.Descriptor
	// pkgQuery is a shell command printing the name of the package owning the
	// path in $1. dpkg prints "<pkg>: <path>", rpm prints the name alone.
	pkgQuery string
}

var pkgInUseHosts = []pkgInUseHost{
	{name: "rhel10", os: e2eos.RedHat10,
		pkgQuery: `rpm -qf --queryformat '%{NAME}' "$1"`},
	{name: "ubuntu2404", os: e2eos.Ubuntu2404,
		pkgQuery: `dpkg -S "$1" | head -1 | cut -d: -f1`},
}

// The binaries the phases drive. inUseBin is a packaged binary an idle host never
// runs, so its package starts out reported as not in use. controlBin is run by the
// keep-alive loop, so its package is the positive control proving the enrichment
// flows at all. suidBin is setuid-root, and stickyBin is a plain binary of the
// same package, which is what shows the setuid observation is sticky.
var (
	// inUseBins are tried in order until one is installed. Both are diffutils
	// binaries on a stock RHEL or Debian host, and neither is on the path of
	// anything a booted, idle VM runs. gzip closes the list because logrotate and
	// man-db do run it, which would show up as the package already being in use.
	inUseBins = []string{"/usr/bin/diff", "/usr/bin/cmp", "/usr/bin/gzip"}
)

const (
	controlBin = "/usr/bin/cat"
	suidBin    = "/usr/bin/su"
	stickyBin  = "/usr/bin/lsblk"
)

type hostPackageInUseSuite struct {
	baseSuite[environments.Host]

	distro pkgInUseHost

	// Resolved from the host in SetupSuite.
	inUseBin   string
	inUsePkg   string
	controlPkg string
	suidPkg    string
	stickyPkg  string
}

// TestSBOMHostPackageInUseRHEL10 and its Ubuntu counterpart provision a VM with
// the Agent installed as a package, then verify the host "package in use"
// enrichment end to end: a host package goes from not in use, to in use once a
// process runs its binary, to stale once that stops, and back to zero after a
// package-database write refreshes the index.
func TestSBOMHostPackageInUseRHEL10(t *testing.T) {
	testHostPackageInUse(t, pkgInUseHosts[0])
}

func TestSBOMHostPackageInUseUbuntu2404(t *testing.T) {
	testHostPackageInUse(t, pkgInUseHosts[1])
}

func testHostPackageInUse(t *testing.T, d pkgInUseHost) {
	e2e.Run(t, &hostPackageInUseSuite{distro: d},
		e2e.WithStackName("sbom-host-pkg-in-use-"+d.name),
		e2e.WithProvisioner(awshost.Provisioner(
			awshost.WithRunOptions(
				scenec2.WithEC2InstanceOptions(
					scenec2.WithOS(d.os),
					scenec2.WithInstanceType("t3.large"),
					scenec2.WithInternetAccess(),
				),
				scenec2.WithFakeIntakeOptions(fakeintake.WithRetentionPeriod(hostSBOMRetentionPeriod)),
				scenec2.WithAgentOptions(
					agentparams.WithAgentConfig(hostAgentConfig),
					agentparams.WithSystemProbeConfig(hostSystemProbeConfig),
				),
			),
		)),
	)
}

func (s *hostPackageInUseSuite) SetupSuite() {
	s.baseSuite.SetupSuite()
	s.Fakeintake = s.Env().FakeIntake.Client()

	// An e2e retry reuses the host, where the detached loops of the earlier
	// attempt would still be running and would spoil the not-in-use baseline.
	for _, loop := range []string{"keepalive", "inuse", "suid", "sticky", "nonroot"} {
		s.stopLoop(loop)
	}

	s.inUseBin = s.firstPresent(inUseBins...)
	s.inUsePkg = s.pkgOwning(s.inUseBin)
	s.controlPkg = s.pkgOwning(controlBin)
	s.suidPkg = s.pkgOwning(suidBin)
	s.stickyPkg = s.pkgOwning(stickyBin)
	s.T().Logf("HOST-PKG-IN-USE[%s] in-use=%s(%s) control=%s suid=%s sticky=%s",
		s.distro.name, s.inUsePkg, s.inUseBin, s.controlPkg, s.suidPkg, s.stickyPkg)
}

// Test00UpAndRunning waits (the 00 prefix runs it first) for the Agent and
// system-probe services to be up, so the phases do not start against an Agent
// that has yet to open its SBOM stream.
func (s *hostPackageInUseSuite) Test00UpAndRunning() {
	s.EventuallyWithTf(func(c *assert.CollectT) {
		for _, service := range []string{"datadog-agent", "datadog-agent-sysprobe"} {
			out, err := s.Env().RemoteHost.Execute("systemctl is-active " + service)
			assert.NoErrorf(c, err, "%s is not active: %s", service, out)
			assert.Equalf(c, "active", strings.TrimSpace(out), "%s is %q", service, strings.TrimSpace(out))
		}
	}, 5*time.Minute, 10*time.Second, "The Agent services never became active")

	// The enrichment reaches the core agent over the runtime-security command
	// socket. Log what the Agent resolved, which is the first thing to look at
	// when no host payload carries the runtime properties.
	out, _ := s.Env().RemoteHost.Execute("sudo datadog-agent config 2>/dev/null | grep -A6 -iE '^sbom:' | head -40")
	s.T().Logf("HOST-PKG-IN-USE[%s] agent sbom config:\n%s", s.distro.name, out)
}

// TestHostPackageInUse drives the whole not-in-use -> in-use -> stale -> setuid ->
// sticky -> refresh -> non-root cycle.
//
// The phases are a timeline rather than independent cases: staleness only means
// something after a package has been in use, and stickiness only after a setuid
// binary has run, so each one builds on the state the previous left behind. They
// stay separate subtests to name which transition broke, and each is gated on
// the one before it so the first failure is the only one reported rather than
// cascading through the rest.
func (s *hostPackageInUseSuite) TestHostPackageInUse() {
	// Keep a control package continuously in use without ever touching the in-use
	// binary, so a dead pipeline is told apart from a package that is genuinely
	// not running.
	s.startLoop("keepalive", controlBin+" /etc/os-release >/dev/null 2>&1")
	defer s.stopLoop("keepalive")

	// Phase 1: baseline. The merge must have run (the in-use package carries a
	// LastSeenRunning property at all) and must report the package as not in use.
	if !s.Run("not-in-use", func() {
		s.EventuallyWithTf(func(c *assert.CollectT) {
			ts, present, inUse := s.packageUsage(c, s.inUsePkg)
			require.Truef(c, present, "no enriched host SBOM yet (%s carries no %s property)", s.inUsePkg, propLastSeenRunning)
			ctrlTS, _, _ := s.packageUsage(c, s.controlPkg)
			require.Positivef(c, ctrlTS, "positive control %s not in use, enrichment is not flowing and %s=0 cannot be trusted", s.controlPkg, s.inUsePkg)
			s.T().Logf("HOST-PKG-IN-USE[%s] baseline: %s LastSeenRunning=%d; %s(control)=%d; in-use packages=%v",
				s.distro.name, s.inUsePkg, ts, s.controlPkg, ctrlTS, inUse)
			assert.Zerof(c, ts, "%s should be not-in-use at baseline, got LastSeenRunning=%d", s.inUsePkg, ts)
		}, 15*time.Minute, 15*time.Second, "the host SBOM never reported %s as not-in-use", s.inUsePkg)
	}) {
		return
	}

	// Phase 2: run the in-use binary repeatedly and watch the package flip to a
	// recent timestamp.
	if !s.Run("in-use", func() {
		startedAt := s.hostEpoch()
		s.startLoop("inuse", s.inUseBin+" --version >/dev/null 2>&1")

		s.EventuallyWithTf(func(c *assert.CollectT) {
			ts, present, inUse := s.packageUsage(c, s.inUsePkg)
			require.Truef(c, present, "%s carries no %s property", s.inUsePkg, propLastSeenRunning)
			require.Positivef(c, ts, "%s still reported not-in-use; in-use packages=%v", s.inUsePkg, inUse)
			age := time.Now().Unix() - ts
			s.T().Logf("HOST-PKG-IN-USE[%s] running: %s LastSeenRunning=%d age=%ds startedAt=%d",
				s.distro.name, s.inUsePkg, ts, age, startedAt)
			assert.GreaterOrEqualf(c, ts, startedAt, "%s LastSeenRunning %d predates the loop start %d, so it is a stale value", s.inUsePkg, ts, startedAt)
			assert.LessOrEqualf(c, age, inUseWindowSec, "%s LastSeenRunning is %ds old, expected at most %ds while in use", s.inUsePkg, age, inUseWindowSec)
			// The loop runs as root and the in-use binary is not setuid.
			assert.Equalf(c, "true", s.packageProperty(s.inUsePkg, propRunningAsRoot), "%s RunningAsRoot should be true, the loop runs as root", s.inUsePkg)
			assert.Equalf(c, "false", s.packageProperty(s.inUsePkg, propHasSetSuidBit), "%s HasSetSuidBit should be false, %s is not setuid", s.inUsePkg, s.inUseBin)
		}, 6*time.Minute, 15*time.Second, "the host SBOM never reported %s as in-use", s.inUsePkg)
	}) {
		return
	}

	// Phase 3: stop the loop and watch the timestamp age out. LastSeenRunning
	// records the last access and is not reset when a process exits, so "no longer
	// running" reads as the timestamp freezing rather than returning to zero.
	if !s.Run("stale-after-stop", func() {
		s.stopLoop("inuse")

		s.EventuallyWithTf(func(c *assert.CollectT) {
			ts, _, _ := s.packageUsage(c, s.inUsePkg)
			require.Positivef(c, ts, "%s was never reported in-use, so staleness cannot be asserted", s.inUsePkg)
			age := time.Now().Unix() - ts
			s.T().Logf("HOST-PKG-IN-USE[%s] stopped: %s LastSeenRunning=%d age=%ds", s.distro.name, s.inUsePkg, ts, age)
			assert.Greaterf(c, age, staleWindowSec, "%s LastSeenRunning is only %ds old, expected more than %ds once stopped", s.inUsePkg, age, staleWindowSec)
		}, 6*time.Minute, 20*time.Second, "the host SBOM never reported %s as stale", s.inUsePkg)
	}) {
		return
	}

	// Phase 4: setuid. Running a setuid-root binary sets HasSetSuidBit on its
	// package.
	if !s.Run("setuid", func() {
		s.startLoop("suid", suidBin+" --version >/dev/null 2>&1")
		defer s.stopLoop("suid")

		s.EventuallyWithTf(func(c *assert.CollectT) {
			ts, present, _ := s.packageUsage(c, s.suidPkg)
			require.Truef(c, present, "%s carries no %s property yet", s.suidPkg, propLastSeenRunning)
			require.Positivef(c, ts, "%s was never reported in use", s.suidPkg)
			suidVal := s.packageProperty(s.suidPkg, propHasSetSuidBit)
			s.T().Logf("HOST-PKG-IN-USE[%s] setuid: %s HasSetSuidBit=%q (ts=%d)", s.distro.name, s.suidPkg, suidVal, ts)
			assert.Equalf(c, "true", suidVal, "%s HasSetSuidBit should be true, %s ran and is setuid-root", s.suidPkg, suidBin)
		}, 6*time.Minute, 15*time.Second, "the host SBOM never reported %s as setuid", s.suidPkg)
	}) {
		return
	}

	// Phase 5: stickiness. With the setuid binary no longer running, a plain
	// binary of the same package keeps that package in use, and its
	// HasSetSuidBit must hold: the observation belongs to the package for the
	// life of the scan, not to the last file of it that was accessed. Reading a
	// timestamp at or after the moment the setuid loop stopped is what shows the
	// payload under test was produced by the plain binary alone.
	if !s.Run("setuid-is-sticky", func() {
		if s.stickyPkg != s.suidPkg {
			s.T().Skipf("%s owns %s while %s owns %s, so no plain binary of the setuid package is at hand",
				s.suidPkg, suidBin, s.stickyPkg, stickyBin)
		}

		stickyFrom := s.hostEpoch()
		s.startLoop("sticky", stickyBin+" >/dev/null 2>&1")
		defer s.stopLoop("sticky")

		s.EventuallyWithTf(func(c *assert.CollectT) {
			ts, _, _ := s.packageUsage(c, s.suidPkg)
			require.GreaterOrEqualf(c, ts, stickyFrom, "%s was not accessed again after the setuid loop stopped at %d, so nothing shows what a plain access does", s.suidPkg, stickyFrom)
			suidVal := s.packageProperty(s.suidPkg, propHasSetSuidBit)
			s.T().Logf("HOST-PKG-IN-USE[%s] sticky: %s HasSetSuidBit=%q (ts=%d, stickyFrom=%d)",
				s.distro.name, s.suidPkg, suidVal, ts, stickyFrom)
			assert.Equalf(c, "true", suidVal, "%s HasSetSuidBit was cleared by an access to %s, which is not setuid", s.suidPkg, stickyBin)
		}, 6*time.Minute, 15*time.Second, "the host SBOM never reported a plain access to %s", s.suidPkg)
	}) {
		return
	}

	// Phase 6: refresh. The resolver recomputes the host index on
	// runtime_security_config.sbom.host.refresh_interval, which the suite shortens
	// to a minute, and a rescan starts every package from never seen running. This
	// is the only path back to zero, and it is what keeps a package upgraded after
	// startup from being reported as never running. It runs without the CWS rule
	// engine, which the bundled refresh_sbom rule would need.
	if !s.Run("refresh-reset", func() {
		s.EventuallyWithTf(func(c *assert.CollectT) {
			v := s.packageProperty(s.inUsePkg, propLastSeenRunning)
			require.NotEmptyf(c, v, "%s carries no %s property", s.inUsePkg, propLastSeenRunning)
			s.T().Logf("HOST-PKG-IN-USE[%s] refresh: %s LastSeenRunning=%q", s.distro.name, s.inUsePkg, v)
			assert.Equalf(c, "0", v, "%s LastSeenRunning should be back to 0 once the host index is recomputed, got %q", s.inUsePkg, v)
		}, 8*time.Minute, 20*time.Second, "the host index never recomputed, leaving %s at its old runtime properties", s.inUsePkg)
	}) {
		return
	}

	// Phase 7: non-root. The refresh cleared the sticky properties, and nothing
	// else on the host runs the in-use binary, so running it only as an
	// unprivileged user is what shows RunningAsRoot reports who ran it.
	//
	// Both properties are read from the newest payload and the timestamp is
	// compared against the moment the loop started. Reading the timestamp across
	// every retained payload would take the one the root loop left behind in the
	// in-use phase, and a refresh sets RunningAsRoot back to false on its own, so
	// the pair would hold without the unprivileged run ever being seen.
	if !s.Run("non-root", func() {
		startedAt := s.hostEpoch()
		s.startLoop("nonroot", "sudo -u nobody "+s.inUseBin+" --version >/dev/null 2>&1")
		defer s.stopLoop("nonroot")

		s.EventuallyWithTf(func(c *assert.CollectT) {
			seen := s.packageProperty(s.inUsePkg, propLastSeenRunning)
			rootVal := s.packageProperty(s.inUsePkg, propRunningAsRoot)
			ts, err := strconv.ParseInt(seen, 10, 64)
			require.NoErrorf(c, err, "%s carries no usable %s property (%q)", s.inUsePkg, propLastSeenRunning, seen)
			s.T().Logf("HOST-PKG-IN-USE[%s] non-root: %s LastSeenRunning=%d RunningAsRoot=%q (startedAt=%d)",
				s.distro.name, s.inUsePkg, ts, rootVal, startedAt)
			require.GreaterOrEqualf(c, ts, startedAt, "%s was not accessed again after the unprivileged loop started at %d, so nothing shows who ran it", s.inUsePkg, startedAt)
			assert.Equalf(c, "false", rootVal, "%s RunningAsRoot should be false, %s only ran as nobody since %d", s.inUsePkg, s.inUseBin, startedAt)
		}, 6*time.Minute, 15*time.Second, "the host SBOM never reported %s as running unprivileged", s.inUsePkg)
	}) {
		return
	}
}

// allHostSBOMs returns every host SBOM payload fakeintake retains, whatever its
// status.
func (s *hostPackageInUseSuite) allHostSBOMs() []*aggregator.SBOMPayload {
	ids, err := s.Fakeintake.GetSBOMIDs()
	if err != nil {
		return nil
	}
	payloads := lo.FlatMap(ids, func(id string, _ int) []*aggregator.SBOMPayload {
		p, err := s.Fakeintake.FilterSBOMs(id)
		if err != nil {
			return nil
		}
		return p
	})
	return lo.Filter(payloads, func(p *aggregator.SBOMPayload, _ int) bool {
		return p.GetType() == sbom.SBOMSourceType_HOST_FILE_SYSTEM
	})
}

// hostSBOMPayloads returns every host SBOM payload that carries a component list.
// A failed scan still sends a payload, with no components and the scan error in
// its body, so the status has to be checked rather than the payload's presence.
func (s *hostPackageInUseSuite) hostSBOMPayloads() []*aggregator.SBOMPayload {
	return lo.Filter(s.allHostSBOMs(), func(p *aggregator.SBOMPayload, _ int) bool {
		return p.Status == sbom.SBOMStatus_SUCCESS && p.GetCyclonedx() != nil
	})
}

// hostScanErrors returns the error of every failed host scan fakeintake retains.
// The Agent's own Trivy scan of the host produces the component list that the
// runtime usage is merged onto, so when it fails there is nothing to enrich and
// naming its error is what tells that apart from the enrichment being broken.
func (s *hostPackageInUseSuite) hostScanErrors() []string {
	var errs []string
	for _, p := range s.allHostSBOMs() {
		if p.Status == sbom.SBOMStatus_FAILED {
			errs = append(errs, p.GetError())
		}
	}
	return lo.Uniq(errs)
}

// packageUsage returns, across every host SBOM payload fakeintake retains, the
// highest LastSeenRunning reported for the named package (the newest access
// wins), whether the property was present at all, and the names of every package
// currently reported as in use, a diagnostic for when the targeted package is not
// the one that flipped.
func (s *hostPackageInUseSuite) packageUsage(c *assert.CollectT, pkg string) (maxTS int64, present bool, inUse []string) {
	payloads := s.hostSBOMPayloads()
	require.NotEmptyf(c, payloads, "no host SBOM carrying components yet; failed host scans report: %v", s.hostScanErrors())

	seen := map[string]struct{}{}
	for _, p := range payloads {
		for _, comp := range p.GetCyclonedx().Components {
			ts, ok := lastSeenRunning(comp)
			if !ok {
				continue
			}
			if comp.GetName() == pkg {
				present = true
				if ts > maxTS {
					maxTS = ts
				}
			}
			if ts > 0 {
				if _, dup := seen[comp.GetName()]; !dup {
					seen[comp.GetName()] = struct{}{}
					inUse = append(inUse, comp.GetName())
				}
			}
		}
	}
	return maxTS, present, inUse
}

// packageProperty returns the value of the named runtime property on the given
// package, read from the most recently collected host payload that carries it.
func (s *hostPackageInUseSuite) packageProperty(pkg, name string) string {
	var value string
	var newest time.Time
	for _, p := range s.hostSBOMPayloads() {
		comp := findComponent(p.GetCyclonedx().Components, pkg)
		if comp == nil {
			continue
		}
		if vals := propertyValues(comp.GetProperties(), name); len(vals) > 0 && !p.GetCollectedTime().Before(newest) {
			newest = p.GetCollectedTime()
			value = vals[len(vals)-1]
		}
	}
	return value
}

// pkgOwning returns the name of the package owning the given path.
func (s *hostPackageInUseSuite) pkgOwning(path string) string {
	out := s.Env().RemoteHost.MustExecute(fmt.Sprintf(`set -- %q; %s`, path, s.distro.pkgQuery))
	return strings.TrimSpace(out)
}

// firstPresent returns the first of the given paths that exists on the host.
func (s *hostPackageInUseSuite) firstPresent(paths ...string) string {
	for _, path := range paths {
		if out, err := s.Env().RemoteHost.Execute(fmt.Sprintf("test -x %q && echo yes", path)); err == nil && strings.TrimSpace(out) == "yes" {
			return path
		}
	}
	s.T().Fatalf("none of %v is installed on the host", paths)
	return ""
}

// startLoop launches a detached loop running body every 15s, which is longer than
// the resolver's 10s enrichment interval so every iteration re-arms its forwarding
// debouncer. A tighter loop forwards once and then suppresses re-forwards within
// the enrichment interval, which makes the single forward race the host scan.
func (s *hostPackageInUseSuite) startLoop(name, body string) {
	script := fmt.Sprintf(
		`sudo nohup sh -c 'echo $$ > /tmp/%s.pid; while true; do %s; sleep 15; done' </dev/null >/dev/null 2>&1 &`,
		name, body)
	s.Env().RemoteHost.MustExecute(script)
	s.T().Logf("HOST-PKG-IN-USE[%s] started the %s loop", s.distro.name, name)
}

// stopLoop stops the loop started by startLoop.
func (s *hostPackageInUseSuite) stopLoop(name string) {
	out, err := s.Env().RemoteHost.Execute(
		fmt.Sprintf(`sudo sh -c 'kill "$(cat /tmp/%s.pid)" 2>/dev/null; rm -f /tmp/%s.pid'; echo stopped`, name, name))
	s.T().Logf("HOST-PKG-IN-USE[%s] stopped the %s loop: %q err=%v", s.distro.name, name, strings.TrimSpace(out), err)
}

// hostEpoch returns the host's wall clock in Unix seconds, which is the clock
// that stamps LastSeenRunning.
func (s *hostPackageInUseSuite) hostEpoch() int64 {
	out := s.Env().RemoteHost.MustExecute("date +%s")
	n, _ := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	return n
}

// TestZZHostDiagnostics runs last (the ZZ prefix sorts it after the phases) and
// dumps the state that explains a failure above: the resolved SBOM settings, the
// host SBOM section of the Agent status, and the tail of the system-probe log. It
// is a debugging aid and always passes.
func (s *hostPackageInUseSuite) TestZZHostDiagnostics() {
	for _, step := range []struct {
		label string
		cmd   string
	}{
		{"sbom-status", "sudo datadog-agent status 2>/dev/null | grep -A25 -i 'SBOM'"},
		{"sockets", "sudo ls -la /opt/datadog-agent/run/"},
		{"sysprobe-log", "sudo tail -60 /var/log/datadog/system-probe.log"},
		{"agent-log-sbom", "sudo grep -iE 'sbom|package in use|scan error|stat root' /var/log/datadog/agent.log | tail -40"},
		{"host-scan-errors", "sudo grep -iE 'scan error|stat root|host scan request' /var/log/datadog/agent.log | tail -20"},
	} {
		out, err := s.Env().RemoteHost.Execute(step.cmd)
		s.T().Logf("DIAG[%s] err=%v\n%s", step.label, err, out)
	}
}
