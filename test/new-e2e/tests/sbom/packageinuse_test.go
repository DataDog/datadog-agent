// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package sbom

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	"github.com/DataDog/agent-payload/v5/sbom"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/sbomtargets"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	scenkubeadm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kubeadm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	provkubeadm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kubeadm"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
)

// Runtime-usage ("package in use") properties merged onto the container-image
// SBOM components by the core-agent SBOM collector from the system-probe SBOM
// resolver. "Not in use" is reported as LastSeenRunning == "0"; "in use" is a
// recent Unix timestamp (seconds). HasSetSuidBit / RunningAsRoot are "true" /
// "false". See pkg/security/resolvers/sbom/report.go and
// comp/core/workloadmeta/collectors/internal/remote/sbomcollector.
const (
	propLastSeenRunning = "LastSeenRunning"
	propHasSetSuidBit   = "HasSetSuidBit"
	propRunningAsRoot   = "RunningAsRoot"
)

// propUsageObservedSince names the metadata property of an enriched SBOM holding
// when the usage it carries started being recorded, as a Unix timestamp
// (seconds). Until the usage window of its workload closes, one refresh period
// later, an unseen package goes out stripped of its runtime properties.
const propUsageObservedSince = "UsageObservedSince"

// usageWindow is how long the usage of a workload is recorded before an unseen
// package reads "0": the refresh period the suites set for the host and the
// images, in packageInUseHelmValues.
const usageWindow = time.Minute

const (
	// inUseWindow is the maximum age a LastSeenRunning timestamp may have while we
	// consider the package "in use". It must comfortably exceed the end-to-end
	// latency (enrichment forward + container SBOM periodic refresh) so a freshly
	// re-emitted payload still reads as recent.
	inUseWindowSec = int64(150)
	// staleWindow is the age past which a frozen LastSeenRunning is considered
	// "no longer in use". Greater than inUseWindow so the two phases never overlap.
	staleWindowSec = int64(210)
)

// pkgInUseDistro parameterizes the package-in-use phases for one workload so the
// same not-in-use -> in-use -> stale -> security -> refresh cycle runs against
// each package format (rpm, dpkg, apk). Package/binary names and the commands
// that drop privileges or write the package database differ per distro; the
// enrichment behaviour under test does not.
type pkgInUseDistro struct {
	name       string // subtest name + log tag
	workload   string // sbomtargets deployment/label (app=)
	repo       string // fakeintake SBOM id match: strings.Contains(id, repo+"@")
	inUsePkg   string // package that flips not-in-use -> in-use
	inUseBin   string // binary run as "<inUseBin> --version"
	controlPkg string // always-in-use positive control (owns the cat the keep-alive runs)
	suidPkg    string // package owning the setuid binary
	suidCmd    string // shell to exec the setuid binary
	stickyCmd  string // shell to exec a NON-setuid binary of suidPkg (stickiness check); "" to skip
	nonRootPkg string // package run only as nobody
	nonRootCmd string // shell that runs a nonRootPkg binary as the nobody user
	dbProbe    string // existing-file path under the package DB dir to write for the refresh trigger
}

// pkgInUseDistros are the workloads exercised end to end. gzip/curl are the
// in-use packages (a real OS package whose binary the idle `tail` workload never
// runs); the setuid coverage uses each distro's setuid binary (util-linux `su`
// on rpm/dpkg, iputils-ping `ping` on apk); non-root coverage runs a package
// only as `nobody`. Alpine has no setuid binary in a stock image, so the apk
// workload is wbitt/network-multitool, whose apk-owned `ping` is setuid-root.
var pkgInUseDistros = []pkgInUseDistro{
	{name: "ubi9", workload: "sbom-ubi9", repo: "registry.access.redhat.com/ubi9/ubi",
		inUsePkg: "gzip", inUseBin: "gzip", controlPkg: "coreutils-single",
		suidPkg: "util-linux", suidCmd: "su --version >/dev/null 2>&1", stickyCmd: "cal >/dev/null 2>&1",
		nonRootPkg: "grep", nonRootCmd: "/usr/sbin/chroot --userspec=nobody / /usr/bin/grep --version >/dev/null 2>&1",
		dbProbe: "/var/lib/rpm/.sbom-refresh-probe"},
	{name: "ubuntu", workload: "sbom-ubuntu", repo: "ubuntu",
		inUsePkg: "gzip", inUseBin: "gzip", controlPkg: "coreutils",
		suidPkg: "util-linux", suidCmd: "su --version >/dev/null 2>&1", stickyCmd: "",
		nonRootPkg: "grep", nonRootCmd: "/usr/sbin/chroot --userspec=nobody / /usr/bin/grep --version >/dev/null 2>&1",
		dbProbe: "/var/lib/dpkg/.sbom-refresh-probe"},
	{name: "alpine", workload: "sbom-alpine", repo: "wbitt/network-multitool",
		inUsePkg: "curl", inUseBin: "curl", controlPkg: "busybox",
		suidPkg: "iputils-ping", suidCmd: "ping -c 1 -W 1 127.0.0.1 >/dev/null 2>&1", stickyCmd: "",
		nonRootPkg: "jq", nonRootCmd: `su -s /bin/sh nobody -c "/usr/bin/jq --version" >/dev/null 2>&1`,
		dbProbe: "/lib/apk/db/.sbom-refresh-probe"},
}

// packageInUseHelmValues extends the container-image SBOM Helm values
// (overlayfs direct scan, os+languages analyzers) for the "package in use"
// enrichment on a containerd kubeadm node. The host is scanned every minute,
// since a usage report of the host rides the next host scan, hourly by
// default. cwsHelmValues and usageOnlyHelmValues complete them, with CWS on
// and off.
func packageInUseHelmValues() string {
	return `datadog:
  criSocketPath: /run/containerd/containerd.sock
  confd:
    sbom.yaml: |-
      ad_identifiers:
        - _sbom
      init_config:
      instances:
        - periodic_refresh_seconds: 60
          host_periodic_refresh_seconds: 60
  kubelet:
    tlsVerify: false
  useHostPID: true
  sbom:
    containerImage:
      enabled: true
      uncompressedLayersSupport: true
      overlayFSDirectScan: true
      analyzers: ["os", "languages"]
agents:
  useHostNetwork: true
  volumeMounts:
    - name: trivycache
      mountPath: /root/.cache/trivy
    - name: imageoverlay
      mountPath: /var/lib/containerd
      readOnly: true
  volumes:
    - name: trivycache
      emptyDir: {}
    - name: imageoverlay
      hostPath:
        path: /var/lib/containerd
`
}

// cwsHelmValues turns on, with CWS:
//   - the system-probe security module + SBOM resolver
//     (DD_RUNTIME_SECURITY_CONFIG_SBOM_ENABLED) that tracks which packages a
//     running process accesses, and
//   - the core-agent enrichment collector (DD_SBOM_ENRICHMENT_USAGE_ENABLED) that
//     merges those runtime properties onto the Trivy container-image SBOM.
//
// The enrichment interval is shortened so a package's in-use timestamp surfaces
// within the test window instead of the 1m default.
func cwsHelmValues() string {
	return `datadog:
  securityAgent:
    runtime:
      enabled: true
agents:
  containers:
    agent:
      env:
        - name: DD_SBOM_ENRICHMENT_USAGE_ENABLED
          value: "true"
        # Must live on the agent container: streaming wipes the security-agent's own env.
        - name: DD_RUNTIME_SECURITY_CONFIG_ENABLED
          value: "true"
    systemProbe:
      env:
        # The UsageConsumer that registers the SBOMCollector gRPC stream the core
        # agent consumes is created by system-probe only when it sees
        # sbom.enrichment.usage.enabled, so this must be set on the system-probe
        # container too (not just the core agent), else the agent's collector gets
        # "unknown service datadog.sbom.SBOMCollector".
        - name: DD_SBOM_ENRICHMENT_USAGE_ENABLED
          value: "true"
        - name: DD_RUNTIME_SECURITY_CONFIG_SBOM_ENABLED
          value: "true"
        - name: DD_RUNTIME_SECURITY_CONFIG_SBOM_ENRICHMENT_INTERVAL
          value: "10s"
`
}

// usageOnlyHelmValues turns on the usage enrichment of the Helm chart, with
// CWS off. The chart then runs system-probe for the enrichment alone, with
// sbom.enrichment.usage.enabled on system-probe and the core agent and with
// HOST_ROOT unset. The enrichment interval is shortened as in cwsHelmValues.
func usageOnlyHelmValues() string {
	return `datadog:
  sbom:
    enrichment:
      usage:
        enabled: true
agents:
  containers:
    systemProbe:
      env:
        - name: DD_RUNTIME_SECURITY_CONFIG_SBOM_ENRICHMENT_INTERVAL
          value: "10s"
`
}

type packageInUseSuite struct {
	baseSuite[environments.Kubernetes]

	// usageOnly is set when system-probe runs the usage enrichment alone, with
	// CWS and its rules that refresh the SBOMs off.
	usageOnly bool
}

