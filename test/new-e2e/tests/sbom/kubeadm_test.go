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

// sbomHostRetentionPeriod overrides fakeintake's 15m default: the host SBOM
// emits its CycloneDX body only on the first scan (later scans are body-less
// heartbeats), and TestHostSBOM runs after TestContainerSBOM's long per-image
// Eventually, so the 15m default can purge that one payload before the read.
const sbomHostRetentionPeriod = "1h"

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
			scenkubeadm.WithFakeintakeOptions(fakeintake.WithMemory(2048), fakeintake.WithRetentionPeriod(sbomHostRetentionPeriod)),
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
			scenkubeadm.WithFakeintakeOptions(fakeintake.WithMemory(2048), fakeintake.WithRetentionPeriod(sbomHostRetentionPeriod)),
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
	tag          string
	digest       string
	imageID      string
	diffIDs      []string
	layerDigests []string
	// componentCount is the exact total component count (OS + language) for the
	// digest-pinned image; it is deterministic (identical across containerd and
	// crio), so the test asserts it exactly.
	componentCount int
	components     []expectedComponent
}

var containerTargets = []containerTarget{
	{
		short: "node", repo: "node", tag: "26.2.0",
		digest:  "sha256:980c5420a7a2ddcb44037726977f2a349e5c7b64217516c7488dce4c74d71583",
		imageID: "sha256:56122bfdab2ec6ccdfb5353a47d6a5ea08018cac6eb44e6a7cec699bdc038f2f",
		diffIDs: []string{
			"sha256:17d38572a7dcb03eff5bfd7354c717d7ee9c69b9d6a29f523722534201e411f4",
			"sha256:47dffecc554065cd728ca9db016fb3b7f3b5577dc8964ac34256e3869ada8db9",
			"sha256:8dc335c5db2d262d9c7a006c747d6102580d3797a3d3fc196b3690e61ba53022",
			"sha256:0e6530456c980dd1b447066f1cb9c2b28e32b3ddde18b48ee25491efb38f0382",
			"sha256:36f8b9c2a40fad592e5ede970f9160aff6a2b9e25b92fe056d777cb8abae641c",
			"sha256:163caf682e98170ebec9be7b2488094b92b8388d7abd34e90971d39e7f68bbfb",
			"sha256:3a2fd11428a5ec6ac86c3cf9de2ccd53bd6b38a2fb1ee0556386e47f9797e8c8",
		},
		layerDigests: []string{
			"sha256:f32f49ce655a9cf7c1fd4ca1417ddb39a54cedf4b7ff35de20f8009c18dd7a96",
			"sha256:8a7504cd2818ce40ac76c17886a03dff25ef0aa06ff6125bf0f0c7302cdc6471",
			"sha256:b53089dca50590292ecc77bf803152a5799650e734717e4b706cb812a02073ba",
			"sha256:8d6d44b254dab2063c4226fc8a0849d5527402d24d3bea80d644a1e4ac3a47e5",
			"sha256:bacec82ae09bce2a0299189b9f5e4266a9ff43adbd55a9ca2ad3cfff82afc63f",
			"sha256:9bc05f2fa3378d114b930cd832f8cb38b14a229fea9350e370ebb2c656361803",
			"sha256:326172fd43b935cd35f1db120b1001ad613abdad67fa49eb70bb68bcf73a5a93",
		},
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
		digest:  "sha256:91eda9776261207ea25fd06b5b7fed8d397dd2c0a283e77f2ab6e91bfa71079d",
		imageID: "sha256:b8fbd9862b05789bb9e5462bf7d67300660e2ab5217aeb637b8db103ac45ea21",
		diffIDs: []string{
			"sha256:29df493baa13de438d6d2ece3a8333032e0b7b9b9d8cce4ee82194da255f61e1",
			"sha256:a044995f677dc855dbd75a95d29117acba23684d7fac58182c25a8dbdd70d8f1",
			"sha256:bf2ba1efee40f4bea5fc45e72a2c97e65e2302341d474f3d56beb1bfc3bd7480",
			"sha256:307f40a40815bf428bac340faf5ad8c966e020c57797a0c9a32345eca6c1bb60",
			"sha256:5f70bf18a086007016e948b04aed3b82103a36bea41755b6cddfaf10ace3c6ef",
		},
		layerDigests: []string{
			"sha256:6a0ac1617861a677b045b7ff88545213ec31c0ff08763195a70a4a5adda577bb",
			"sha256:17423c887b377ec28c3923bc88337384a7a0c1f2b50b7faf4912760e8d503ebb",
			"sha256:1a70bdedd442d430ea119cf8db8c0031b4eedeb5bde886892773876ded7629e8",
			"sha256:1512b43389a873b0519ae3d6c1a3c4d9b19295b8e3330b040d95d73c50ba2f9e",
			"sha256:4f4fb700ef54461cfa02571ae0db9a0dc1e0cdb5577484a6d75e68dc38e8acc1",
		},
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
		digest:  "sha256:80b1f4c34a7eed1b03a05d12b55768f3e522eef6ec294c6fbd5fa47b6b2892ee",
		imageID: "sha256:c04f6c0e54326101acce230068ad4783242335106d0ec2322660c0f8dd72089c",
		diffIDs: []string{
			"sha256:470f7d5ad4c7fdffde1c80a31b1722932d09adbd9fec6c41454ac84337d15783",
		},
		layerDigests: []string{
			"sha256:9a785c83dc42af66688cf5d3dd8e0500ce65bad5a679b45744d62c2fffbb60d8",
		},
		componentCount: 189,
		components: []expectedComponent{
			{name: "glibc", layer: true},    // rpm (OS)
			{name: "bash"},                  // rpm (OS)
			{purlPrefix: "pkg:rpm/redhat/"}, // rpm package
		},
	},
	{
		// ubi9/python-312 is a multi-layer RHEL image: rpm (OS) + pip (pypi) across layers.
		short: "python-312", repo: "registry.access.redhat.com/ubi9/python-312", tag: "9.8-1779945122",
		digest:  "sha256:52d1ffcda3b9552934f947b7d41fb0cb66973bdc0d7e91814facadc126f68663",
		imageID: "sha256:ad02b9631880f45ec370056476ceb23031d67069b598e50829df5983f95c641f",
		diffIDs: []string{
			"sha256:71275925ca13ef2f569403246b30b57d44ee7fe1d932461993c525a61ecddecd",
			"sha256:24f9bb9093bd82585055c570f497aceff7363569a71199655e196d8809e68c98",
			"sha256:34a244bf94d4b67b4158a52d8026dbf3bed78f17b201d2411f5479655deb5909",
			"sha256:401bb44ee01c0f49978e8dac56f921e19fdd134c3de998672a9ba9a634139786",
		},
		layerDigests: []string{
			"sha256:8669339e03904c8270fbf33fd1b3fcb3ef7c957a6e9c23dbaf342cfa06f86a8e",
			"sha256:77af39e07c8772be94a5bbd590e288d7e5808598a5b8f657776376702d9f0d6b",
			"sha256:3da9d5da9c0d150232a8d10166e9501729302ed14fd8716bcb813eba4f01132c",
			"sha256:919d6b60d4b0380d56f4687d3bbe5d21add847eaf593a71049d4ff0aa6083007",
		},
		componentCount: 475,
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
		digest:  "sha256:250e5c97be05e1eb2272fbdbd810dfd638f9012e1e6f65c99390ad3239943a08",
		imageID: "sha256:f494e154bc1f458228780ebfb2cef8654f0b0e9c860e8bf3ce24fa49f509670a",
		diffIDs: []string{
			"sha256:17d38572a7dcb03eff5bfd7354c717d7ee9c69b9d6a29f523722534201e411f4",
			"sha256:47dffecc554065cd728ca9db016fb3b7f3b5577dc8964ac34256e3869ada8db9",
			"sha256:8dc335c5db2d262d9c7a006c747d6102580d3797a3d3fc196b3690e61ba53022",
			"sha256:0e6530456c980dd1b447066f1cb9c2b28e32b3ddde18b48ee25491efb38f0382",
			"sha256:d733499dfbe122f0e7c4e2dfaf53abc6181c8c4ebf85c54cffc4ce7e0d647189",
			"sha256:feefbf087553a613ca8cb9f3d9c1d71f22ea75cfcc766d2f16bc87d7d9675ee3",
			"sha256:d87bf50b46d0c178e8de5d3d8bf51665c999dd671c0de38dc84cdd914228a5cf",
		},
		layerDigests: []string{
			"sha256:f32f49ce655a9cf7c1fd4ca1417ddb39a54cedf4b7ff35de20f8009c18dd7a96",
			"sha256:8a7504cd2818ce40ac76c17886a03dff25ef0aa06ff6125bf0f0c7302cdc6471",
			"sha256:b53089dca50590292ecc77bf803152a5799650e734717e4b706cb812a02073ba",
			"sha256:8d6d44b254dab2063c4226fc8a0849d5527402d24d3bea80d644a1e4ac3a47e5",
			"sha256:0a4465cc9f09dc7bd8fce31cf033e53ff50a486cf15839bdc54eca5ac36b9eb2",
			"sha256:c965dce520b37b106f5c288e74f6012e9b05c65468dc8c04ef822c46c6abdfc1",
			"sha256:61719a06ef521afb108a06cdbdfebeaa93b146a042ebb781b7a728219a3108b3",
		},
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
		digest:  "sha256:d4233f4242ea25346f157709bb8417c615e7478468e2699c8e86a4e1f0156de8",
		imageID: "sha256:94de028496f47434dc707899bb5d38489554c3d1cc88c2501052302f8d7250ee",
		diffIDs: []string{
			"sha256:8f4ceb8cc1a2056b98f0424fad4715dd334aecc9769186b3ea0394f131524e27",
			"sha256:916d866d5b0dc17158c78e5a09717fcf619b04450125caafa9c1b8f7aa6a2c45",
			"sha256:0d80db6a0977d5ade37d5d248772c0e9aaa1b6c898ddb31b26c803ee1a9f57a2",
			"sha256:28e03088bc157bfe42d66970cf7f47de35821aabaece5f767804902a8fa1d779",
			"sha256:dabae3068d73ee8ac585dcb1f8a5e92853c8b08848900c149c884206c3518ec9",
			"sha256:1668f0142f52f4a99824f74fb149d5405eedb0415c55ca90e417ac45d2368bd4",
			"sha256:3b09e2b7aceadb3fdd11fce389a5a315748d6a6542c4d3d21fab8946c5f84470",
		},
		layerDigests: []string{
			"sha256:903681d87777d28dc56866a07a2774c3fd5bf65fd734b24c9d0ecd9a13c9f636",
			"sha256:3cbbe86a28c2f6b3c3e0e8c6dcfba369e1ea656cf8daf69be789e0fe2105982b",
			"sha256:6ed93aa58a52c9abc1ee472f1ac74b73d3adcccc2c30744498fd5f14f3f5d22c",
			"sha256:787c78da43830be6d988d34c7ee091f98d828516ce5478ca10a4933d655191bf",
			"sha256:1cd9229db862d463801f9e37a37ba49d4c1aa4b46e873cb3a3e31b736835f74b",
			"sha256:0d01fe7cfd1e043c33f90fdd5048dec68165450638b1d977c7f20ac31ed9de7b",
			"sha256:41b75ad313a6ea8772d1021a013072df0494022b5b7c77fe7df228fb662a8f20",
		},
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

				diffIDSet := make(map[string]bool, len(target.diffIDs))
				for _, d := range target.diffIDs {
					diffIDSet[d] = true
				}
				digestSet := make(map[string]bool, len(target.layerDigests))
				for _, d := range target.layerDigests {
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
					assert.Equalf(c, target.diffIDs, metaDiffIDs, "DiffID list does not match the real image config for %s", target.short)
					assert.Containsf(c, propertyValues(metaProps, propImageID), target.imageID, "ImageID does not match the real image config for %s", target.short)
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
