// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package sbom

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	"github.com/DataDog/agent-payload/v5/sbom"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	scenkubeadm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kubeadm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	provkubeadm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kubeadm"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"

	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
)

const (
	propDiffID      = "aquasecurity:trivy:DiffID"
	propLayerDiffID = "aquasecurity:trivy:LayerDiffID"
	propLayerDigest = "aquasecurity:trivy:LayerDigest"
	propImageID     = "aquasecurity:trivy:ImageID"
	propRepoDigest  = "aquasecurity:trivy:RepoDigest"
	propRepoTag     = "aquasecurity:trivy:RepoTag"
)

// kubeadmSBOMHelmValues builds the Agent Helm values for the SBOM suite on the
// given container runtime. It is merged on top of the framework defaults (which
// already enable host + container SBOM with a fast 60s refresh) and enables the
// os and languages analyzers on both host and container images, so language
// packages (npm/apk/pypi) are catalogued alongside OS packages. Host language
// scanning stays within the test window because the trivy fork defers per-file
// analyzers past OS package detection, skipping files owned by an OS package.
//
// The only runtime differences are the CRI socket and the container-storage
// overlay mount; everything else (and the resulting SBOM) is identical, which is
// exactly what the shared assertions verify.
func kubeadmSBOMHelmValues(runtime kubeComp.ContainerRuntime) string {
	criSocket := "/run/containerd/containerd.sock"
	storagePath := "/var/lib/containerd"
	// containerd serves layers from its (compressed) content store, so it needs
	// the uncompress toggle; CRI-O reads uncompressed diff dirs directly.
	uncompressed := "\n      uncompressedLayersSupport: true"
	if runtime == kubeComp.CRIO {
		criSocket = "/var/run/crio/crio.sock"
		storagePath = "/var/lib/containers/storage"
		uncompressed = ""
	}
	return fmt.Sprintf(`datadog:
  criSocketPath: %[1]s
  kubelet:
    tlsVerify: false
  useHostPID: true
  sbom:
    host:
      enabled: true
      analyzers: ["os", "languages"]
    containerImage:
      enabled: true%[3]s
      overlayFSDirectScan: true
      analyzers: ["os", "languages"]
agents:
  useHostNetwork: true
  volumeMounts:
    - name: trivycache
      mountPath: /root/.cache/trivy
    - name: imageoverlay
      mountPath: %[2]s
      readOnly: true
  volumes:
    - name: trivycache
      emptyDir: {}
    - name: imageoverlay
      hostPath:
        path: %[2]s
`, criSocket, storagePath, uncompressed)
}

type kubeadmSuite struct {
	sbomTargetsSuite[environments.Kubernetes]
}

// TestSBOMKubeadmSuite provisions a RHEL 10 host running single-node Kubernetes
// (kubeadm + containerd), installs the Agent (CI dev image) via Helm, runs four
// target images, and asserts both host and container-image SBOMs in fakeintake.
func TestSBOMKubeadmSuite(t *testing.T) {
	prov := provkubeadm.Provisioner(
		provkubeadm.WithRunOptions(
			scenkubeadm.WithVMOptions(
				scenec2.WithOS(e2eos.RedHat10),
				scenec2.WithInstanceType("t3.2xlarge"),
			),
			scenkubeadm.WithFakeintakeOptions(fakeintake.WithMemory(2048)),
			scenkubeadm.WithDeploySBOMWorkloads(),
			scenkubeadm.WithAgentOptions(
				kubernetesagentparams.WithDualShipping(),
				kubernetesagentparams.WithTimeout(900),
				kubernetesagentparams.WithHelmValues(kubeadmSBOMHelmValues(kubeComp.Containerd)),
			),
		),
	)
	e2e.Run(t, &kubeadmSuite{sbomTargetsSuite[environments.Kubernetes]{expectLayerDigests: true}}, e2e.WithProvisioner(prov))
}