// usageOnlyKubeadmSuite is the package-in-use suite with CWS off, under a
// type of its own, as the stack of a suite takes the name of its type.
type usageOnlyKubeadmSuite struct {
	packageInUseSuite
}

// packageInUseProvisioner provisions the RHEL 10 single-node kubeadm cluster of
// TestSBOMKubeadmSuite with the SBOM workloads, and the Agent with
// packageInUseHelmValues and helmValues.
func packageInUseProvisioner(helmValues string) provisioners.TypedProvisioner[environments.Kubernetes] {
	return provkubeadm.Provisioner(
		provkubeadm.WithRunOptions(
			scenkubeadm.WithVMOptions(
				scenec2.WithOS(e2eos.RedHat10),
				scenec2.WithInstanceType("t3.2xlarge"),
			),
			scenkubeadm.WithFakeintakeOptions(fakeintake.WithMemory(2048), fakeintake.WithRetentionPeriod(sbomHostRetentionPeriod)),
			scenkubeadm.WithDeploySBOMWorkloads(),
			scenkubeadm.WithAgentOptions(
				kubernetesagentparams.WithDualShipping(),
				kubernetesagentparams.WithTimeout(900),
				kubernetesagentparams.WithHelmValues(packageInUseHelmValues()),
				kubernetesagentparams.WithHelmValues(helmValues),
			),
		),
	)
}

// TestSBOMPackageInUseKubeadmSuite provisions the same RHEL 10 single-node
// kubeadm cluster as TestSBOMKubeadmSuite, but additionally enables the CWS SBOM
// resolver and the core-agent usage enrichment, then verifies the "package in
// use" feature end to end across package formats, ubi9 (rpm), ubuntu (dpkg) and
// alpine (apk). A package goes from not in use, to in use once a service runs
// its binary, and back to stale once the service stops.
func TestSBOMPackageInUseKubeadmSuite(t *testing.T) {
	e2e.Run(t, &packageInUseSuite{}, e2e.WithProvisioner(packageInUseProvisioner(cwsHelmValues())))
}

// TestSBOMUsageOnlyKubeadmSuite runs the package-in-use checks on the same
// cluster with the usage enrichment of the Helm chart and CWS off. The refresh
// checks need the rules of CWS, and skip.
func TestSBOMUsageOnlyKubeadmSuite(t *testing.T) {
	e2e.Run(t, &usageOnlyKubeadmSuite{packageInUseSuite{usageOnly: true}}, e2e.WithProvisioner(packageInUseProvisioner(usageOnlyHelmValues())))
}

func (s *packageInUseSuite) SetupSuite() {
	s.baseSuite.SetupSuite()
	s.clusterName = s.Env().KubernetesCluster.ClusterName
	s.Fakeintake = s.Env().FakeIntake.Client()
}

// Test00UpAndRunning waits (the 00 prefix runs it first) for the Agent DaemonSet
// pods to be ready before the package-in-use assertions run, and checks the
// premise of the usage-only suite.
func (s *packageInUseSuite) Test00UpAndRunning() {
	err := s.Env().WaitForAgentReady(
		s.T().Context(),
		environments.WithLinuxNodeAgentReady(),
		environments.WithAgentReadinessTimeout(10*time.Minute),
	)
	s.Require().NoError(err, "Not all agents eventually became ready in time.")

	if s.usageOnly {
		s.assertUsageOnlyDeployment()
	}
}

// assertUsageOnlyDeployment checks the premise of the usage-only suite: the
// containers of the Agent pod, the environment of system-probe, the consumers
// system-probe brings up, and the root it reads the host packages through.
func (s *packageInUseSuite) assertUsageOnlyDeployment() {
	agent := s.nodeAgent(s.T())
	var systemProbe *corev1.Container
	for i, c := range agent.Spec.Containers {
		s.NotEqualf("security-agent", c.Name, "the Agent pod %s runs the security-agent", agent.Name)
		if c.Name == "system-probe" {
			systemProbe = &agent.Spec.Containers[i]
		}
	}
	s.Require().NotNilf(systemProbe, "the Agent pod %s runs no system-probe", agent.Name)
	for _, env := range systemProbe.Env {
		s.NotEqualf("HOST_ROOT", env.Name, "system-probe runs with HOST_ROOT=%s", env.Value)
	}

	s.EventuallyWithTf(func(c *assert.CollectT) {
		log := s.systemProbeLog(c, 0)
		assert.Truef(c, strings.Contains(log, "event monitoring usage consumer initialized"), "system-probe logged no usage consumer")
		assert.Falsef(c, strings.Contains(log, "event monitoring cws consumer initialized"), "system-probe logged the CWS consumer")
		assert.NotEmpty(c, hostScans(c, log), "system-probe logged no scan of the host packages through the root of init")
	}, 5*time.Minute, 10*time.Second, "system-probe never ran the usage enrichment alone")
}

// TestPackageInUse drives the full not-in-use -> in-use -> stale -> security ->
// refresh cycle for each workload (rpm, dpkg, apk) as a nested subtest, then
// checks the scope and the shape of the SBOMs the enrichment merges into.
func (s *packageInUseSuite) TestPackageInUse() {
	for _, d := range pkgInUseDistros {
		s.Run(d.name, func() {
			s.runPackageInUse(d)
		})
	}

	s.Run("out-of-scope-components", func() {
		s.runOutOfScopeComponents()
	})
	s.Run("component-list-preserved", func() {
		s.runComponentListPreserved()
	})
}

