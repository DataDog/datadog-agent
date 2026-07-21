// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package kubernetes

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
)

// containerd ships in the docker-ce repository. RHEL 10 ($releasever=10) is not
// served yet, so we pin the repo to el9 - the el9 containerd.io binary runs
// unchanged on RHEL 10 and avoids depending on a release that may be missing.
const kubeadmContainerdRepoReleasever = "9"

// kubeadmFlannelVersion pins the flannel CNI manifest to a known-good release.
// The manifest was previously fetched from the floating
// .../releases/latest/download/kube-flannel.yml URL, which broke cluster
// provisioning (HTTP 404) when the upstream latest release stopped publishing
// that asset. Pin an explicit tag so the CNI apply does not depend on whatever
// the current upstream latest release happens to ship.
const kubeadmFlannelVersion = "v0.28.5"

// ContainerRuntime selects the CRI installed on the kubeadm node. The Agent
// produces identical SBOMs across runtimes; only the install steps and the CRI
// socket differ.
type ContainerRuntime int

const (
	// Containerd installs containerd as the container runtime (the default).
	Containerd ContainerRuntime = iota
	// CRIO installs CRI-O as the container runtime.
	CRIO
)

// rootScript wraps a multi-line script so the whole thing runs as root. The
// command runner only prepends "sudo " to the command (see unixOSCommand), which
// for a bare multi-line script would elevate just the first line; feeding the
// script to `sudo bash` via a heredoc runs every line as root instead.
func rootScript(script string) pulumi.StringInput {
	return pulumi.String("bash <<'KUBEADM_EOF'\nset -euxo pipefail\n" + script + "\nKUBEADM_EOF")
}

