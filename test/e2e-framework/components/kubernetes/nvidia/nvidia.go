// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package nvidia

import (
	_ "embed"
	"fmt"
	"regexp"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"

	pulumik8s "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
)

const nvkindPackage = "github.com/NVIDIA/nvkind/cmd/nvkind"

const nvkindClusterValues = `
image: %s
workers:
- devices: all
`

//go:embed nvkind-config-template.yml
var nvkindConfigTemplate string

// KindCluster represents a Kubernetes cluster that is GPU-enabled using nvkind.
// Both the cluster and the GPU operator should be depended on for resources that require GPU support.
type KindCluster struct {
	*kubernetes.Cluster

	GPUOperator *helm.Release
}

// NewKindCluster creates a new Kubernetes cluster using nvkind so that nodes can be GPU-enabled.
// We need a different function rather than re-using kubernetes.NewKindCluster because we GPU-enabled kind
// clusters require a set of patches that aren't trivial. Instead of writing them all down here, we have
// decided to use the nvkind tool to create the cluster. This means that we cannot follow the same code path
// as for regular kind clusters.
func NewKindCluster(env config.Env, vm *remote.Host, name string, clusterOpts *KindClusterOptions, opts ...pulumi.ResourceOption) (*KindCluster, error) {
	// Configure the nvidia container toolkit
	cmd, err := configureContainerToolkit(env, vm, clusterOpts, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare NVIDIA runtime: %w", err)
	}
	opts = utils.MergeOptions(opts, utils.PulumiDependsOn(cmd))

	// Create the cluster
	cluster, err := initNvkindCluster(env, vm, name, clusterOpts, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create nvkind cluster: %w", err)
	}

	// Create the provider based on the kubeconfig output we have
	cluster.KubeProvider, err = pulumik8s.NewProvider(env.Ctx(), env.CommonNamer().ResourceName("k8s-provider"), &pulumik8s.ProviderArgs{
		EnableServerSideApply: pulumi.Bool(true),
		Kubeconfig:            cluster.KubeConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("kubernetes.NewProvider: %w", err)
	}
	opts = append(opts, pulumi.Provider(cluster.KubeProvider), pulumi.Parent(cluster.KubeProvider), pulumi.DeletedWith(cluster.KubeProvider))

	// Now install the operator
	operator, err := installGPUOperator(env, clusterOpts, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to install GPU operator: %w", err)
	}

	return &KindCluster{
		Cluster:     cluster,
		GPUOperator: operator,
	}, nil
}

func configureContainerToolkit(env config.Env, vm *remote.Host, clusterOpts *KindClusterOptions, opts ...pulumi.ResourceOption) (pulumi.Resource, error) {
	// Ensure we have Docker
	dockerManager, err := docker.NewManager(env, vm, opts...)
	if err != nil {
		return nil, err
	}
	opts = utils.MergeOptions(opts, utils.PulumiDependsOn(dockerManager))

	// Enable the NVIDIA Container Toolkit (nvidia-ctk) runtime for Docker
	// Source for these commands: https://github.com/NVIDIA/nvkind#setup
	ctkRuntime := "sudo nvidia-ctk runtime configure --runtime=docker --set-as-default --cdi.enabled "
	ctkConfig := "sudo nvidia-ctk config --set accept-nvidia-visible-devices-as-volume-mounts=true --in-place"
	ctkDockerRestart := "sudo systemctl restart docker"
	ctkConfigureCmdline := strings.Join([]string{ctkRuntime, ctkConfig, ctkDockerRestart}, " && ")

	ctkConfigureCmd, err := vm.OS.Runner().Command(
		env.CommonNamer().ResourceName("nvidia-ctk-configure"),
		&command.Args{
			Create: pulumi.String(ctkConfigureCmdline),
		},
		opts...,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to configure NVIDIA runtime: %w", err)
	}

	// Validate that the runtime is configured properly
	return vm.OS.Runner().Command(
		env.CommonNamer().ResourceName("nvidia-ctk-check"),
		&command.Args{
			Create: pulumi.Sprintf("docker run --runtime=nvidia -e NVIDIA_VISIBLE_DEVICES=all %s nvidia-smi -L", clusterOpts.cudaSanityCheckImage),
		},
		utils.MergeOptions(opts, utils.PulumiDependsOn(ctkConfigureCmd))...,
	)
}

// installNvkind installs the nvkind tool with all the necessary requisites
func installNvkind(env config.Env, vm *remote.Host, kindVersion string, clusterOpts *KindClusterOptions, opts ...pulumi.ResourceOption) (command.Command, error) {
	// kind is a requisite for nvkind, as it calls it under the hood
	kindInstall, err := kubernetes.InstallKindBinary(env, vm, kindVersion, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to install kind: %w", err)
	}

	// kubectl is a requisite for nvkind, it's called under the hood
	kubectlInstall, err := vm.OS.Runner().Command(
		env.CommonNamer().ResourceName("kubectl-install"),
		&command.Args{
			// use snap installer as it contains multiple versions, rather than APT
			Create: pulumi.Sprintf("sudo snap install kubectl --classic --channel=%s/stable", clusterOpts.kubeVersion),
		},
		opts...,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to install kubectl: %w", err)
	}

	// We need Go to install nvkind as it's a go package
	golangInstall, err := vm.OS.Runner().Command(
		env.CommonNamer().ResourceName("golang-install"),
		&command.Args{
			// use snap installer as it contains multiple versions, rather than APT
			Create: pulumi.Sprintf("sudo snap install --classic go --channel=%s/stable", clusterOpts.hostGoVersion),
		},
		opts...,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to install golang: %w", err)
	}

	// Install nvkind using go install
	nvkindInstall, err := vm.OS.Runner().Command(
		env.CommonNamer().ResourceName("nvkind-install"),
		&command.Args{
			// Ensure it gets installed to the global $PATH to avoid having to copy it or change $PATH
			Create: pulumi.Sprintf("sudo GOBIN=/usr/local/bin go install %s@%s", nvkindPackage, clusterOpts.nvkindVersion),
		},
		utils.MergeOptions(opts, utils.PulumiDependsOn(golangInstall, kindInstall, kubectlInstall))...,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to install nvkind: %w", err)
	}

	return nvkindInstall, nil
}

// inivtNvkindCluster creates a new Kubernetes cluster using nvkind so that nodes can be GPU-enabled, installing
// the necessary components and configuring the cluster.
func initNvkindCluster(env config.Env, vm *remote.Host, name string, clusterOpts *KindClusterOptions, opts ...pulumi.ResourceOption) (*kubernetes.Cluster, error) {
	return components.NewComponent(env, name, func(clusterComp *kubernetes.Cluster) error {
		opts = utils.MergeOptions[pulumi.ResourceOption](opts, pulumi.Parent(clusterComp))
		kindVersionConfig, err := kubernetes.GetKindVersionConfig(clusterOpts.kubeVersion)
		if err != nil {
			return err
		}

		// Install nvkind to create the cluster
		nvkindInstall, err := installNvkind(env, vm, kindVersionConfig.KindVersion, clusterOpts, opts...)
		if err != nil {
			return fmt.Errorf("failed to install nvkind: %w", err)
		}
		opts = utils.MergeOptions(opts, utils.PulumiDependsOn(nvkindInstall))

		// Copy the cluster configuration to the VM
		nvkindTemplatePath := "/tmp/nvkind-config.yaml"
		nvkindTemplate, err := vm.OS.FileManager().CopyInlineFile(
			pulumi.String(nvkindConfigTemplate),
			nvkindTemplatePath,
			opts...)
		if err != nil {
			return err
		}

		nodeImage := fmt.Sprintf("%s/%s:%s", env.InternalDockerhubMirror(), clusterOpts.kindImage, kindVersionConfig.NodeImageVersion)
		nvkindValuesPath := "/tmp/nvkind-values.yaml"
		nvkindValuesContent := pulumi.Sprintf(nvkindClusterValues, nodeImage)
		nvkindValues, err := vm.OS.FileManager().CopyInlineFile(
			nvkindValuesContent,
			nvkindValuesPath,
			opts...)
		if err != nil {
			return err
		}

		opts = utils.MergeOptions(opts, utils.PulumiDependsOn(nvkindTemplate, nvkindValues))

		// Run the nvkind command to create the cluster
		kindClusterName := env.CommonNamer().DisplayName(49)
		nvkindCreateCluster, err := vm.OS.Runner().Command(
			env.CommonNamer().ResourceName("nvkind-create"),
			&command.Args{
				Create:   pulumi.Sprintf("nvkind cluster create --name %s --config-template %s --config-values %s", kindClusterName, nvkindTemplatePath, nvkindValuesPath),
				Delete:   pulumi.Sprintf("kind delete clusters %s || true", kindClusterName),
				Triggers: pulumi.Array{nvkindValuesContent, pulumi.String(nvkindConfigTemplate)},
			},
			opts...)
		if err != nil {
			return fmt.Errorf("failed to create nvkind cluster: %w", err)
		}
		opts = utils.MergeOptions(opts, utils.PulumiDependsOn(nvkindCreateCluster))

		// Get the kubeconfig for the cluster so we can connect to it
		kubeConfigCmd, err := vm.OS.Runner().Command(
			env.CommonNamer().ResourceName("kind-kubeconfig"),
			&command.Args{
				Create: pulumi.Sprintf("kind get kubeconfig --name %s", kindClusterName),
			},
			opts...,
		)
		if err != nil {
			return err
		}

		// Patch Kubeconfig based on private IP output
		// Also add skip tls
		clusterComp.KubeConfig = pulumi.All(kubeConfigCmd.StdoutOutput(), vm.Address).ApplyT(func(args []interface{}) string {
			allowInsecure := regexp.MustCompile("certificate-authority-data:.+").ReplaceAllString(args[0].(string), "insecure-skip-tls-verify: true")
			return strings.ReplaceAll(allowInsecure, "0.0.0.0", args[1].(string))
		}).(pulumi.StringOutput)
		clusterComp.ClusterName = kindClusterName.ToStringOutput()

		return nil
	}, opts...)
}

// installGPUOperator installs the GPU operator in the cluster
func installGPUOperator(env config.Env, clusterOpts *KindClusterOptions, opts ...pulumi.ResourceOption) (*helm.Release, error) {
	// Create namespace
	operatorNs := "gpu-operator"
	ns, err := corev1.NewNamespace(env.Ctx(), operatorNs, &corev1.NamespaceArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name: pulumi.String(operatorNs),
		},
	}, opts...)
	if err != nil {
		return nil, err
	}
	opts = append(opts, utils.PulumiDependsOn(ns))

	helmInstall, err := helm.NewRelease(env.Ctx(), "gpu-operator", &helm.ReleaseArgs{
		RepositoryOpts: helm.RepositoryOptsArgs{
			Repo: pulumi.String("https://nvidia.github.io/gpu-operator"),
		},
		Chart:            pulumi.String("gpu-operator"),
		Namespace:        pulumi.String(operatorNs),
		Version:          pulumi.String(clusterOpts.gpuOperatorVersion),
		CreateNamespace:  pulumi.Bool(true),
		DependencyUpdate: pulumi.BoolPtr(true),
	}, opts...)
	if err != nil {
		return nil, err
	}

	return helmInstall, nil
}