func (s *packageInUseSuite) runPackageInUse(d pkgInUseDistro) {
	repo := d.repo

	// Keep the idle workload forwarding its runtime SBOM steadily (without
	// touching the in-use package) so the enrichment merge reliably runs once its
	// image SBOM lands.
	s.keepActive(d)

	// Phase 1: baseline. The enrichment merge must have run (the in-use package
	// carries a LastSeenRunning property at all) and it must be reported "not in
	// use" (LastSeenRunning == "0"): the idle workload only runs `tail`.
	s.Run("not-in-use", func() {
		s.EventuallyWithTf(func(collect *assert.CollectT) {
			c := &myCollectT{CollectT: collect, errors: []error{}}
			collect = nil //nolint:ineffassign

			ts, present, inUse := s.packageUsage(c, repo, d.inUsePkg)
			require.Truef(c, present, "no enriched %s SBOM yet (%s carries no %s property)", d.name, d.inUsePkg, propLastSeenRunning)
			// Positive control: a package the keep-alive actually runs must be in
			// use, proving enrichment is flowing - otherwise inUsePkg=="0" below would
			// also hold for a dead pipeline (the property now defaults to "0").
			ctrlTS, _, _ := s.packageUsage(c, repo, d.controlPkg)
			require.Positivef(c, ctrlTS, "positive control %s not in use - enrichment not flowing; %s=0 cannot be trusted", d.controlPkg, d.inUsePkg)
			s.T().Logf("PKG-IN-USE[%s] baseline: %s LastSeenRunning=%d; %s(control)=%d; in-use components=%v", d.name, d.inUsePkg, ts, d.controlPkg, ctrlTS, inUse)
			assert.Zerof(c, ts, "%s should be not-in-use at baseline, got LastSeenRunning=%d", d.inUsePkg, ts)
			// 14m: the enrichment can only merge once the workload's overlayfs Trivy
			// SBOM is ready in workloadmeta, which lands ~10-15m into the run.
		}, 14*time.Minute, 15*time.Second, "%s SBOM never reported %s as not-in-use", d.name, d.inUsePkg)

		starts := lo.CountBy(s.repoUsageSBOMs(s.T(), repo), func(sb usageSBOM) bool { return !sb.since.IsZero() })
		s.Positivef(starts, "no %s SBOM holds the start of its usage", d.name)
	})

	// The first SBOM of the image in use waits for the usage of the image,
	// which merges before it goes out, so every SBOM of the image in use
	// carries it. An SBOM sent before the first container of the image runs
	// goes out unused, as it is.
	s.Run("first-sbom-enriched", func() {
		s.EventuallyWithTf(func(c *assert.CollectT) {
			enriched, raw := s.repoPayloads(c, repo)
			s.T().Logf("PKG-IN-USE[%s] first-sbom: %d SBOMs with usage, %d without", d.name, enriched, raw)
			require.Positivef(c, enriched, "no %s SBOM with usage", d.name)
			assert.Zerof(c, raw, "%d %s SBOMs went out without the usage of the image", raw, d.name)
		}, time.Minute, 15*time.Second, "%s SBOMs went out without usage", d.name)
	})

	// Phase 2: start a service that repeatedly runs the in-use binary, and verify
	// the package flips to in-use (a recent LastSeenRunning timestamp).
	s.Run("in-use", func() {
		// Node-clock instant just before the workload starts running the binary; the
		// observed timestamp must be at or after this, proving it reflects a real
		// access from this phase rather than a stale or coincidental value.
		startedAt := s.nodeEpoch(d)
		s.startInUseService(d)

		s.EventuallyWithTf(func(collect *assert.CollectT) {
			c := &myCollectT{CollectT: collect, errors: []error{}}
			collect = nil //nolint:ineffassign

			ts, present, inUse := s.packageUsage(c, repo, d.inUsePkg)
			require.Truef(c, present, "%s carries no %s property", d.inUsePkg, propLastSeenRunning)
			require.Positivef(c, ts, "%s still reported not-in-use (LastSeenRunning=0); in-use components=%v", d.inUsePkg, inUse)
			age := time.Now().Unix() - ts
			s.T().Logf("PKG-IN-USE[%s] running: %s LastSeenRunning=%d age=%ds startedAt=%d; in-use components=%v", d.name, d.inUsePkg, ts, age, startedAt, inUse)
			assert.GreaterOrEqualf(c, ts, startedAt, "%s LastSeenRunning %d predates the service start %d (stale/coincidental value)", d.inUsePkg, ts, startedAt)
			assert.LessOrEqualf(c, age, inUseWindowSec, "%s LastSeenRunning is %ds old, expected <= %ds while in use", d.inUsePkg, age, inUseWindowSec)
			// The workload runs as root and the in-use binary is not setuid, so the
			// security enrichment must reflect that on the in-use component.
			assert.Equalf(c, "true", s.packageProperty(repo, d.inUsePkg, propRunningAsRoot), "%s RunningAsRoot should be true (workload runs as root)", d.inUsePkg)
			assert.Equalf(c, "false", s.packageProperty(repo, d.inUsePkg, propHasSetSuidBit), "%s HasSetSuidBit should be false (not a setuid binary)", d.inUsePkg)
		}, 5*time.Minute, 15*time.Second, "%s SBOM never reported %s as in-use after starting the service", d.name, d.inUsePkg)
	})

	// Phase 3: stop the service and verify the package ages out of the in-use
	// window. LastSeenRunning is a monotonic "last seen" timestamp that is not
	// reset on process exit, so "back to not in use" is observed as the timestamp
	// freezing and growing stale rather than returning to "0".
	s.Run("stale-after-stop", func() {
		s.stopInUseService(d)

		s.EventuallyWithTf(func(collect *assert.CollectT) {
			c := &myCollectT{CollectT: collect, errors: []error{}}
			collect = nil //nolint:ineffassign

			ts, _, _ := s.packageUsage(c, repo, d.inUsePkg)
			require.Positivef(c, ts, "%s was never reported in-use, cannot assert staleness", d.inUsePkg)
			age := time.Now().Unix() - ts
			s.T().Logf("PKG-IN-USE[%s] stopped: %s LastSeenRunning=%d age=%ds", d.name, d.inUsePkg, ts, age)
			assert.Greaterf(c, age, staleWindowSec, "%s LastSeenRunning is only %ds old, expected > %ds (stale/not running)", d.inUsePkg, age, staleWindowSec)
		}, 5*time.Minute, 20*time.Second, "%s SBOM never reported %s as stale after stopping the service", d.name, d.inUsePkg)
	})

	// Phase 4: security properties. Cover the property values the in-use phases do
	// not: HasSetSuidBit == "true" (a setuid-root binary is run, owned by suidPkg)
	// and RunningAsRoot == "false" (nonRootPkg is run only as the unprivileged
	// nobody user).
	s.Run("security-properties", func() {
		s.startSecurityProbes(d)
		defer s.stopSecurityProbes(d)

		s.EventuallyWithTf(func(collect *assert.CollectT) {
			c := &myCollectT{CollectT: collect, errors: []error{}}
			collect = nil //nolint:ineffassign

			suidTS, suidPresent, _ := s.packageUsage(c, repo, d.suidPkg)
			require.Truef(c, suidPresent, "%s carries no %s property yet", d.suidPkg, propLastSeenRunning)
			require.Positivef(c, suidTS, "%s was never reported in use", d.suidPkg)

			nrTS, nrPresent, _ := s.packageUsage(c, repo, d.nonRootPkg)
			require.Truef(c, nrPresent, "%s carries no %s property yet", d.nonRootPkg, propLastSeenRunning)
			require.Positivef(c, nrTS, "%s was never reported in use", d.nonRootPkg)

			suidVal := s.packageProperty(repo, d.suidPkg, propHasSetSuidBit)
			rootVal := s.packageProperty(repo, d.nonRootPkg, propRunningAsRoot)
			s.T().Logf("PKG-IN-USE[%s] security: %s HasSetSuidBit=%q (ts=%d), %s RunningAsRoot=%q (ts=%d)", d.name, d.suidPkg, suidVal, suidTS, d.nonRootPkg, rootVal, nrTS)

			assert.Equalf(c, "true", suidVal, "%s HasSetSuidBit should be true (a setuid-root binary of it was run)", d.suidPkg)
			assert.Equalf(c, "false", rootVal, "%s RunningAsRoot should be false (run only as nobody)", d.nonRootPkg)
		}, 5*time.Minute, 15*time.Second, "%s SBOM never reported the expected security properties", d.name)
	})

	// Phase 5: refresh reset. Writing the package database and exiting fires the
	// bundled need_refresh_sbom / refresh_sbom rules, re-scanning the workload and
	// zeroing its runtime properties. This is the only path back to "0": stopping
	// a service merely freezes the timestamp. The refresh restarts the usage
	// window of the image, so the in-use package, stopped by then, loses its
	// runtime properties until the window closes, and reads "0" after.
	s.Run("refresh-reset", func() {
		if s.usageOnly {
			s.T().Skip("the rules that refresh the SBOMs run with CWS")
		}
		refreshedAt := time.Unix(s.nodeEpoch(d), 0)
		s.triggerSBOMRefresh(d)

		s.EventuallyWithTf(func(collect *assert.CollectT) {
			c := &myCollectT{CollectT: collect, errors: []error{}}
			collect = nil //nolint:ineffassign

			// Read the newest payload, not the max across payloads: the max would
			// still see the earlier in-use payloads and never observe the reset.
			v := s.packageProperty(repo, d.inUsePkg, propLastSeenRunning)
			require.NotEmptyf(c, v, "%s carries no %s property", d.inUsePkg, propLastSeenRunning)
			s.T().Logf("PKG-IN-USE[%s] refresh: %s LastSeenRunning=%q", d.name, d.inUsePkg, v)
			assert.Equalf(c, "0", v, "%s LastSeenRunning should reset to 0 after a package-DB refresh, got %q", d.inUsePkg, v)
		}, 6*time.Minute, 20*time.Second, "%s SBOM never reset %s to 0 after the package-DB refresh", d.name, d.inUsePkg)

		// The keep-alive runs the control package all along, so an SBOM of the
		// window shows it seen since the refresh.
		inWindow, controlSeen := 0, false
		for _, sb := range s.repoUsageSBOMs(s.T(), repo) {
			if open, _ := usageWindowOf(sb, usageWindow); !open || sb.since.Before(refreshedAt) {
				continue
			}
			inWindow++
			inUse := findComponent(sb.components, d.inUsePkg)
			s.Require().NotNilf(inUse, "no %s in the %s SBOM", d.inUsePkg, d.name)
			s.Emptyf(propertyValues(inUse.GetProperties(), propLastSeenRunning), "%s carries %s in the usage window the refresh opened", d.inUsePkg, propLastSeenRunning)
			if ts, _ := lastSeenRunning(findComponent(sb.components, d.controlPkg)); ts >= sb.since.Unix() {
				controlSeen = true
			}
		}
		s.T().Logf("PKG-IN-USE[%s] refresh: %d SBOMs in the usage window the refresh opened", d.name, inWindow)
		s.Positivef(inWindow, "no %s SBOM went out in the usage window the refresh opened", d.name)
		s.Truef(controlSeen, "no %s SBOM of the window shows %s seen since the refresh", d.name, d.controlPkg)
	})
}

// hostShellContainer names the container of the pod startHostShell runs.
const hostShellContainer = "shell"