// TestSBOMKubeadmCrioSuite is the CRI-O counterpart of TestSBOMKubeadmSuite: the
// same RHEL 10 single-node kubeadm cluster and the same SBOM assertions, but with
// CRI-O as the container runtime. Reusing the assertions verifies the Agent emits
// identical host + container SBOMs regardless of the runtime.
func TestSBOMKubeadmCrioSuite(t *testing.T) {
	prov := provkubeadm.Provisioner(
		provkubeadm.WithRunOptions(
			scenkubeadm.WithVMOptions(
				scenec2.WithOS(e2eos.RedHat10),
				scenec2.WithInstanceType("t3.2xlarge"),
			),
			scenkubeadm.WithContainerRuntime(kubeComp.CRIO),
			scenkubeadm.WithFakeintakeOptions(fakeintake.WithMemory(2048)),
			scenkubeadm.WithDeploySBOMWorkloads(),
			scenkubeadm.WithAgentOptions(
				kubernetesagentparams.WithDualShipping(),
				kubernetesagentparams.WithTimeout(900),
				kubernetesagentparams.WithHelmValues(kubeadmSBOMHelmValues(kubeComp.CRIO)),
			),
		),
	)
	e2e.Run(t, &kubeadmSuite{sbomTargetsSuite[environments.Kubernetes]{expectLayerDigests: true}}, e2e.WithProvisioner(prov))
}

func (s *kubeadmSuite) SetupSuite() {
	s.baseSuite.SetupSuite()
	s.clusterName = s.Env().KubernetesCluster.ClusterName
	s.Fakeintake = s.Env().FakeIntake.Client()
}

