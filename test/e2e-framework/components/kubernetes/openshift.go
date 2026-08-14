// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package kubernetes

import (
	"regexp"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	oscomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
)

const crcVersion = "2.58.0"

// OpenShiftClusterArgs holds the configuration parameters for a CRC-based OpenShift cluster.
type OpenShiftClusterArgs struct {
	PullSecretPath string
	CPUs           string
	Memory         string
	Disk           string
}

func NewLocalOpenShiftCluster(env config.Env, name string, args OpenShiftClusterArgs, pulumiResourceOptions ...pulumi.ResourceOption) (*Cluster, error) {
	return components.NewComponent(env, name, func(clusterComp *Cluster) error {
		openShiftClusterName := env.CommonNamer().DisplayName(49)
		pulumiResourceOptions = utils.MergeOptions[pulumi.ResourceOption](pulumiResourceOptions, pulumi.Parent(clusterComp))
		commonEnvironment := env
		runner := command.NewLocalRunner(env, command.LocalRunnerArgs{
			OSCommand: command.NewUnixOSCommand(),
		})

		crcSetup, err := runner.Command(commonEnvironment.CommonNamer().ResourceName("crc-setup"), &command.Args{
			Create: pulumi.Sprintf("crc config set cpus %s && crc config set memory %s && crc config set disk-size %s && crc setup", args.CPUs, args.Memory, args.Disk),
			Delete: pulumi.String("crc cleanup"),
		}, pulumiResourceOptions...)
		if err != nil {
			return err
		}
		// Run crc start in a new session (setsid) so that vfkit — the hypervisor
		// child process — ends up in a different process group. The pulumi-command
		// provider uses Setpgid + CommandKiller to kill the entire process group on
		// cleanup; without setsid, vfkit gets SIGTERM'd when pulumi exits.
		startCluster, err := runner.Command(commonEnvironment.CommonNamer().ResourceName("crc-start"), &command.Args{
			Create: pulumi.Sprintf(`python3 -c "import os,subprocess,sys; os.setsid(); sys.exit(subprocess.call(sys.argv[1:]))" crc start -p %s`, args.PullSecretPath),
			Delete: pulumi.String("(crc stop || true) && crc delete -f"),
			Triggers: pulumi.Array{
				pulumi.String(args.PullSecretPath),
				pulumi.String(args.CPUs),
				pulumi.String(args.Memory),
				pulumi.String(args.Disk),
			},
		}, utils.MergeOptions(pulumiResourceOptions, utils.PulumiDependsOn(crcSetup), pulumi.DeleteBeforeReplace(true))...)
		if err != nil {
			return err
		}

		kubeConfigCmd, err := runner.Command(commonEnvironment.CommonNamer().ResourceName("get-kubeconfig"), &command.Args{
			Create: pulumi.String("cat ~/.crc/machines/crc/kubeconfig"),
		}, utils.MergeOptions(pulumiResourceOptions, utils.PulumiDependsOn(startCluster))...)
		if err != nil {
			return err
		}

		clusterComp.KubeConfig = kubeConfigCmd.StdoutOutput()
		clusterComp.ClusterName = openShiftClusterName.ToStringOutput()
		return nil
	}, pulumiResourceOptions...)
}