// TestHostPackageInUse checks the runtime usage enrichment of the host SBOM: a
// package the host itself runs carries the properties the image SBOMs carry.
// Each subtest runs its own host processes.
func (s *packageInUseSuite) TestHostPackageInUse() {
	shell := s.startHostShell()

	// The first host SBOM waits for the usage of the host, so it holds that of
	// kubelet, which starts with the node before the Agent. A second host SBOM
	// settles which one came first.
	s.Run("first-sbom", func() {
		var first []*cyclonedx_v1_4.Component
		s.EventuallyWithTf(func(c *assert.CollectT) {
			var bodies int
			first, bodies = oldestHostSBOM(c, s.Fakeintake)
			require.GreaterOrEqualf(c, bodies, 2, "a single host SBOM with a body in fake intake yet")
			// 10m: run on its own, the subtest also waits out the Agent start and
			// its first host scans.
		}, 10*time.Minute, 15*time.Second, "the host SBOM never went out twice")

		kubelet := findComponent(first, "kubelet")
		s.Require().NotNil(kubelet, "no kubelet in the first host SBOM")
		ts, _ := lastSeenRunning(kubelet)
		s.T().Logf("PKG-IN-USE[host] first-sbom: kubelet LastSeenRunning=%d", ts)
		s.Positivef(ts, "the first host SBOM went out without the usage of kubelet")
		s.Equalf([]string{"true"}, propertyValues(kubelet.GetProperties(), propRunningAsRoot), "kubelet %s in the first host SBOM", propRunningAsRoot)
	})

	// kubelet starts with the node, before the Agent, so its use comes from
	// the processes the probe finds running when it starts.
	s.Run("daemon", func() {
		s.EventuallyWithTf(func(c *assert.CollectT) {
			kubelet := findComponent(newestHostSBOM(c, s.Fakeintake, time.Time{}), "kubelet")
			require.NotNilf(c, kubelet, "no kubelet in the host SBOM")
			ts, _ := lastSeenRunning(kubelet)
			s.T().Logf("PKG-IN-USE[host] daemon: kubelet LastSeenRunning=%d", ts)
			assert.Positivef(c, ts, "kubelet, running since the node started, is unused")
			assert.Equalf(c, []string{"true"}, propertyValues(kubelet.GetProperties(), propRunningAsRoot), "kubelet %s, kubelet runs as root", propRunningAsRoot)
			// 10m: run on its own, the subtest also waits out the Agent start and
			// its first host scans.
		}, 10*time.Minute, 15*time.Second, "the host SBOM never reported kubelet in use")
	})

	// rpm lists the directories a package owns among its files, and a listing
	// of the license directory of gzip leaves gzip unused. sed, run after the
	// listing, is the positive control: the host SBOM that carries its run
	// carries the listing too.
	s.Run("directory", func() {
		listedAt := s.hostEpoch(shell)

		s.EventuallyWithTf(func(collect *assert.CollectT) {
			c := &myCollectT{CollectT: collect, errors: []error{}}
			collect = nil //nolint:ineffassign

			s.hostExec(c, shell, "ls /usr/share/licenses/gzip >/dev/null && sed --version >/dev/null")

			comps := newestHostSBOM(c, s.Fakeintake, time.Time{})
			sed := findComponent(comps, "sed")
			require.NotNilf(c, sed, "no sed in the host SBOM")
			sedTS, _ := lastSeenRunning(sed)
			require.GreaterOrEqualf(c, sedTS, listedAt, "sed not reported in use yet")
			gzip := findComponent(comps, "gzip")
			require.NotNilf(c, gzip, "no gzip in the host SBOM")
			gzipTS, _ := lastSeenRunning(gzip)
			s.T().Logf("PKG-IN-USE[host] directory: gzip LastSeenRunning=%d, sed LastSeenRunning=%d, listed at %d", gzipTS, sedTS, listedAt)
			assert.Lessf(c, gzipTS, listedAt, "the listing of /usr/share/licenses/gzip put gzip in use")
			// 10m: run on its own, the subtest also waits out the Agent start and
			// its first host scans.
		}, 10*time.Minute, 15*time.Second, "the host SBOM never reported sed in use after the listing")
	})

	s.Run("in-use", func() {
		startedAt := s.hostEpoch(shell)

		s.EventuallyWithTf(func(collect *assert.CollectT) {
			c := &myCollectT{CollectT: collect, errors: []error{}}
			collect = nil //nolint:ineffassign

			s.hostExec(c, shell, "gzip --version >/dev/null")

			comps := newestHostSBOM(c, s.Fakeintake, time.Time{})
			gzip := findComponent(comps, "gzip")
			require.NotNilf(c, gzip, "no gzip in the host SBOM")
			ts, present := lastSeenRunning(gzip)
			glibc := findComponent(comps, "glibc")
			require.NotNilf(c, glibc, "no glibc in the host SBOM")
			glibcTS, _ := lastSeenRunning(glibc)
			s.T().Logf("PKG-IN-USE[host] gzip LastSeenRunning=%d present=%v, glibc LastSeenRunning=%d, startedAt=%d", ts, present, glibcTS, startedAt)
			require.Truef(c, present, "gzip carries no %s yet", propLastSeenRunning)
			assert.GreaterOrEqualf(c, ts, startedAt, "gzip LastSeenRunning %d predates the run at %d", ts, startedAt)
			// The loader opens libc.so.6 for every process of the run, and the
			// probe samples those opens.
			assert.GreaterOrEqualf(c, glibcTS, startedAt, "glibc LastSeenRunning %d predates the run at %d", glibcTS, startedAt)
			assert.Equalf(c, []string{"true"}, propertyValues(gzip.GetProperties(), propRunningAsRoot), "gzip %s, the host runs it as root", propRunningAsRoot)
			assert.Equalf(c, []string{"false"}, propertyValues(gzip.GetProperties(), propHasSetSuidBit), "gzip %s, the gzip package ships no setuid binary", propHasSetSuidBit)
			osComp := findOSComponent(comps)
			if osComp == nil {
				osComp = findComponent(comps, "redhat")
			}
			require.NotNilf(c, osComp, "no OS component in the host SBOM")
			assertNoRuntimeProperties(c, "host", osComp)
			// 10m: run on its own, the subtest also waits out the Agent start and
			// its first host scans.
		}, 10*time.Minute, 15*time.Second, "the host SBOM never reported gzip in use")
	})

	s.Run("setuid", func() {
		s.EventuallyWithTf(func(collect *assert.CollectT) {
			c := &myCollectT{CollectT: collect, errors: []error{}}
			collect = nil //nolint:ineffassign

			s.hostExec(c, shell, "su --version >/dev/null")

			utilLinux := findComponent(newestHostSBOM(c, s.Fakeintake, time.Time{}), "util-linux")
			require.NotNilf(c, utilLinux, "no util-linux in the host SBOM")
			suid := propertyValues(utilLinux.GetProperties(), propHasSetSuidBit)
			s.T().Logf("PKG-IN-USE[host] util-linux HasSetSuidBit=%v", suid)
			assert.Equalf(c, []string{"true"}, suid, "util-linux %s, the host ran its setuid su", propHasSetSuidBit)
		}, 5*time.Minute, 15*time.Second, "the host SBOM never reported the setuid su of util-linux")
	})

	// While the usage of the host was recorded for less than its window, the
	// unseen packages lose their runtime properties. Past the window, every
	// package carries them, and the unseen ones read "0".
	s.Run("unobserved", func() {
		s.EventuallyWithTf(func(c *assert.CollectT) {
			open, closed := 0, 0
			for _, sb := range hostUsageSBOMs(c, s.Fakeintake) {
				inWindow, pastWindow := usageWindowOf(sb, usageWindow)
				if inWindow {
					open++
					assertUsageUnknown(c, sb)
				}
				if pastWindow {
					closed++
					assertUsageDefaulted(c, sb)
				}
			}
			s.T().Logf("PKG-IN-USE[host] unobserved: %d host SBOMs in the usage window, %d past it", open, closed)
			require.Positivef(c, closed, "no host SBOM past the usage window yet")
		}, 10*time.Minute, 15*time.Second, "the host SBOMs never gave every package its usage past the usage window")
	})

	// A write to the rpm database of the host fires the bundled
	// need_refresh_sbom / refresh_sbom rules, which scan the host packages
	// again. The scan keeps the usage of the packages it finds again, gzip
	// among them. A query of the rpm database, run first, leaves the host
	// packages as they were scanned.
	s.Run("refresh", func() {
		if s.usageOnly {
			s.T().Skip("the rules that refresh the SBOMs run with CWS")
		}

		var used int64
		s.EventuallyWithTf(func(collect *assert.CollectT) {
			c := &myCollectT{CollectT: collect, errors: []error{}}
			collect = nil //nolint:ineffassign

			s.hostExec(c, shell, "gzip --version >/dev/null")

			gzip := findComponent(newestHostSBOM(c, s.Fakeintake, time.Time{}), "gzip")
			require.NotNilf(c, gzip, "no gzip in the host SBOM")
			used, _ = lastSeenRunning(gzip)
			assert.Positivef(c, used, "gzip not reported in use yet")
		}, 5*time.Minute, 15*time.Second, "the host SBOM never reported gzip in use before the refresh")

		// The rules match a write to an existing file of the package database,
		// so the probe is created first and then written. The query runs 10s
		// before the write, longer than the refresh of the host takes to fire,
		// so a scan logged in between comes from the query.
		const probe = "/var/lib/rpm/.sbom-refresh-probe"
		s.T().Cleanup(func() { s.hostExec(s.T(), shell, "rm -f "+probe) })
		queried := s.hostEpoch(shell)
		out := s.hostExec(s.T(), shell, "rpm -q gzip >/dev/null && sleep 10 && date +%s && touch "+probe+" && echo probe >> "+probe)
		n, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
		s.Require().NoErrorf(err, "date printed %q", out)
		written := time.Unix(n, 0)

		var scanned time.Time
		s.EventuallyWithTf(func(c *assert.CollectT) {
			scans := hostScans(c, s.systemProbeLog(c, queried))
			s.T().Logf("PKG-IN-USE[host] refresh: host scans %v, query at %d, write at %s", scans, queried, written)
			require.NotEmptyf(c, scans, "no scan of the host packages yet")
			scanned = scans[len(scans)-1]
			require.Falsef(c, scanned.Before(written), "no scan of the host packages since the rpm database write")
			assert.Falsef(c, scans[0].Before(written), "the rpm query scanned the host packages at %s", scans[0])
		}, 2*time.Minute, 5*time.Second, "the rpm database write never scanned the host packages alone")

		// The scan forwards the usage of the host, which rides the next host scan
		// of the core agent: the host SBOM sent two minutes later carries it.
		s.EventuallyWithTf(func(collect *assert.CollectT) {
			c := &myCollectT{CollectT: collect, errors: []error{}}
			collect = nil //nolint:ineffassign

			gzip := findComponent(newestHostSBOM(c, s.Fakeintake, scanned.Add(2*time.Minute)), "gzip")
			require.NotNilf(c, gzip, "no gzip in the host SBOM")
			ts, _ := lastSeenRunning(gzip)
			s.T().Logf("PKG-IN-USE[host] refresh: gzip LastSeenRunning=%d, %d before the scan", ts, used)
			assert.GreaterOrEqualf(c, ts, used, "gzip lost the usage recorded before the scan of the host packages")
		}, 6*time.Minute, 20*time.Second, "the host SBOM never carried gzip after the scan of the host packages")
	})
}

