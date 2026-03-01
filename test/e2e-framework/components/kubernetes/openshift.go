// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package kubernetes

import (
	_ "embed"
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

//go:embed openshift-control-plane-checks.sh
var OpenShiftControlPlaneChecksScriptContent string

func NewLocalOpenShiftCluster(env config.Env, name string, pullSecretPath string, opts ...pulumi.ResourceOption) (*Cluster, error) {
	return components.NewComponent(env, name, func(clusterComp *Cluster) error {
		openShiftClusterName := env.CommonNamer().DisplayName(49)
		opts = utils.MergeOptions[pulumi.ResourceOption](opts, pulumi.Parent(clusterComp))
		commonEnvironment := env
		runner := command.NewLocalRunner(env, command.LocalRunnerArgs{
			OSCommand: command.NewUnixOSCommand(),
		})

		crcSetup, err := runner.Command(commonEnvironment.CommonNamer().ResourceName("crc-setup"), &command.Args{
			Create: pulumi.String("crc setup"),
		}, opts...)
		if err != nil {
			return err
		}
		startCluster, err := runner.Command(commonEnvironment.CommonNamer().ResourceName("crc-start"), &command.Args{
			Create: pulumi.Sprintf("crc start -p %s", pullSecretPath),
			Delete: pulumi.String("crc stop"),
			Triggers: pulumi.Array{
				pulumi.String(pullSecretPath),
			},
		}, utils.MergeOptions(opts, utils.PulumiDependsOn(crcSetup))...)
		if err != nil {
			return err
		}

		kubeConfigCmd, err := runner.Command(commonEnvironment.CommonNamer().ResourceName("get-kubeconfig"), &command.Args{
			Create: pulumi.String("cat ~/.crc/machines/crc/kubeconfig"),
		}, utils.MergeOptions(opts, utils.PulumiDependsOn(startCluster))...)
		if err != nil {
			return err
		}

		clusterComp.KubeConfig = kubeConfigCmd.StdoutOutput()
		clusterComp.ClusterName = openShiftClusterName.ToStringOutput()
		return nil
	}, opts...)
}

func NewOpenShiftCluster(env config.Env, vm *remote.Host, name string, pullSecretPath string, opts ...pulumi.ResourceOption) (*Cluster, error) {
	return components.NewComponent(env, name, func(clusterComp *Cluster) error {
		openShiftClusterName := env.CommonNamer().DisplayName(49)
		opts = utils.MergeOptions[pulumi.ResourceOption](opts, pulumi.Parent(clusterComp))
		runner := vm.OS.Runner()
		commonEnvironment := env

		// Read and copy the pull secret file to the VM
		pullSecretContent, err := utils.ReadSecretFile(pullSecretPath)
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

		// Install crc dependencies and the crc binary
		installCRCDependencies, err := runner.Command(commonEnvironment.CommonNamer().ResourceName("install-libvirt"), &command.Args{
			Create: pulumi.String(`
		sudo dnf install -y libvirt NetworkManager`),
		}, opts...)
		if err != nil {
			return err
		}
		installCRC, err := InstallCRCBinary(env, vm, utils.MergeOptions(opts, utils.PulumiDependsOn(installCRCDependencies))...)
		if err != nil {
			return err
		}

		// To avoid the crc-daemon.service being stopped when the user session ends, we enable linger for the user
		enableLinger, err := runner.Command(commonEnvironment.CommonNamer().ResourceName("enable-linger"), &command.Args{
			Create: pulumi.String("loginctl enable-linger"),
		}, utils.MergeOptions(opts, utils.PulumiDependsOn(installCRCDependencies))...)
		if err != nil {
			return err
		}

		setupCRC, err := runner.Command(commonEnvironment.CommonNamer().ResourceName("crc-setup"), &command.Args{
			Create: pulumi.String("crc config set preset microshift && crc config set disk-size 100 && crc config set cpus 6 && crc config set memory 16384 && crc setup"),
		}, utils.MergeOptions(opts, utils.PulumiDependsOn(pullSecretFile, enableLinger, installCRC))...)
		if err != nil {
			return err
		}

		startCRC, err := runner.Command(commonEnvironment.CommonNamer().ResourceName("crc-start"), &command.Args{
			Create: pulumi.String(`crc start -p /tmp/pull-secret.txt`),
			Delete: pulumi.String("crc stop && crc delete && crc cleanup"),
			Triggers: pulumi.Array{
				pulumi.String(pullSecretPath),
			},
		}, utils.MergeOptions(opts, utils.PulumiDependsOn(setupCRC))...)
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
		}, utils.MergeOptions(opts, utils.PulumiDependsOn(startCRC, socatForwarding))...)
		if err != nil {
			return err
		}

		// Wait for the control plane to be fully ready before proceeding
		waitControlPlane, err := runner.Command(commonEnvironment.CommonNamer().ResourceName("wait-control-plane-ready"), &command.Args{
			Create: pulumi.String(OpenShiftControlPlaneChecksScriptContent),
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

func InstallCRCBinary(env config.Env, vm *remote.Host, opts ...pulumi.ResourceOption) (pulumi.Resource, error) {
	openShiftArch := vm.OS.Descriptor().Architecture
	if openShiftArch == oscomp.AMD64Arch {
		openShiftArch = "amd64"
	}
	return vm.OS.Runner().Command(
		env.CommonNamer().ResourceName("crc-install"),
		&command.Args{
			Create: pulumi.Sprintf(`curl -fsSL https://developers.redhat.com/content-gateway/file/pub/openshift-v4/clients/crc/2.52.0/crc-linux-%s.tar.xz | \
	sudo tar -xJ -C /usr/local/bin --strip-components=1 crc-linux-2.52.0-%s/crc`, openShiftArch, openShiftArch),
		}, opts...)
}
