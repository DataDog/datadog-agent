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
	provkubeadm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kubeadm"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
// (overlayfs direct scan, os+languages analyzers) with everything needed for the
// "package in use" enrichment on a containerd kubeadm node:
//   - the system-probe security module + SBOM resolver
//     (DD_RUNTIME_SECURITY_CONFIG_SBOM_ENABLED) that tracks which packages a
//     running process accesses, and
//   - the core-agent enrichment collector (DD_SBOM_ENRICHMENT_USAGE_ENABLED) that
//     merges those runtime properties onto the Trivy container-image SBOM.
//
// The enrichment/forward intervals are shortened so a package's in-use timestamp
// surfaces within the test window instead of the 1m default.
func packageInUseHelmValues() string {
	return `datadog:
  criSocketPath: /run/containerd/containerd.sock
  kubelet:
    tlsVerify: false
  useHostPID: true
  securityAgent:
    runtime:
      enabled: true
  sbom:
    containerImage:
      enabled: true
      uncompressedLayersSupport: true
      overlayFSDirectScan: true
      analyzers: ["os", "languages"]
agents:
  useHostNetwork: true
  containers:
    agent:
      env:
        - name: DD_SBOM_ENRICHMENT_USAGE_ENABLED
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
        - name: DD_RUNTIME_SECURITY_CONFIG_SBOM_ENRICHMENT_TICKER
          value: "10s"
        # forward_interval x maxRetryForwarding(10) is the window the resolver
        # waits for the image's Trivy SBOM to be available before giving up
        # forwarding for good. Keep it wide enough to outlast the initial
        # overlayfs Trivy scans (a 5s interval gave up after ~50s, before the
        # container SBOMs were ready).
        - name: DD_RUNTIME_SECURITY_CONFIG_SBOM_FORWARD_INTERVAL
          value: "30s"
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

type packageInUseSuite struct {
	baseSuite[environments.Kubernetes]
}

// TestSBOMPackageInUseKubeadmSuite provisions the same RHEL 10 single-node
// kubeadm cluster as TestSBOMKubeadmSuite, but additionally enables the CWS SBOM
// resolver and the core-agent usage enrichment, then verifies the "package in
// use" feature end to end across package formats - ubi9 (rpm), ubuntu (dpkg) and
// alpine (apk): a package goes from not in use, to in use once a service runs
// its binary, and back to stale once the service stops.
func TestSBOMPackageInUseKubeadmSuite(t *testing.T) {
	prov := provkubeadm.Provisioner(
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
			),
		),
	)
	e2e.Run(t, &packageInUseSuite{}, e2e.WithProvisioner(prov))
}

func (s *packageInUseSuite) SetupSuite() {
	s.baseSuite.SetupSuite()
	s.clusterName = s.Env().KubernetesCluster.ClusterName
	s.Fakeintake = s.Env().FakeIntake.Client()
}

// Test00UpAndRunning waits (the 00 prefix runs it first) for the Agent DaemonSet
// pods - including the security-agent and system-probe containers enabled here -
// to be ready before the package-in-use assertions run.
func (s *packageInUseSuite) Test00UpAndRunning() {
	ctx := context.Background()
	s.EventuallyWithTf(func(c *assert.CollectT) {
		nodes, err := s.Env().KubernetesCluster.Client().CoreV1().Nodes().List(ctx, metav1.ListOptions{
			LabelSelector: fields.OneTermEqualSelector("kubernetes.io/os", "linux").String(),
		})
		require.NoErrorf(c, err, "Failed to list Linux nodes")

		pods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
			LabelSelector: fields.OneTermEqualSelector("app", s.Env().Agent.LinuxNodeAgent.LabelSelectors["app"]).String(),
		})
		require.NoErrorf(c, err, "Failed to list Linux datadog agent pods")

		assert.Len(c, pods.Items, len(nodes.Items))
		for _, pod := range pods.Items {
			for _, cs := range append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...) {
				assert.Truef(c, cs.Ready, "Container %s of pod %s isn't ready", cs.Name, pod.Name)
				assert.Zerof(c, cs.RestartCount, "Container %s of pod %s has restarted", cs.Name, pod.Name)
			}
		}
	}, 10*time.Minute, 10*time.Second, "Not all agents eventually became ready in time.")
}

// TestPackageInUse drives the full not-in-use -> in-use -> stale -> security ->
// refresh cycle for each workload (rpm, dpkg, apk) as a nested subtest.
func (s *packageInUseSuite) TestPackageInUse() {
	for _, d := range pkgInUseDistros {
		s.Run(d.name, func() {
			s.runPackageInUse(d)
		})
	}
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
	// a service merely freezes the timestamp. The in-use package is no longer
	// running, so after the refresh its newest payload must report "0".
	s.Run("refresh-reset", func() {
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
	})
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