// startHostShell runs a privileged pod in the PID namespace of the node, on the
// Agent image the node already holds, and returns its name. The pod goes away
// with the test.
func (s *packageInUseSuite) startHostShell() string {
	ctx := s.T().Context()
	client := s.Env().KubernetesCluster.Client()
	agent := s.nodeAgent(s.T())

	privileged := true
	pod, err := client.CoreV1().Pods(sbomtargets.Namespace).Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{GenerateName: "sbom-host-shell-"},
		Spec: corev1.PodSpec{
			NodeName: agent.Spec.NodeName,
			HostPID:  true,
			Containers: []corev1.Container{{
				Name:            hostShellContainer,
				Image:           agent.Spec.Containers[0].Image,
				Command:         []string{"sleep", "infinity"},
				SecurityContext: &corev1.SecurityContext{Privileged: &privileged},
			}},
			RestartPolicy: corev1.RestartPolicyNever,
			Tolerations:   []corev1.Toleration{{Operator: corev1.TolerationOpExists}},
		},
	}, metav1.CreateOptions{})
	s.Require().NoError(err, "failed to create the host shell pod")
	s.T().Cleanup(func() {
		_ = client.CoreV1().Pods(sbomtargets.Namespace).Delete(context.Background(), pod.Name, metav1.DeleteOptions{})
	})

	s.Require().EventuallyWithTf(func(c *assert.CollectT) {
		p, err := client.CoreV1().Pods(sbomtargets.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
		require.NoError(c, err)
		assert.Equal(c, corev1.PodRunning, p.Status.Phase)
	}, 2*time.Minute, 5*time.Second, "host shell pod %s never ran", pod.Name)

	return pod.Name
}

// nodeAgent returns the Agent pod of the node.
func (s *packageInUseSuite) nodeAgent(t require.TestingT) corev1.Pod {
	agents, err := s.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(s.T().Context(), metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", s.Env().Agent.LinuxNodeAgent.LabelSelectors["app"]).String(),
	})
	require.NoError(t, err, "failed to list the Agent pods")
	require.NotEmpty(t, agents.Items, "no Agent pod on the node")
	return agents.Items[0]
}

// hostExec runs script on the node from shell, as a transient systemd unit
// started in the mount namespace of init, and returns its output. The unit runs
// as root in a cgroup of the host, so its processes are host processes.
func (s *packageInUseSuite) hostExec(t require.TestingT, shell, script string) string {
	cmd := []string{"nsenter", "-t", "1", "-m", "--", "systemd-run", "--wait", "--pipe", "--quiet", "--collect", "/bin/sh", "-c", script}
	stdout, stderr, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(sbomtargets.Namespace, shell, hostShellContainer, cmd)
	require.NoErrorf(t, err, "host exec %q failed: %s", script, stderr)
	return stdout
}

// systemProbeLog returns the log of system-probe since since, a Unix time of
// the node, with the timestamps of the kubelet.
func (s *packageInUseSuite) systemProbeLog(t require.TestingT, since int64) string {
	sinceTime := metav1.NewTime(time.Unix(since, 0))
	logs, err := s.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").GetLogs(s.nodeAgent(t).Name, &corev1.PodLogOptions{
		Container:  "system-probe",
		SinceTime:  &sinceTime,
		Timestamps: true,
	}).DoRaw(s.T().Context())
	require.NoError(t, err, "failed to read the system-probe log")
	return string(logs)
}

// hostScans returns, in order, the times at which log, the system-probe log
// with the timestamps of the kubelet, records a scan of the host packages.
func hostScans(t require.TestingT, log string) []time.Time {
	var scans []time.Time
	for _, line := range strings.Split(log, "\n") {
		stamp, msg, _ := strings.Cut(strings.TrimSpace(line), " ")
		if !strings.Contains(msg, "Generating SBOM for ") || !strings.HasSuffix(msg, "/proc/1/root") {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, stamp)
		require.NoErrorf(t, err, "system-probe log line %q", line)
		scans = append(scans, at)
	}
	return scans
}

// hostEpoch returns the wall clock of the node (Unix seconds), the clock that
// stamps LastSeenRunning.
func (s *packageInUseSuite) hostEpoch(shell string) int64 {
	stdout, stderr, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(sbomtargets.Namespace, shell, hostShellContainer, []string{"date", "+%s"})
	s.Require().NoErrorf(err, "date failed: %s", stderr)
	n, err := strconv.ParseInt(strings.TrimSpace(stdout), 10, 64)
	s.Require().NoError(err, "date printed %q", stdout)
	return n
}

// newestHostSBOM returns the components of the newest full host SBOM intake
// collected after after.
func newestHostSBOM(c require.TestingT, intake *fakeintakeclient.Client, after time.Time) []*cyclonedx_v1_4.Component {
	ids, err := intake.GetSBOMIDs()
	require.NoErrorf(c, err, "Failed to query fake intake")

	newest := after
	var components []*cyclonedx_v1_4.Component
	for _, id := range ids {
		payloads, err := intake.FilterSBOMs(id)
		if err != nil {
			continue
		}
		for _, p := range payloads {
			if p.GetType() != sbom.SBOMSourceType_HOST_FILE_SYSTEM || p.Status != sbom.SBOMStatus_SUCCESS || p.GetCyclonedx() == nil {
				continue
			}
			if p.GetCollectedTime().Before(newest) {
				continue
			}
			newest = p.GetCollectedTime()
			components = p.GetCyclonedx().Components
		}
	}
	require.NotEmptyf(c, components, "no host SBOM with a body collected after %s in fake intake yet", after)
	return components
}

// usageSBOM is an SBOM with a body as the fake intake received it, with the
// start of the usage it carries, if it holds one.
type usageSBOM struct {
	collected  time.Time
	since      time.Time
	components []*cyclonedx_v1_4.Component
}

func newUsageSBOM(collected time.Time, bom *cyclonedx_v1_4.Bom) usageSBOM {
	return usageSBOM{collected: collected, since: usageObservedSince(bom), components: bom.GetComponents()}
}

// usageObservedSince returns the start of the usage bom carries, or the zero
// time.
func usageObservedSince(bom *cyclonedx_v1_4.Bom) time.Time {
	for _, p := range bom.GetMetadata().GetProperties() {
		if p.GetName() != propUsageObservedSince {
			continue
		}
		if seconds, err := strconv.ParseInt(p.GetValue(), 10, 64); err == nil {
			return time.Unix(seconds, 0)
		}
	}
	return time.Time{}
}

// isEnriched reports whether bom went through the runtime enrichment merge: it
// holds the start of its usage, or a component carries its usage. In the usage
// window of its workload, an SBOM whose packages are all unseen holds the start
// alone.
func isEnriched(bom *cyclonedx_v1_4.Bom) bool {
	return !usageObservedSince(bom).IsZero() || lo.SomeBy(bom.GetComponents(), func(comp *cyclonedx_v1_4.Component) bool {
		_, ok := lastSeenRunning(comp)
		return ok
	})
}

// usageWindowOf tells whether sb went out while the usage window of its
// workload, window long, was open, or once it had closed. Around the end of the
// window, which the latency of the sends and the skew of the clocks blur, both
// read false.
func usageWindowOf(sb usageSBOM, window time.Duration) (open, closed bool) {
	if sb.since.IsZero() {
		return false, false
	}
	return sb.collected.Before(sb.since.Add(window - 10*time.Second)), !sb.collected.Before(sb.since.Add(window + 30*time.Second))
}