func NewOpenShiftCluster(env config.Env, vm *remote.Host, name string, args OpenShiftClusterArgs, opts ...pulumi.ResourceOption) (*Cluster, error) {
	return components.NewComponent(env, name, func(clusterComp *Cluster) error {
		openShiftClusterName := env.CommonNamer().DisplayName(49)
		opts = utils.MergeOptions[pulumi.ResourceOption](opts, pulumi.Parent(clusterComp))
		runner := vm.OS.Runner()
		commonEnvironment := env

		openShiftInstallBinary, err := InstallOpenShiftBinary(env, vm, opts...)
		if err != nil {
			return err
		}

		pullSecretContent, err := utils.ReadSecretFile(args.PullSecretPath)
		if err != nil {
			return err
		}
		pullSecretFile, err := vm.OS.FileManager().CopyInlineFile(
			pullSecretContent,
			"/tmp/pull-secret.txt",
		)
		if err != nil {
			return err
		}

		installLibvirt, err := runner.Command(commonEnvironment.CommonNamer().ResourceName("install-libvirt"), &command.Args{
			Create: pulumi.String(`
		sudo dnf install -y libvirt NetworkManager`),
		}, utils.MergeOptions(opts, utils.PulumiDependsOn(openShiftInstallBinary))...)
		if err != nil {
			return err
		}
		// To avoid the crc-daemon.service being stopped when the user session ends, we enable linger for the user
		enableLinger, err := runner.Command(commonEnvironment.CommonNamer().ResourceName("enable-linger"), &command.Args{
			Create: pulumi.String("loginctl enable-linger"),
		}, utils.MergeOptions(opts, utils.PulumiDependsOn(installLibvirt))...)
		if err != nil {
			return err
		}

		setupCRC, err := runner.Command(commonEnvironment.CommonNamer().ResourceName("crc-setup"), &command.Args{
			Create: pulumi.Sprintf("crc config set cpus %s && crc config set memory %s && crc config set disk-size %s && crc setup", args.CPUs, args.Memory, args.Disk),
			Delete: pulumi.String("crc cleanup"),
		}, utils.MergeOptions(opts, utils.PulumiDependsOn(pullSecretFile, enableLinger))...)
		if err != nil {
			return err
		}

		startCRC, err := runner.Command(commonEnvironment.CommonNamer().ResourceName("crc-start"), &command.Args{
			Create: pulumi.String(`crc start -p /tmp/pull-secret.txt`),
			Delete: pulumi.String("(crc stop || true) && crc delete -f"),
			Triggers: pulumi.Array{
				pulumi.String(args.PullSecretPath),
				pulumi.String(args.CPUs),
				pulumi.String(args.Memory),
				pulumi.String(args.Disk),
			},
		}, utils.MergeOptions(opts, utils.PulumiDependsOn(setupCRC), pulumi.DeleteBeforeReplace(true))...)
		if err != nil {
			return err
		}
		socatInstall, err := runner.Command(commonEnvironment.CommonNamer().ResourceName("install-socat"), &command.Args{
			Create: pulumi.String("sudo dnf install -y socat"),
		}, utils.MergeOptions(opts, utils.PulumiDependsOn(startCRC))...)
		if err != nil {
			return err
		}

		socatForwarding, err := runner.Command(commonEnvironment.CommonNamer().ResourceName("socat-kubeapi-proxy"), &command.Args{
			Create: pulumi.String(`
                sudo nohup socat TCP-LISTEN:8443,bind=0.0.0.0,fork TCP:127.0.0.1:6443 > /tmp/socat.log 2>&1 &
            `),
		}, utils.MergeOptions(opts, utils.PulumiDependsOn(socatInstall))...)
		if err != nil {
			return err
		}

		kubeConfig, err := runner.Command(commonEnvironment.CommonNamer().ResourceName("get-kubeconfig"), &command.Args{
			Create: pulumi.String("cat ~/.crc/machines/crc/kubeconfig"),
		}, utils.MergeOptions(opts, utils.PulumiDependsOn(socatForwarding))...)
		if err != nil {
			return err
		}

		// Wait for the control plane to be fully ready before proceeding
		waitControlPlane, err := runner.Command(commonEnvironment.CommonNamer().ResourceName("wait-control-plane-ready"), &command.Args{
			Create: pulumi.String(`
# Wait for API server to be responsive
for i in {1..30}; do
  if curl -sk https://127.0.0.1:6443/healthz 2>/dev/null | grep -q ok; then
    echo "API server responsive"
    break
  fi
  echo "Waiting for API server (attempt $i/30)..."
  sleep 10
done

export KUBECONFIG=~/.crc/machines/crc/kubeconfig

# Wait for nodes to be ready
echo "Waiting for nodes to be Ready..."
for i in {1..60}; do
  ready_nodes=$(kubectl get nodes --no-headers 2>/dev/null | grep -c ' Ready ')
  if [ "$ready_nodes" -gt 0 ]; then
    echo "Found $ready_nodes Ready nodes"
    break
  fi
  echo "Waiting for nodes (attempt $i/60)..."
  sleep 5
done

# Wait for some system pods to be running
echo "Waiting for system pods to be running..."
for namespace in openshift-kube-apiserver openshift-kube-controller-manager; do
  for i in {1..60}; do
    running_pods=$(kubectl get pods -n "$namespace" --field-selector=status.phase=Running --no-headers 2>/dev/null | wc -l)
    if [ "$running_pods" -gt 0 ]; then
      echo "Namespace $namespace has $running_pods running pod(s)"
      break
    fi
    if [ $i -lt 60 ]; then
      echo "Waiting for $namespace pods (attempt $i/60)..."
      sleep 5
    fi
  done
done

echo "Control plane is ready"
exit 0
`),
		}, utils.MergeOptions(opts, utils.PulumiDependsOn(kubeConfig))...)
		if err != nil {
			return err
		}

		clusterComp.KubeConfig = pulumi.All(kubeConfig.StdoutOutput(), vm.Address, waitControlPlane.StdoutOutput()).ApplyT(func(args []interface{}) string {
			kubeconfigRaw := args[0].(string)
			vmIP := args[1].(string)
			// args[2] is the output from waitControlPlane, ensuring it completes first
			allowInsecure := regexp.MustCompile("certificate-authority-data:.+").ReplaceAllString(kubeconfigRaw, "insecure-skip-tls-verify: true")
			updated := strings.ReplaceAll(allowInsecure, "api.crc.testing:6443", vmIP+":8443")
			return updated
		}).(pulumi.StringOutput)
		clusterComp.ClusterName = openShiftClusterName.ToStringOutput()
		return nil
	}, opts...)
}

func InstallOpenShiftBinary(env config.Env, vm *remote.Host, opts ...pulumi.ResourceOption) (pulumi.Resource, error) {
	openShiftArch := vm.OS.Descriptor().Architecture
	if openShiftArch == oscomp.AMD64Arch {
		openShiftArch = "amd64"
	}
	return vm.OS.Runner().Command(
		env.CommonNamer().ResourceName("crc-install"),
		&command.Args{
			Create: pulumi.Sprintf(`curl -fsSL https://developers.redhat.com/content-gateway/file/pub/openshift-v4/clients/crc/%s/crc-linux-%s.tar.xz | \
	sudo tar -xJ -C /usr/local/bin --strip-components=1 crc-linux-%s-%s/crc`, crcVersion, openShiftArch, crcVersion, openShiftArch),
		}, opts...)
}