// Test00UpAndRunning waits (with a long timeout, hence the 00 prefix so it runs
// first) for the Agent DaemonSet to be ready before the SBOM assertions run.
func (s *kubeadmSuite) Test00UpAndRunning() {
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

// expectedComponent describes a meaningful SBOM component to assert on. When name
// is set, a component with that exact name must exist; when only purlPrefix is set,
// any component whose PURL starts with the prefix satisfies the check.
type expectedComponent struct {
	name       string
	purlPrefix string
	// version, when set, is asserted exactly (the complete version string).
	version string
	// layer requires this component to carry a LayerDiffID that is a real diff_id
	// of the image, plus a well formed LayerDigest (the OCI manifest digest).
	layer bool
}

// containerTarget mirrors a sbomtargets workload and lists ~3 meaningful
// components to verify, spanning OS and application packages. digest is the
// expected manifest digest, asserted against the SBOM RepoDigest. The workload
// is pinned by digest, so the runtime records no RepoTag (image_tag is absent).
type containerTarget struct {
	short string
	repo  string
	// tag documents the version the digest pins; it is surfaced only when a
	// runtime records a RepoTag, so it is verified opportunistically.
	tag    string
	digest string
	// componentCount is the exact total component count (OS + language) for the
	// digest-pinned image; it is deterministic (identical across containerd and
	// crio), so the test asserts it exactly.
	componentCount int
	components     []expectedComponent
}

var containerTargets = []containerTarget{
	{
		short: "node", repo: "node", tag: "26.2.0",
		digest:         "sha256:980c5420a7a2ddcb44037726977f2a349e5c7b64217516c7488dce4c74d71583",
		componentCount: 602,
		components: []expectedComponent{
			{name: "node", version: "26.2.0"},                        // generic runtime
			{name: "libc6", version: "2.41-12+deb13u3", layer: true}, // Debian glibc (OS)
			// 3 npm application packages (bundled under npm's node_modules), layer-attributed.
			{name: "chalk", version: "5.6.2", purlPrefix: "pkg:npm/", layer: true},
			{name: "abbrev", version: "4.0.0", purlPrefix: "pkg:npm/", layer: true},
			{name: "cacache", version: "20.0.4", purlPrefix: "pkg:npm/", layer: true},
		},
	},
	{
		short: "golang", repo: "golang", tag: "1.26.3-alpine",
		digest:         "sha256:91eda9776261207ea25fd06b5b7fed8d397dd2c0a283e77f2ab6e91bfa71079d",
		componentCount: 186,
		components: []expectedComponent{
			{name: "alpine", version: "3.23.4"},               // OS component
			{name: "musl", version: "1.2.5-r23", layer: true}, // apk libc (OS)
			{purlPrefix: "pkg:apk/"},                          // apk package
			// The Go toolchain binaries are built from stdlib only (no third-party
			// modules); gobinary reports a single application package, layer-attributed.
			{name: "stdlib", version: "v1.26.3", purlPrefix: "pkg:golang/", layer: true},
		},
	},
	{
		// ubi is a single-layer image: exercises the single-layer overlayfs scan
		// (a single containerd bind mount, no lowerdir/upperdir).
		short: "ubi", repo: "registry.access.redhat.com/ubi9/ubi", tag: "9.8-1780376557",
		digest:         "sha256:80b1f4c34a7eed1b03a05d12b55768f3e522eef6ec294c6fbd5fa47b6b2892ee",
		componentCount: 191,
		components: []expectedComponent{
			{name: "glibc", layer: true},    // rpm (OS)
			{name: "bash"},                  // rpm (OS)
			{purlPrefix: "pkg:rpm/redhat/"}, // rpm package
		},
	},
	{
		// ubi9/python-312 is a multi-layer RHEL image: rpm (OS) + pip (pypi) across layers.
		short: "python-312", repo: "registry.access.redhat.com/ubi9/python-312", tag: "9.8-1779945122",
		digest:         "sha256:52d1ffcda3b9552934f947b7d41fb0cb66973bdc0d7e91814facadc126f68663",
		componentCount: 479,
		components: []expectedComponent{
			{name: "glibc", layer: true},    // rpm glibc (OS)
			{purlPrefix: "pkg:rpm/redhat/"}, // rpm package
			// pip is the only pypi package the fix keeps: the system python under
			// /usr/lib is rpm-owned (skipped); the s2i pip under /opt/app-root survives.
			{name: "pip", version: "24.2", purlPrefix: "pkg:pypi/", layer: true},
		},
	},
	{
		short: "python", repo: "python", tag: "3.14.5",
		digest:         "sha256:250e5c97be05e1eb2272fbdbd810dfd638f9012e1e6f65c99390ad3239943a08",
		componentCount: 473,
		components: []expectedComponent{
			{name: "python", version: "3.14.5"},                                    // generic runtime
			{name: "libc6", version: "2.41-12+deb13u3", layer: true},               // Debian glibc (OS)
			{name: "pip", version: "26.1.1", purlPrefix: "pkg:pypi/", layer: true}, // pypi application package
		},
	},
	{
		// ruby:3.3.4-bookworm is a Debian image whose application packages are
		// rubygems; the count and gem versions are calibrated from the first run.
		short: "ruby", repo: "ruby", tag: "3.3.4-bookworm",
		digest:         "sha256:d4233f4242ea25346f157709bb8417c615e7478468e2699c8e86a4e1f0156de8",
		componentCount: 557,
		components: []expectedComponent{
			{name: "libc6", layer: true}, // Debian glibc (OS)
			{purlPrefix: "pkg:gem/"},     // rubygems application package
		},
	},
}

var (
	inventoryOnce sync.Once
	dumpedTargets sync.Map
	rawDumped     sync.Map
)

// dumpRawOnce logs, once per image, the raw SBOM payloads (type/status/error)
// when none passed the success+container+metadata filter - to diagnose why an
// image's SBOM is missing (e.g. status FAILED with an error, or a heartbeat).
func (s *sbomTargetsSuite[Env]) dumpRawOnce(short string, payloads []*aggregator.SBOMPayload) {
	if _, loaded := rawDumped.LoadOrStore(short, true); loaded {
		return
	}
	for _, p := range payloads {
		bom := p.GetCyclonedx()
		s.T().Logf("SBOM-RAW[%s] id=%q type=%v status=%v err=%q hasCyclonedx=%v hasMetadataComponent=%v",
			short, p.GetId(), p.GetType(), p.Status, p.GetError(), bom != nil,
			bom != nil && bom.Metadata != nil && bom.Metadata.GetComponent() != nil)
	}
}

// TestContainerSBOM verifies each target image's SBOM: tags, meaningful
// components (exact version + real layer attribution), and that the image
// metadata (DiffID list, ImageID, RepoDigest, RepoTag) matches the real image.
func (s *sbomTargetsSuite[Env]) TestContainerSBOM() {
	for _, target := range containerTargets {
		s.Run("image="+target.short, func() {
			s.EventuallyWithTf(func(collect *assert.CollectT) {
				c := &myCollectT{CollectT: collect, errors: []error{}}
				collect = nil //nolint:ineffassign

				sbomIDs, err := s.Fakeintake.GetSBOMIDs()
				require.NoErrorf(c, err, "Failed to query fake intake")

				ids := lo.Filter(sbomIDs, func(id string, _ int) bool {
					// Container SBOM ids are "<repo>@<digest>"; the trailing "@"
					// anchors the repo so e.g. "python" does not match the
					// "python-312" target's id.
					return strings.Contains(id, target.repo+"@")
				})
				if len(ids) == 0 {
					s.logSBOMInventory()
				}
				require.NotEmptyf(c, ids, "No SBOM for %s yet", target.short)

				rawPayloads := lo.FlatMap(ids, func(id string, _ int) []*aggregator.SBOMPayload {
					p, err := s.Fakeintake.FilterSBOMs(id)
					assert.NoErrorf(c, err, "Failed to query fake intake")
					return p
				})
				payloads := lo.Filter(rawPayloads, func(p *aggregator.SBOMPayload, _ int) bool {
					bom := p.GetCyclonedx()
					return p.GetType() == sbom.SBOMSourceType_CONTAINER_IMAGE_LAYERS &&
						p.Status == sbom.SBOMStatus_SUCCESS &&
						bom != nil && bom.Metadata != nil && bom.Metadata.GetComponent() != nil
				})
				if len(payloads) == 0 {
					s.dumpRawOnce(target.short, rawPayloads)
				}
				require.NotEmptyf(c, payloads, "No successful container SBOM for %s yet", target.short)

				// Ground truth from the real image config and manifest for the
				// expected digest.
				realDiffIDs, realDigests, realImageID, err := realImageRootFS(target.repo + "@" + target.digest)
				if !assert.NoErrorf(c, err, "failed to fetch real image config for %s", target.short) {
					return
				}
				diffIDSet := make(map[string]bool, len(realDiffIDs))
				for _, d := range realDiffIDs {
					diffIDSet[d] = true
				}
				digestSet := make(map[string]bool, len(realDigests))
				for _, d := range realDigests {
					digestSet[d] = true
				}

				for _, p := range payloads {
					bom := p.GetCyclonedx()
					metaProps := bom.Metadata.GetComponent().GetProperties()
					comps := bom.Components
					metaDiffIDs := propertyValues(metaProps, propDiffID)

					s.dumpImageOnce(target, metaProps, comps, metaDiffIDs)

					expectedTags := []*regexp.Regexp{
						regexp.MustCompile(`^architecture:(amd|arm)64$`),
						regexp.MustCompile(`^image_id:.*` + regexp.QuoteMeta(target.repo) + `.*@sha256:`),
						regexp.MustCompile(`^image_name:.*` + regexp.QuoteMeta(target.repo)),
						regexp.MustCompile(`^os_name:linux$`),
						regexp.MustCompile(`^short_image:` + regexp.QuoteMeta(target.short) + `$`),
						regexp.MustCompile(`^scan_method:overlayfs$`),
					}
					// Digest-pinned workloads carry no RepoTag, so image_tag is absent;
					// verify it only when a runtime happens to surface it.
					optionalTags := []*regexp.Regexp{
						regexp.MustCompile(`^image_tag:` + regexp.QuoteMeta(target.tag) + `$`),
					}
					assert.NoErrorf(c, assertTags(p.GetTags(), expectedTags, optionalTags, true), "Tags mismatch for %s", target.short)

					// Metadata must match the real image config exactly.
					assert.Equalf(c, realDiffIDs, metaDiffIDs, "DiffID list does not match the real image config for %s", target.short)
					assert.Containsf(c, propertyValues(metaProps, propImageID), realImageID, "ImageID does not match the real image config for %s", target.short)
					if repoDigests := propertyValues(metaProps, propRepoDigest); assert.NotEmptyf(c, repoDigests, "no RepoDigest for %s", target.short) {
						// A runtime may surface several RepoDigests (crio adds the platform
						// manifest digest next to the pulled index digest); the pinned digest
						// must appear in one of them, regardless of order.
						assert.Truef(c, lo.ContainsBy(repoDigests, func(rd string) bool {
							return strings.Contains(rd, target.digest)
						}), "no RepoDigest references the expected digest %s for %s (got %v)", target.digest, target.short, repoDigests)
					}
					// Digest-pinned images carry no RepoTag; assert its content only when present.
					if repoTags := propertyValues(metaProps, propRepoTag); len(repoTags) > 0 {
						assert.Truef(c, strings.Contains(repoTags[0], target.repo+":"+target.tag), "RepoTag %q does not reference %s:%s", repoTags[0], target.repo, target.tag)
					}

					// Total component count is exact for the digest-pinned image (verified
					// identical across containerd and crio); versions and layer attribution
					// are checked below.
					assert.Equalf(c, target.componentCount, len(comps), "%s has %d components, expected exactly %d", target.short, len(comps), target.componentCount)
					for _, ec := range target.components {
						if ec.name == "" {
							if ec.purlPrefix != "" {
								assert.Truef(c, hasComponentWithPurlPrefix(comps, ec.purlPrefix), "no component with purl prefix %q in %s", ec.purlPrefix, target.short)
							}
							continue
						}
						comp := findComponent(comps, ec.name)
						if !assert.NotNilf(c, comp, "missing component %q in %s", ec.name, target.short) {
							continue
						}
						if ec.version != "" {
							assert.Equalf(c, ec.version, comp.GetVersion(), "component %q version mismatch in %s", ec.name, target.short)
						} else {
							assert.NotEmptyf(c, comp.GetVersion(), "component %q has no version in %s", ec.name, target.short)
						}
						if ec.purlPrefix != "" {
							assert.Truef(c, strings.HasPrefix(comp.GetPurl(), ec.purlPrefix), "component %q purl %q lacks prefix %q in %s", ec.name, comp.GetPurl(), ec.purlPrefix, target.short)
						}
						if ec.layer {
							assertComponentLayer(c, diffIDSet, digestSet, comp, target.short, s.expectLayerDigests)
						}
					}

					// Every component attributed to a layer must reference real diff_ids.
					layered := 0
					for _, comp := range comps {
						if len(propertyValues(comp.GetProperties(), propLayerDiffID)) == 0 &&
							len(propertyValues(comp.GetProperties(), propLayerDigest)) == 0 {
							continue
						}
						layered++
						assertComponentLayer(c, diffIDSet, digestSet, comp, target.short, s.expectLayerDigests)
					}
					assert.NotZerof(c, layered, "no component carried layer (DiffID/Digest) attribution in %s", target.short)
				}
			}, 8*time.Minute, 15*time.Second, "Failed finding/validating container SBOM for %s", target.short)
		})
	}
}

// dumpImageOnce logs the Agent's authoritative metadata + meaningful-component
// values for an image exactly once, so exact versions/layers can be read from
// the CI trace without flooding it.
func (s *sbomTargetsSuite[Env]) dumpImageOnce(target containerTarget, metaProps []*cyclonedx_v1_4.Property, comps []*cyclonedx_v1_4.Component, metaDiffIDs []string) {
	if _, loaded := dumpedTargets.LoadOrStore(target.short, true); loaded {
		return
	}
	s.T().Logf("SBOM-DUMP[%s] ImageID=%v RepoDigest=%v RepoTag=%v DiffIDs=%v",
		target.short, propertyValues(metaProps, propImageID), propertyValues(metaProps, propRepoDigest),
		propertyValues(metaProps, propRepoTag), metaDiffIDs)
	s.T().Logf("SBOM-COUNT[%s] components=%d application=%d", target.short, len(comps), len(applicationComponents(comps)))
	for _, ec := range target.components {
		if ec.name == "" {
			continue
		}
		if comp := findComponent(comps, ec.name); comp != nil {
			s.T().Logf("SBOM-DUMP[%s] comp=%q version=%q purl=%q LayerDiffID=%v LayerDigest=%v",
				target.short, comp.GetName(), comp.GetVersion(), comp.GetPurl(),
				propertyValues(comp.GetProperties(), propLayerDiffID), propertyValues(comp.GetProperties(), propLayerDigest))
		}
	}
}

// logSBOMInventory logs every SBOM id with its type and status exactly once -
// used to diagnose missing payloads (e.g. an image that was not scanned).
func (s *sbomTargetsSuite[Env]) logSBOMInventory() {
	inventoryOnce.Do(func() {
		ids, err := s.Fakeintake.GetSBOMIDs()
		if err != nil {
			s.T().Logf("SBOM-INV: GetSBOMIDs error: %v", err)
			return
		}
		for _, id := range ids {
			ps, err := s.Fakeintake.FilterSBOMs(id)
			if err != nil {
				continue
			}
			for _, p := range ps {
				s.T().Logf("SBOM-INV id=%q type=%v status=%v", id, p.GetType(), p.Status)
			}
		}
	})
}

// TestHostSBOM verifies the host (RHEL 10) SBOM: it is the OS itself (not an
// image), lists meaningful rpm packages, and carries no layer/DiffID metadata.
func (s *sbomTargetsSuite[Env]) TestHostSBOM() {
	s.EventuallyWithTf(func(collect *assert.CollectT) {
		c := &myCollectT{CollectT: collect, errors: []error{}}
		collect = nil //nolint:ineffassign

		sbomIDs, err := s.Fakeintake.GetSBOMIDs()
		require.NoErrorf(c, err, "Failed to query fake intake")

		payloads := lo.FlatMap(sbomIDs, func(id string, _ int) []*aggregator.SBOMPayload {
			p, err := s.Fakeintake.FilterSBOMs(id)
			assert.NoErrorf(c, err, "Failed to query fake intake")
			return p
		})
		hosts := lo.Filter(payloads, func(p *aggregator.SBOMPayload, _ int) bool {
			// Filter to a successful host SBOM that carries a CycloneDX body.
			// fakeintake retains earlier non-success scans and success heartbeats
			// (which have no CycloneDX), so requiring every retained host payload
			// to be valid would never converge once a stale one is present.
			return p.GetType() == sbom.SBOMSourceType_HOST_FILE_SYSTEM &&
				p.Status == sbom.SBOMStatus_SUCCESS &&
				p.GetCyclonedx() != nil
		})
		if len(hosts) == 0 {
			s.logSBOMInventory()
		}
		require.NotEmptyf(c, hosts, "No successful host SBOM yet")

		for _, p := range hosts {
			bom := p.GetCyclonedx()
			comps := bom.Components
			assert.Greaterf(c, len(comps), 300, "host SBOM has %d components, expected more than 300", len(comps))

			osComp := findOSComponent(comps)
			if osComp == nil {
				osComp = findComponent(comps, "redhat")
			}
			if assert.NotNilf(c, osComp, "no OS component in host SBOM") {
				assert.Truef(c, strings.HasPrefix(osComp.GetVersion(), "10"), "host OS version %q is not RHEL 10", osComp.GetVersion())
			}

			// Presence by name + ecosystem only: rpm package versions drift with
			// RHEL updates, so the version is not asserted.
			for _, name := range []string{"glibc", "bash", "systemd"} {
				comp := findComponent(comps, name)
				if assert.NotNilf(c, comp, "missing host rpm package %q", name) {
					assert.Truef(c, strings.HasPrefix(comp.GetPurl(), "pkg:rpm/redhat/"), "host rpm package %q has unexpected purl %q", name, comp.GetPurl())
				}
			}

			// languages is enabled on the host too, but a minimal RHEL host has no
			// language packages to surface: its Go binaries (kubelet, containerd, ...)
			// are owned by rpm packages, so the deferred analyzers skip them, and the
			// flannel CNI plugin only appears as a container SBOM. The value here is
			// that the scan completes (SUCCESS above) without timing out, which the
			// trivy deferral fix makes possible. Log the count; don't assert a floor.
			s.T().Logf("SBOM-COUNT[host] components=%d application=%d", len(comps), len(applicationComponents(comps)))

			// A host SBOM must not look like a container image.
			assert.Emptyf(c, p.GetDdTags(), "host SBOM should carry no image dd tags")
			assert.Emptyf(c, p.GetRepoTags(), "host SBOM should carry no repo tags")
			assert.Emptyf(c, p.GetRepoDigests(), "host SBOM should carry no repo digests")
			assert.Emptyf(c, propertyValues(metadataProperties(bom), propDiffID), "host SBOM should carry no layer DiffIDs")
			for _, comp := range comps {
				assert.Emptyf(c, propertyValues(comp.GetProperties(), propLayerDiffID), "host component %q should carry no LayerDiffID", comp.GetName())
			}
		}
	}, 10*time.Minute, 15*time.Second, "Failed finding/validating host SBOM")
}

func findComponent(comps []*cyclonedx_v1_4.Component, name string) *cyclonedx_v1_4.Component {
	for _, c := range comps {
		if c.GetName() == name {
			return c
		}
	}
	return nil
}

func findOSComponent(comps []*cyclonedx_v1_4.Component) *cyclonedx_v1_4.Component {
	for _, c := range comps {
		if c.GetType() == cyclonedx_v1_4.Classification_CLASSIFICATION_OPERATING_SYSTEM {
			return c
		}
	}
	return nil
}

func hasComponentWithPurlPrefix(comps []*cyclonedx_v1_4.Component, prefix string) bool {
	for _, c := range comps {
		if strings.HasPrefix(c.GetPurl(), prefix) {
			return true
		}
	}
	return false
}

// osPackagePurlPrefixes are the purl prefixes emitted by OS package managers.
// Anything else with a pkg: purl is an application/language package.
var osPackagePurlPrefixes = []string{"pkg:rpm/", "pkg:apk/", "pkg:deb/"}

// applicationComponents returns the application/language packages (npm, pypi,
// golang, ...): components with a pkg: purl that is not an OS package. This is
// what the "languages" analyzer contributes on top of OS packages.
func applicationComponents(comps []*cyclonedx_v1_4.Component) []*cyclonedx_v1_4.Component {
	var out []*cyclonedx_v1_4.Component
	for _, c := range comps {
		purl := c.GetPurl()
		if !strings.HasPrefix(purl, "pkg:") {
			continue
		}
		isOS := false
		for _, p := range osPackagePurlPrefixes {
			if strings.HasPrefix(purl, p) {
				isOS = true
				break
			}
		}
		if !isOS {
			out = append(out, c)
		}
	}
	return out
}

func propertyValues(props []*cyclonedx_v1_4.Property, name string) []string {
	var out []string
	for _, p := range props {
		if p.GetName() == name {
			out = append(out, p.GetValue())
		}
	}
	return out
}

func metadataProperties(bom *cyclonedx_v1_4.Bom) []*cyclonedx_v1_4.Property {
	if bom.Metadata == nil {
		return nil
	}
	return bom.Metadata.GetComponent().GetProperties()
}

// assertComponentLayer verifies a component is attributed to a real image layer.
// LayerDiffID is the layer's uncompressed-content diff_id and must be one of the
// image's real diff_ids. LayerDigest is the OCI manifest layer (compressed-blob)
// digest: containerd and CRI-O expose one, where it must be one of the image's
// real manifest layer digests (hence a sha256 distinct from the diff_id). Docker
// exposes no per-layer manifest digest and leaves it empty rather than fabricate
// one from the diff_id, which expectDigest gates.
func assertComponentLayer(c assert.TestingT, diffIDs, digests map[string]bool, comp *cyclonedx_v1_4.Component, image string, expectDigest bool) {
	diffID := propertyValues(comp.GetProperties(), propLayerDiffID)
	if !assert.Lenf(c, diffID, 1, "component %q should carry exactly one LayerDiffID in %s", comp.GetName(), image) {
		return
	}
	assert.Truef(c, diffIDs[diffID[0]], "component %q LayerDiffID %q is not a real diff_id of %s", comp.GetName(), diffID[0], image)

	digest := propertyValues(comp.GetProperties(), propLayerDigest)
	if !expectDigest {
		assert.Emptyf(c, digest, "component %q must carry no LayerDigest in %s: the runtime exposes no per-layer manifest digest", comp.GetName(), image)
		return
	}
	if assert.Lenf(c, digest, 1, "component %q should carry exactly one LayerDigest in %s", comp.GetName(), image) {
		assert.Truef(c, digests[digest[0]], "component %q LayerDigest %q is not a real manifest layer digest of %s", comp.GetName(), digest[0], image)
		assert.NotEqualf(c, diffID[0], digest[0], "component %q LayerDigest must differ from its LayerDiffID in %s", comp.GetName(), image)
	}
}

// realImageRootFS fetches, from the registry, the linux/amd64 ground truth for
// an image: the ordered rootfs DiffIDs and image config ID (ImageID) from the
// config blob, and the ordered per-layer compressed digests from the manifest
// blob. The manifest digests are the LayerDigest values the Agent must report
// for runtimes that expose them (containerd, CRI-O).
func realImageRootFS(ref string) (diffIDs []string, digests []string, imageID string, err error) {
	img, err := crane.Pull(ref, crane.WithPlatform(&v1.Platform{OS: "linux", Architecture: "amd64"}))
	if err != nil {
		return nil, nil, "", err
	}
	configName, err := img.ConfigName()
	if err != nil {
		return nil, nil, "", err
	}
	imageID = configName.String()
	cfg, err := img.ConfigFile()
	if err != nil {
		return nil, nil, "", err
	}
	for _, d := range cfg.RootFS.DiffIDs {
		diffIDs = append(diffIDs, d.String())
	}
	manifest, err := img.Manifest()
	if err != nil {
		return nil, nil, "", err
	}
	for _, l := range manifest.Layers {
		digests = append(digests, l.Digest.String())
	}
	return diffIDs, digests, imageID, nil
}