// hostUsageSBOMs returns the host SBOMs with a body the fake intake holds.
func hostUsageSBOMs(c require.TestingT, intake *fakeintakeclient.Client) []usageSBOM {
	ids, err := intake.GetSBOMIDs()
	require.NoErrorf(c, err, "Failed to query fake intake")

	var sboms []usageSBOM
	for _, id := range ids {
		payloads, err := intake.FilterSBOMs(id)
		if err != nil {
			continue
		}
		for _, p := range payloads {
			if p.GetType() != sbom.SBOMSourceType_HOST_FILE_SYSTEM || p.Status != sbom.SBOMStatus_SUCCESS || p.GetCyclonedx() == nil {
				continue
			}
			sboms = append(sboms, newUsageSBOM(p.GetCollectedTime(), p.GetCyclonedx()))
		}
	}
	return sboms
}

// repoUsageSBOMs returns the successful SBOMs with a body of the images of repo.
func (s *packageInUseSuite) repoUsageSBOMs(c require.TestingT, repo string) []usageSBOM {
	ids, err := s.Fakeintake.GetSBOMIDs()
	require.NoErrorf(c, err, "Failed to query fake intake")

	var sboms []usageSBOM
	for _, id := range ids {
		if !strings.Contains(id, repo+"@") {
			continue
		}
		payloads, err := s.Fakeintake.FilterSBOMs(id)
		if err != nil {
			continue
		}
		for _, p := range payloads {
			if p.GetType() != sbom.SBOMSourceType_CONTAINER_IMAGE_LAYERS || p.Status != sbom.SBOMStatus_SUCCESS || p.GetCyclonedx() == nil {
				continue
			}
			sboms = append(sboms, newUsageSBOM(p.GetCollectedTime(), p.GetCyclonedx()))
		}
	}
	return sboms
}

// isOSPackage reports whether comp is a package of the dpkg, rpm or apk
// databases, the packages runtime usage covers.
func isOSPackage(comp *cyclonedx_v1_4.Component) bool {
	for _, prefix := range []string{"pkg:deb/", "pkg:rpm/", "pkg:apk/"} {
		if strings.HasPrefix(comp.GetPurl(), prefix) {
			return true
		}
	}
	return false
}

// assertUsageUnknown checks that sb, sent in the usage window of its workload,
// leaves the usage of the unseen packages unknown: every LastSeenRunning it
// holds is a positive timestamp, and a package carries its other runtime
// properties with LastSeenRunning alone.
func assertUsageUnknown(c assert.TestingT, sb usageSBOM) {
	for _, comp := range sb.components {
		if ts, ok := lastSeenRunning(comp); ok {
			assert.Positivef(c, ts, "%s reads %s 0 in the usage window, in the SBOM of %s", comp.GetName(), propLastSeenRunning, sb.collected)
			continue
		}
		assert.Emptyf(c, propertyValues(comp.GetProperties(), propHasSetSuidBit), "%s carries %s without %s, in the SBOM of %s", comp.GetName(), propHasSetSuidBit, propLastSeenRunning, sb.collected)
		assert.Emptyf(c, propertyValues(comp.GetProperties(), propRunningAsRoot), "%s carries %s without %s, in the SBOM of %s", comp.GetName(), propRunningAsRoot, propLastSeenRunning, sb.collected)
	}
}

// assertUsageDefaulted checks that sb, a host SBOM sent past the usage window,
// gives most OS packages their usage, some of them "0". A package carries all
// three runtime properties, or none while system-probe has yet to index it.
func assertUsageDefaulted(c assert.TestingT, sb usageSBOM) {
	carried, unknown, unused := 0, 0, 0
	for _, comp := range sb.components {
		if !isOSPackage(comp) {
			continue
		}
		n := 0
		for _, name := range []string{propLastSeenRunning, propHasSetSuidBit, propRunningAsRoot} {
			if len(propertyValues(comp.GetProperties(), name)) > 0 {
				n++
			}
		}
		switch n {
		case 0:
			unknown++
		case 3:
			carried++
			if ts, _ := lastSeenRunning(comp); ts == 0 {
				unused++
			}
		default:
			assert.Failf(c, "partial usage", "%s carries %d of the 3 runtime properties past the usage window, in the SBOM of %s", comp.GetName(), n, sb.collected)
		}
	}
	assert.Greaterf(c, carried, unknown, "%d OS packages carry their usage and %d none past the usage window, in the SBOM of %s", carried, unknown, sb.collected)
	assert.Positivef(c, unused, "no package reads %s 0 past the usage window, in the SBOM of %s", propLastSeenRunning, sb.collected)
}

// oldestHostSBOM returns the components of the first host SBOM with a body the
// fake intake received, and how many host SBOMs with a body it holds.
func oldestHostSBOM(c require.TestingT, intake *fakeintakeclient.Client) ([]*cyclonedx_v1_4.Component, int) {
	ids, err := intake.GetSBOMIDs()
	require.NoErrorf(c, err, "Failed to query fake intake")

	var oldest time.Time
	var components []*cyclonedx_v1_4.Component
	bodies := 0
	for _, id := range ids {
		payloads, err := intake.FilterSBOMs(id)
		if err != nil {
			continue
		}
		for _, p := range payloads {
			if p.GetType() != sbom.SBOMSourceType_HOST_FILE_SYSTEM || p.Status != sbom.SBOMStatus_SUCCESS || p.GetCyclonedx() == nil {
				continue
			}
			bodies++
			if components != nil && !p.GetCollectedTime().Before(oldest) {
				continue
			}
			oldest = p.GetCollectedTime()
			components = p.GetCyclonedx().Components
		}
	}
	require.NotEmptyf(c, components, "no host SBOM with a body in fake intake yet")
	return components, bodies
}

// runOutOfScopeComponents asserts the runtime properties reach the OS packages
// alone. The resolver reads the dpkg, rpm and apk databases, so the image's
// operating-system component and the application packages stay out of its scope,
// and the absence of a property marks them so.
//
// Every image SBOM holds one operating-system component, so every enriched image
// exercises this. The phase stands alone, which suits the leaf-test retry loop in
// tasks/new_e2e_tests.py: it reads whatever enriched payloads the fake intake
// holds, and the Agent's own containers and the control plane keep it supplied.
func (s *packageInUseSuite) runOutOfScopeComponents() {
	s.EventuallyWithTf(func(collect *assert.CollectT) {
		c := &myCollectT{CollectT: collect, errors: []error{}}
		collect = nil //nolint:ineffassign

		images := s.enrichedImages(c)
		require.NotEmptyf(c, images, "no runtime-enriched container SBOM in fake intake")

		for _, img := range images {
			apps := applicationComponents(img.components)
			s.T().Logf("PKG-IN-USE[scope] id=%q components=%d application=%d", img.id, len(img.components), len(apps))

			if osComp := findOSComponent(img.components); osComp != nil {
				assertNoRuntimeProperties(c, img.id, osComp)
			}
			for _, comp := range apps {
				assertNoRuntimeProperties(c, img.id, comp)
			}
		}
		// 15m: run on its own, the phase waits out the first enrichment, which
		// lands ~10-15m in, once the overlayfs Trivy SBOMs reach workloadmeta.
	}, 15*time.Minute, 15*time.Second, "runtime properties kept landing on components the resolver cannot observe")
}

func assertNoRuntimeProperties(c assert.TestingT, id string, comp *cyclonedx_v1_4.Component) {
	for _, name := range []string{propLastSeenRunning, propHasSetSuidBit, propRunningAsRoot} {
		assert.Emptyf(c, propertyValues(comp.GetProperties(), name),
			"%s %q carries %s in %s, but the resolver cannot observe it", comp.GetType(), comp.GetName(), name, id)
	}
}

// enrichedImage holds the components of one image's newest enriched payload.
type enrichedImage struct {
	id         string
	components []*cyclonedx_v1_4.Component
}

// enrichedImages returns, for every container image in the fake intake, the
// components of its newest payload carrying a LastSeenRunning property. That
// property marks the payloads the enrichment merge has been through, which are
// the ones worth asserting on.
func (s *packageInUseSuite) enrichedImages(c *myCollectT) []enrichedImage {
	ids, err := s.Fakeintake.GetSBOMIDs()
	require.NoErrorf(c, err, "Failed to query fake intake")

	var images []enrichedImage
	for _, id := range ids {
		payloads, err := s.Fakeintake.FilterSBOMs(id)
		if err != nil {
			continue
		}

		var newest time.Time
		var components []*cyclonedx_v1_4.Component
		for _, p := range payloads {
			if p.GetType() != sbom.SBOMSourceType_CONTAINER_IMAGE_LAYERS || p.Status != sbom.SBOMStatus_SUCCESS || p.GetCyclonedx() == nil {
				continue
			}
			if p.GetCollectedTime().Before(newest) {
				continue
			}
			comps := p.GetCyclonedx().Components
			if !lo.SomeBy(comps, func(comp *cyclonedx_v1_4.Component) bool {
				_, enriched := lastSeenRunning(comp)
				return enriched
			}) {
				continue
			}
			newest = p.GetCollectedTime()
			components = comps
		}

		if len(components) > 0 {
			images = append(images, enrichedImage{id: id, components: components})
		}
	}
	return images
}