// NewKubeadmCluster brings up a single-node Kubernetes cluster directly on vm
// (a real host, not a nested container) using kubeadm with the selected
// container runtime (containerd or CRI-O). Because the node is the host, an
// Agent DaemonSet scheduled on it sees the real host filesystem and the host CRI
// socket - which is what lets it produce a host SBOM of the VM's OS and
// container-image SBOMs of the workloads.
//
// It mirrors NewOpenShiftCluster: an ordered chain of remote commands provisions
// the cluster, then the admin kubeconfig is captured and rewritten to reach the
// API server at the VM address (kubeadm binds 0.0.0.0:6443, so no port forward
// is needed).
func NewKubeadmCluster(env config.Env, vm *remote.Host, name string, kubeVersion string, runtime ContainerRuntime, opts ...pulumi.ResourceOption) (*Cluster, error) {
	return components.NewComponent(env, name, func(clusterComp *Cluster) error {
		clusterName := env.CommonNamer().DisplayName(49)
		opts = utils.MergeOptions[pulumi.ResourceOption](opts, pulumi.Parent(clusterComp))
		runner := vm.OS.Runner()
		namer := env.CommonNamer()

		// pkgs.k8s.io repositories are keyed by minor version (e.g. "v1.34").
		parsed := strings.TrimPrefix(utils.ParseKubernetesVersion(kubeVersion), "v")
		minor := parsed
		if parts := strings.SplitN(parsed, ".", 3); len(parts) >= 2 {
			minor = parts[0] + "." + parts[1]
		}

		prereqs, err := runner.Command(namer.ResourceName("kubeadm-prereqs"), &command.Args{
			Sudo: true,
			Create: rootScript(`cloud-init status --wait || true
# kubeadm/kubelet require swap off, bridged traffic visible to iptables, and IP forwarding.
swapoff -a
sed -ri 's/^([^#].*[[:space:]]swap[[:space:]])/#\1/' /etc/fstab || true
# Single-node e2e box: relax SELinux and firewalld rather than enumerate every port.
setenforce 0 || true
sed -i 's/^SELINUX=enforcing/SELINUX=permissive/' /etc/selinux/config || true
systemctl disable --now firewalld || true
# NetworkManager otherwise hijacks the CNI (flannel) interfaces on RHEL and breaks pod networking.
mkdir -p /etc/NetworkManager/conf.d
cat >/etc/NetworkManager/conf.d/k8s-cni.conf <<'EOF'
[keyfile]
unmanaged-devices=interface-name:cni0;interface-name:flannel*;interface-name:veth*
EOF
systemctl reload NetworkManager || true
cat >/etc/modules-load.d/k8s.conf <<'EOF'
overlay
br_netfilter
EOF
modprobe overlay
# br_netfilter lives in kernel-modules-extra on RHEL 10, which is absent for the running
# kernel on the minimal image; install it (and kernel-modules) for the exact running kernel.
modprobe br_netfilter || { dnf install -y "kernel-modules-extra-$(uname -r)" "kernel-modules-$(uname -r)" || true; depmod -a || true; modprobe br_netfilter; }
cat >/etc/sysctl.d/99-kubernetes.conf <<'EOF'
net.bridge.bridge-nf-call-iptables  = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward                 = 1
EOF
sysctl --system || true`),
		}, opts...)
		if err != nil {
			return err
		}

		// The container-runtime install is the only runtime-specific step;
		// everything downstream (kubelet/kubeadm/kubectl, flannel, readiness) and
		// the SBOM the Agent produces are runtime-agnostic. criSocket is reused by
		// the kubeadm init below.
		criSocket := "unix:///run/containerd/containerd.sock"
		runtimeName := "kubeadm-containerd"
		runtimeScript := fmt.Sprintf(`curl -fsSL https://download.docker.com/linux/centos/docker-ce.repo -o /etc/yum.repos.d/docker-ce.repo
sed -i 's/\$releasever/%s/g' /etc/yum.repos.d/docker-ce.repo
dnf install -y containerd.io
mkdir -p /etc/containerd
containerd config default >/etc/containerd/config.toml
# kubelet uses the systemd cgroup driver; containerd must match or the node never goes Ready.
sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml
# Route docker.io pulls through mirror.gcr.io to avoid Docker Hub anonymous rate limits (429).
# containerd 2.x writes a version-3 config that single-quotes the value (config_path = ''),
# while 1.x double-quotes it; match either quote form so the certs.d mirror is actually enabled.
sed -i -E 's#^([[:space:]]*config_path[[:space:]]*=[[:space:]]*).*#\1"/etc/containerd/certs.d"#' /etc/containerd/config.toml
mkdir -p /etc/containerd/certs.d/docker.io
cat >/etc/containerd/certs.d/docker.io/hosts.toml <<'HOSTS'
server = "https://docker.io"
[host."https://mirror.gcr.io"]
  capabilities = ["pull", "resolve"]
[host."https://registry-1.docker.io"]
  capabilities = ["pull", "resolve"]
[host."https://docker.io"]
  capabilities = ["pull", "resolve"]
HOSTS
systemctl enable --now containerd
systemctl restart containerd`, kubeadmContainerdRepoReleasever)
		if runtime == CRIO {
			criSocket = "unix:///var/run/crio/crio.sock"
			runtimeName = "kubeadm-crio"
			// CRI-O ships in pkgs.k8s.io under addons:/cri-o. Its stable streams can
			// lag the Kubernetes core repo (a new k8s minor may have no matching
			// CRI-O stream yet), so probe down from the kubelet minor and use the
			// newest stream that exists. The repo is $basearch (no $releasever), so
			// one repo serves RHEL 9 and 10, and CRI-O defaults to the systemd
			// cgroup driver, matching the kubelet.
			runtimeScript = fmt.Sprintf(`kmaj=$(echo %[1]s | cut -d. -f1); kmin=$(echo %[1]s | cut -d. -f2)
crio_ver=""
for d in 0 1 2 3 4; do
  cand="v${kmaj}.$((kmin-d))"
  if curl -fsSL -o /dev/null "https://pkgs.k8s.io/addons:/cri-o:/stable:/${cand}/rpm/repodata/repomd.xml"; then crio_ver="$cand"; break; fi
done
test -n "$crio_ver" || { echo "no CRI-O stable stream available at or below v%[1]s"; exit 1; }
cat >/etc/yum.repos.d/cri-o.repo <<EOF
[cri-o]
name=CRI-O
baseurl=https://pkgs.k8s.io/addons:/cri-o:/stable:/${crio_ver}/rpm/
enabled=1
gpgcheck=1
gpgkey=https://pkgs.k8s.io/addons:/cri-o:/stable:/${crio_ver}/rpm/repodata/repomd.xml.key
EOF
dnf install -y cri-o container-selinux
# Route docker.io pulls through mirror.gcr.io to avoid Docker Hub anonymous rate limits (429).
mkdir -p /etc/containers/registries.conf.d
cat >/etc/containers/registries.conf.d/000-docker-mirror.conf <<'EOF'
[[registry]]
prefix = "docker.io"
location = "docker.io"
[[registry.mirror]]
location = "mirror.gcr.io"
EOF
# Drop CRI-O's packaged CNI bridge so flannel owns pod networking (the containerd path has none).
rm -f /etc/cni/net.d/*crio* || true
systemctl enable --now crio`, minor)
		}

		runtimeInstall, err := runner.Command(namer.ResourceName(runtimeName), &command.Args{
			Sudo:   true,
			Create: rootScript(runtimeScript),
		}, utils.MergeOptions(opts, utils.PulumiDependsOn(prereqs))...)
		if err != nil {
			return err
		}

		tools, err := runner.Command(namer.ResourceName("kubeadm-tools"), &command.Args{
			Sudo: true,
			Create: rootScript(fmt.Sprintf(`cat >/etc/yum.repos.d/kubernetes.repo <<'EOF'
[kubernetes]
name=Kubernetes
baseurl=https://pkgs.k8s.io/core:/stable:/v%[1]s/rpm/
enabled=1
gpgcheck=1
gpgkey=https://pkgs.k8s.io/core:/stable:/v%[1]s/rpm/repodata/repomd.xml.key
exclude=kubelet kubeadm kubectl cri-tools
EOF
dnf install -y kubelet kubeadm kubectl cri-tools --disableexcludes=kubernetes
systemctl enable --now kubelet`, minor)),
		}, utils.MergeOptions(opts, utils.PulumiDependsOn(runtimeInstall))...)
		if err != nil {
			return err
		}

		initCluster, err := runner.Command(namer.ResourceName("kubeadm-init"), &command.Args{
			Sudo: true,
			Create: rootScript(fmt.Sprintf(`PRIVATE_IP="$(hostname -I | awk '{print $1}')"
kubeadm init \
  --pod-network-cidr=10.244.0.0/16 \
  --cri-socket=%s \
  --apiserver-advertise-address="${PRIVATE_IP}" \
  --apiserver-cert-extra-sans="${PRIVATE_IP}"`, criSocket)),
			Delete: rootScript("kubeadm reset -f || true\nrm -rf /etc/cni/net.d /etc/kubernetes || true"),
		}, utils.MergeOptions(opts, utils.PulumiDependsOn(tools))...)
		if err != nil {
			return err
		}

		// Flannel's default network is 10.244.0.0/16, matching --pod-network-cidr above.
		// Pin the manifest to a specific flannel release (see kubeadmFlannelVersion)
		// rather than the floating latest release, whose kube-flannel.yml asset can
		// disappear and 404 the CNI apply.
		cni, err := runner.Command(namer.ResourceName("kubeadm-cni"), &command.Args{
			Sudo: true,
			Create: rootScript(fmt.Sprintf(`export KUBECONFIG=/etc/kubernetes/admin.conf
kubectl apply -f https://raw.githubusercontent.com/flannel-io/flannel/%s/Documentation/kustomization/kube-flannel/kube-flannel.yml`, kubeadmFlannelVersion)),
		}, utils.MergeOptions(opts, utils.PulumiDependsOn(initCluster))...)
		if err != nil {
			return err
		}

		// Untaint the control-plane so the single node also schedules workloads,
		// then block until the node is Ready (this gates the kubeconfig export).
		waitReady, err := runner.Command(namer.ResourceName("kubeadm-wait-ready"), &command.Args{
			Sudo: true,
			Create: rootScript(`export KUBECONFIG=/etc/kubernetes/admin.conf
kubectl taint nodes --all node-role.kubernetes.io/control-plane- || true
kubectl taint nodes --all node-role.kubernetes.io/master- || true
for i in $(seq 1 60); do
  if kubectl get nodes --no-headers 2>/dev/null | grep -qw Ready; then break; fi
  echo "waiting for node Ready ($i/60)"; sleep 10
done
kubectl wait --for=condition=Ready node --all --timeout=300s
kubectl get nodes -o wide`),
		}, utils.MergeOptions(opts, utils.PulumiDependsOn(cni))...)
		if err != nil {
			return err
		}

		kubeConfigCmd, err := runner.Command(namer.ResourceName("kubeadm-kubeconfig"), &command.Args{
			Sudo:   true,
			Create: pulumi.String("cat /etc/kubernetes/admin.conf"),
		}, utils.MergeOptions(opts, utils.PulumiDependsOn(waitReady))...)
		if err != nil {
			return err
		}

		clusterComp.KubeConfig = pulumi.All(kubeConfigCmd.StdoutOutput(), vm.Address, waitReady.StdoutOutput()).ApplyT(func(args []interface{}) string {
			kubeconfigRaw := args[0].(string)
			vmIP := args[1].(string)
			// args[2] forces this to run after the readiness wait.
			insecure := regexp.MustCompile(`certificate-authority-data:.+`).ReplaceAllString(kubeconfigRaw, "insecure-skip-tls-verify: true")
			return regexp.MustCompile(`server: https://\S+`).ReplaceAllString(insecure, "server: https://"+vmIP+":6443")
		}).(pulumi.StringOutput)
		clusterComp.ClusterName = clusterName.ToStringOutput()
		return nil
	}, opts...)
}