// packageUsage returns, across every successful container-image SBOM payload for
// the given repo retained by fakeintake, the highest LastSeenRunning timestamp
// reported for the named package's component (newest access wins), whether that
// property was present on any payload, and the names of all components currently
// reported as in use (a diagnostic for when the targeted package is not the one
// that flipped).
func (s *packageInUseSuite) packageUsage(c *myCollectT, repo, pkg string) (maxTS int64, present bool, inUse []string) {
	ids, err := s.Fakeintake.GetSBOMIDs()
	require.NoErrorf(c, err, "Failed to query fake intake")
	ids = lo.Filter(ids, func(id string, _ int) bool { return strings.Contains(id, repo+"@") })
	if len(ids) == 0 {
		s.dumpSBOMInventory()
	}
	require.NotEmptyf(c, ids, "No SBOM id for %s yet", repo)

	payloads := lo.FlatMap(ids, func(id string, _ int) []*aggregator.SBOMPayload {
		p, err := s.Fakeintake.FilterSBOMs(id)
		assert.NoErrorf(c, err, "Failed to query fake intake")
		return p
	})
	payloads = lo.Filter(payloads, func(p *aggregator.SBOMPayload, _ int) bool {
		return p.GetType() == sbom.SBOMSourceType_CONTAINER_IMAGE_LAYERS &&
			p.Status == sbom.SBOMStatus_SUCCESS && p.GetCyclonedx() != nil
	})
	require.NotEmptyf(c, payloads, "No successful container SBOM for %s yet", repo)

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
// package's component from the most recently collected SBOM payload for the
// repo, or "" if absent.
func (s *packageInUseSuite) packageProperty(repo, pkg, name string) string {
	ids, err := s.Fakeintake.GetSBOMIDs()
	if err != nil {
		return ""
	}
	ids = lo.Filter(ids, func(id string, _ int) bool { return strings.Contains(id, repo+"@") })
	var value string
	var newest time.Time
	for _, id := range ids {
		payloads, err := s.Fakeintake.FilterSBOMs(id)
		if err != nil {
			continue
		}
		for _, p := range payloads {
			if p.GetType() != sbom.SBOMSourceType_CONTAINER_IMAGE_LAYERS || p.Status != sbom.SBOMStatus_SUCCESS || p.GetCyclonedx() == nil {
				continue
			}
			if comp := findComponent(p.GetCyclonedx().Components, pkg); comp != nil {
				if vals := propertyValues(comp.GetProperties(), name); len(vals) > 0 && !p.GetCollectedTime().Before(newest) {
					newest = p.GetCollectedTime()
					value = vals[len(vals)-1]
				}
			}
		}
	}
	return value
}

// imagePayloads summarises, for one container image, what the fake intake holds:
// the newest enriched payload, and the largest component count seen on a payload
// the enrichment had yet to touch.
type imagePayloads struct {
	id           string
	rootRef      string // bom-ref of the image itself, the root of the dependency graph
	components   []*cyclonedx_v1_4.Component
	dependencies []*cyclonedx_v1_4.Dependency
	rawCount     int
}

// danglingRefs returns the dependency refs of the payload that resolve to no
// component. The image itself roots the dependency graph and rides in the BOM
// metadata, so its ref resolves too, as do those of nested components.
func danglingRefs(img imagePayloads) []string {
	refs := make(map[string]struct{}, len(img.components)+1)
	if img.rootRef != "" {
		refs[img.rootRef] = struct{}{}
	}
	collectBomRefs(refs, img.components)

	var dangling []string
	for _, dep := range img.dependencies {
		if _, ok := refs[dep.GetRef()]; !ok {
			dangling = append(dangling, dep.GetRef())
		}
	}
	return dangling
}

func collectBomRefs(refs map[string]struct{}, comps []*cyclonedx_v1_4.Component) {
	for _, comp := range comps {
		refs[comp.GetBomRef()] = struct{}{}
		collectBomRefs(refs, comp.GetComponents())
	}
}

// runComponentListPreserved asserts the enrichment merge annotates the image SBOM
// and leaves its shape alone. The merge rebuilds the component list every round,
// so a component dropped there vanishes from the payload the backend sees while
// the dependency graph, carried over as it stands, goes on referencing it.
//
// The phase stands alone, which suits the leaf-test retry loop in
// tasks/new_e2e_tests.py: it reads whatever enriched payloads the fake intake
// holds, and the Agent's own containers and the control plane keep it supplied
// with the richest ones on the node.
func (s *packageInUseSuite) runComponentListPreserved() {
	s.EventuallyWithTf(func(collect *assert.CollectT) {
		c := &myCollectT{CollectT: collect, errors: []error{}}
		collect = nil //nolint:ineffassign

		images := s.imagePayloads(c)
		require.NotEmptyf(c, images, "no runtime-enriched container SBOM in fake intake")

		for _, img := range images {
			// Components sharing a name and version are what a merge keyed on those
			// two collapses, so log how many groups this image holds: that says how
			// much bite the assertions below have here.
			groups := map[string]int{}
			for _, comp := range img.components {
				groups[comp.GetName()+"@"+comp.GetVersion()]++
			}
			repeated := lo.CountBy(lo.Values(groups), func(n int) bool { return n > 1 })
			dangling := danglingRefs(img)
			s.T().Logf("PKG-IN-USE[shape] id=%q components=%d raw=%d dependencies=%d repeated_name_version=%d dangling_refs=%d",
				img.id, len(img.components), img.rawCount, len(img.dependencies), repeated, len(dangling))

			assert.GreaterOrEqualf(c, len(img.components), img.rawCount,
				"enriched %s has %d components, fewer than the %d of its un-enriched payload", img.id, len(img.components), img.rawCount)
			assert.Emptyf(c, dangling,
				"dependency refs of %s resolve to no component, the merge dropped them: %v", img.id, dangling)
		}
		// 15m: run on its own, the phase waits out the first enrichment, which
		// lands ~10-15m in, once the overlayfs Trivy SBOMs reach workloadmeta.
	}, 15*time.Minute, 15*time.Second, "the enrichment merge kept reshaping the component list")
}

// imagePayloads walks every container image payload in the fake intake and returns
// one entry per image whose SBOM has been enriched. isEnriched marks the payloads
// the merge has been through, which leaves the raw Trivy ones as the baseline to
// compare against.
func (s *packageInUseSuite) imagePayloads(c *myCollectT) []imagePayloads {
	ids, err := s.Fakeintake.GetSBOMIDs()
	require.NoErrorf(c, err, "Failed to query fake intake")

	var images []imagePayloads
	for _, id := range ids {
		payloads, err := s.Fakeintake.FilterSBOMs(id)
		if err != nil {
			continue
		}

		img := imagePayloads{id: id}
		var newest time.Time
		for _, p := range payloads {
			if p.GetType() != sbom.SBOMSourceType_CONTAINER_IMAGE_LAYERS || p.Status != sbom.SBOMStatus_SUCCESS || p.GetCyclonedx() == nil {
				continue
			}
			comps := p.GetCyclonedx().Components
			if !isEnriched(p.GetCyclonedx()) {
				img.rawCount = max(img.rawCount, len(comps))
				continue
			}
			if p.GetCollectedTime().Before(newest) {
				continue
			}
			newest = p.GetCollectedTime()
			img.components = comps
			img.dependencies = p.GetCyclonedx().Dependencies
			img.rootRef = p.GetCyclonedx().GetMetadata().GetComponent().GetBomRef()
		}

		if len(img.components) > 0 {
			images = append(images, img)
		}
	}
	return images
}

// repoPayloads counts the successful SBOM payloads of the images of repo sent
// in use, enriched with runtime usage and raw.
func (s *packageInUseSuite) repoPayloads(c require.TestingT, repo string) (enriched, raw int) {
	ids, err := s.Fakeintake.GetSBOMIDs()
	require.NoErrorf(c, err, "Failed to query fake intake")

	for _, id := range ids {
		if !strings.Contains(id, repo+"@") {
			continue
		}
		payloads, err := s.Fakeintake.FilterSBOMs(id)
		if err != nil {
			continue
		}
		for _, p := range payloads {
			if p.GetType() != sbom.SBOMSourceType_CONTAINER_IMAGE_LAYERS || p.Status != sbom.SBOMStatus_SUCCESS || p.GetCyclonedx() == nil || !p.GetInUse() {
				continue
			}
			if isEnriched(p.GetCyclonedx()) {
				enriched++
			} else {
				raw++
			}
		}
	}
	return enriched, raw
}

// startInUseService launches, inside the workload pod, a detached loop that
// repeatedly executes the in-use binary so the package is continuously seen
// running. The loop's pid is recorded so stopInUseService can stop it.
func (s *packageInUseSuite) startInUseService(d pkgInUseDistro) {
	// Run the binary every 15s (> the 10s enrichment interval) so each execution
	// re-arms the resolver's forwarding debouncer: a tighter loop only forwards
	// once (the resolver suppresses re-forwards within the enrichment interval),
	// which is fragile if that single forward races the image SBOM becoming ready.
	script := fmt.Sprintf(`nohup sh -c 'echo $$ > /tmp/inuse.pid; while true; do %s --version >/dev/null 2>&1; sleep 15; done' </dev/null >/dev/null 2>&1 &`, d.inUseBin)
	stdout, stderr := s.podExec(d, "sh", "-c", script)
	s.T().Logf("PKG-IN-USE[%s] start service: stdout=%q stderr=%q", d.name, stdout, stderr)
}

// stopInUseService stops the in-use loop started by startInUseService.
func (s *packageInUseSuite) stopInUseService(d pkgInUseDistro) {
	stdout, stderr := s.podExec(d, "sh", "-c", `kill "$(cat /tmp/inuse.pid)" 2>/dev/null; rm -f /tmp/inuse.pid; echo stopped`)
	s.T().Logf("PKG-IN-USE[%s] stop service: stdout=%q stderr=%q", d.name, stdout, stderr)
}

// startSecurityProbes launches, inside the workload pod, a detached loop that
// exercises the security properties the in-use phases leave uncovered:
//   - suidCmd execs the distro's setuid-root binary as root, so the resolver
//     records HasSetSuidBit on suidPkg;
//   - stickyCmd, when set, execs a NON-setuid binary of the SAME suidPkg, so a
//     correct (sticky) resolver keeps HasSetSuidBit true rather than clearing it;
//   - nonRootCmd runs a nonRootPkg binary as the unprivileged nobody user, so its
//     RunningAsRoot stays false.
func (s *packageInUseSuite) startSecurityProbes(d pkgInUseDistro) {
	body := d.suidCmd + "; "
	if d.stickyCmd != "" {
		body += d.stickyCmd + "; "
	}
	body += d.nonRootCmd + "; "
	script := `nohup sh -c 'echo $$ > /tmp/secprobe.pid; while true; do ` + body + `sleep 15; done' </dev/null >/dev/null 2>&1 &`
	stdout, stderr := s.podExec(d, "sh", "-c", script)
	s.T().Logf("PKG-IN-USE[%s] start security probes: stdout=%q stderr=%q", d.name, stdout, stderr)
}

// stopSecurityProbes stops the loop started by startSecurityProbes.
func (s *packageInUseSuite) stopSecurityProbes(d pkgInUseDistro) {
	stdout, stderr := s.podExec(d, "sh", "-c", `kill "$(cat /tmp/secprobe.pid)" 2>/dev/null; rm -f /tmp/secprobe.pid; echo stopped`)
	s.T().Logf("PKG-IN-USE[%s] stop security probes: stdout=%q stderr=%q", d.name, stdout, stderr)
}

// triggerSBOMRefresh writes the package database so the bundled need_refresh_sbom
// / refresh_sbom rules fire and the workload is re-scanned. The rules match a
// write to an existing file under the package DB dir but not the O_CREAT of a
// brand-new file (its path is not resolved at the open probe), so the probe file
// is created first and then written: the second open, on the now-existing path,
// is what fires the rule.
func (s *packageInUseSuite) triggerSBOMRefresh(d pkgInUseDistro) {
	s.podExec(d, "touch", d.dbProbe)
	stdout, stderr := s.podExec(d, "sh", "-c", "echo probe >> "+d.dbProbe)
	s.T().Logf("PKG-IN-USE[%s] refresh trigger: stdout=%q stderr=%q", d.name, stdout, stderr)
}

// nodeEpoch returns the workload node's wall clock (Unix seconds), read from the
// pod so it shares the clock that stamps LastSeenRunning.
func (s *packageInUseSuite) nodeEpoch(d pkgInUseDistro) int64 {
	stdout, _ := s.podExec(d, "date", "+%s")
	n, _ := strconv.ParseInt(strings.TrimSpace(stdout), 10, 64)
	return n
}

// podExec runs cmd in the given workload pod's container and returns stdout/stderr.
func (s *packageInUseSuite) podExec(d pkgInUseDistro, cmd ...string) (string, string) {
	pods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(sbomtargets.Namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", d.workload).String(),
	})
	require.NoErrorf(s.T(), err, "failed to list %s workload pods", d.workload)
	require.NotEmptyf(s.T(), pods.Items, "no %s workload pod found in namespace %s", d.workload, sbomtargets.Namespace)

	stdout, stderr, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(sbomtargets.Namespace, pods.Items[0].Name, "main", cmd)
	require.NoErrorf(s.T(), err, "pod exec failed: %s", stderr)
	return stdout, stderr
}

// TestZZDumpAgentDiagnostics runs last (the ZZ prefix sorts it after the
// package-in-use test) and dumps the Agent's SBOM/CWS state - the SBOM status
// section and the system-probe/security logs - which otherwise live only in the
// flare artifact, not the test trace. It is a debugging aid and always passes.
func (s *packageInUseSuite) TestZZDumpAgentDiagnostics() {
	pods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(context.Background(), metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", s.Env().Agent.LinuxNodeAgent.LabelSelectors["app"]).String(),
	})
	if err != nil || len(pods.Items) == 0 {
		s.T().Logf("DIAG: could not list agent pods: %v", err)
		return
	}
	pod := pods.Items[0].Name

	// Best-effort probes confirming the enrichment is wired up: the usage flag on
	// the agent, the resolved runtime_security_config, and the shared
	// runtime-security command socket the core agent's collector connects to.
	for _, step := range []struct {
		label string
		cmd   []string
	}{
		{"agent-env", []string{"sh", "-c", "env | grep -iE 'DD_SBOM|DD_RUNTIME_SECURITY' | sort"}},
		{"agent-config", []string{"sh", "-c", "agent config 2>/dev/null | grep -iE 'enrichment|runtime_security_config' | head -40"}},
		{"agent-sockets", []string{"sh", "-c", "ls -la /var/run/sysprobe/ 2>&1"}},
	} {
		stdout, stderr, err := s.Env().KubernetesCluster.KubernetesClient.PodExec("datadog", pod, "agent", step.cmd)
		s.T().Logf("DIAG[%s] err=%v\n%s\n%s", step.label, err, stdout, stderr)
	}
}

// keepActive starts, inside the workload pod, a detached loop that continuously
// accesses a non-in-use file. An idle container (its entrypoint is
// `tail -f /dev/null`) forwards its runtime SBOM only a handful of times and can
// miss the window once its image SBOM is available; keeping it active makes the
// resolver re-forward steadily, the way the always-busy Agent containers do. The
// in-use binary is never touched here, so it stays not-in-use until the in-use
// phase. `cat` is owned by the distro's controlPkg (the positive control).
func (s *packageInUseSuite) keepActive(d pkgInUseDistro) {
	script := `nohup sh -c 'while true; do cat /etc/os-release >/dev/null 2>&1; sleep 12; done' </dev/null >/dev/null 2>&1 &`
	stdout, stderr := s.podExec(d, "sh", "-c", script)
	s.T().Logf("PKG-IN-USE[%s] keepalive: stdout=%q stderr=%q", d.name, stdout, stderr)
}

// pkgInUseInventoryOnce guards dumpSBOMInventory so the inventory is logged at
// most once even though it is called from a retry loop.
var pkgInUseInventoryOnce sync.Once

// dumpSBOMInventory logs every SBOM id with its type and status once, to diagnose
// a missing payload (e.g. the image was never scanned or never enriched).
func (s *packageInUseSuite) dumpSBOMInventory() {
	pkgInUseInventoryOnce.Do(func() {
		ids, err := s.Fakeintake.GetSBOMIDs()
		if err != nil {
			s.T().Logf("PKG-IN-USE inventory: GetSBOMIDs error: %v", err)
			return
		}
		for _, id := range ids {
			ps, err := s.Fakeintake.FilterSBOMs(id)
			if err != nil {
				continue
			}
			for _, p := range ps {
				s.T().Logf("PKG-IN-USE inventory id=%q type=%v status=%v", id, p.GetType(), p.Status)
			}
		}
	})
}

// lastSeenRunning parses the LastSeenRunning property of a component into a Unix
// timestamp. The second return is false when the component carries no such
// property (i.e. the runtime enrichment has not been merged onto it yet).
func lastSeenRunning(comp *cyclonedx_v1_4.Component) (int64, bool) {
	vals := propertyValues(comp.GetProperties(), propLastSeenRunning)
	if len(vals) == 0 {
		return 0, false
	}
	var maxTS int64
	for _, v := range vals {
		if ts, err := strconv.ParseInt(v, 10, 64); err == nil && ts > maxTS {
			maxTS = ts
		}
	}
	return maxTS, true
}
