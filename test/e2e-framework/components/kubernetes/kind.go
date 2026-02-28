// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package kubernetes

import (
	"embed"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
)

const (
	kindReadinessWait = "60s"
	kindNodeImageName = "kindest/node"
)

//go:embed kind-cluster.yaml
var kindClusterConfig embed.FS

//go:embed hosts.toml
var containerdDockerioHostConfig string

// Install Kind on a Linux virtual machine.
func NewKindCluster(env config.Env, vm *remote.Host, name string, kubeVersion string, opts ...pulumi.ResourceOption) (*Cluster, error) {
	return NewKindClusterWithConfig(env, vm, name, kubeVersion, KindConfigFlags{}, opts...)
}

func validateKubeVersionFormat(kubeVersion string) error {
	// Pattern: v{semver}@sha256:{hash}
	// Example: v1.32.0@sha256:c48c62eac5da28cdadcf560d1d8616cfa6783b58f0d94cf63ad1bf49600cb027
	pattern := `^v\d+\.\d+\.\d+@sha256:[a-f0-9]{64}$`
	matched, err := regexp.MatchString(pattern, kubeVersion)
	if err != nil {
		return fmt.Errorf("error validating kubeVersion format: %w", err)
	}
	if !matched {
		return fmt.Errorf("kubeVersion must be in format 'v{semver}@sha256:{hash}' (e.g., v1.32.0@sha256:c48c62eac5da28cdadcf560d1d8616cfa6783b58f0d94cf63ad1bf49600cb027), got: %s", kubeVersion)
	}
	return nil
}

func NewKindClusterWithConfig(env config.Env, vm *remote.Host, name, kubeVersion string, kindFlags KindConfigFlags, opts ...pulumi.ResourceOption) (*Cluster, error) {
	return components.NewComponent(env, name, func(clusterComp *Cluster) error {
		kindClusterName := env.CommonNamer().DisplayName(49) // We can have some issues if the name is longer than 50 characters
		opts = utils.MergeOptions[pulumi.ResourceOption](opts, pulumi.Parent(clusterComp))
		runner := vm.OS.Runner()
		commonEnvironment := env
		packageManager := vm.OS.PackageManager()
		curlCommand, err := packageManager.Ensure("curl", nil, "", os.WithPulumiResourceOptions(opts...))
		if err != nil {
			return err
		}

		dockerManager, err := docker.NewManager(env, vm, opts...)
		if err != nil {
			return err
		}
		opts = utils.MergeOptions(opts, utils.PulumiDependsOn(dockerManager, curlCommand))

		// 	We'll first try to resolve the kind version and node image from our static map, if we can't find
		// 	it (ex. 1.34 not in our map yet), we'll continue because we have the ability to pull down arbitrary
		// 	versions from the mirror - although it should be noted the sha is required. So sometimes the version is
		// 	just the tag, and sometimes it's the tag with the sha.
		kindVersionConfig, err := GetKindVersionConfig(kubeVersion)
		if err != nil {
			log.Printf("[WARN] Could not find version %s in our static map, using default kind version and the provided k8s version as node image", kubeVersion)

			// Validate the kubeVersion format when not in static map
			if err := validateKubeVersionFormat(kubeVersion); err != nil {
				return err
			}

			kindVersionConfig = &KindConfig{
				KindVersion:            env.KindVersion(),
				NodeImageVersion:       kubeVersion,
				UseNewContainerdConfig: true,
			}
		}

		// we select if we need to use the new containerd registry config format based on the kubernetes version
		// and the flag stored in kindVersionConfig
		kindFlags.NewContainerdRegistryConfig = kindVersionConfig.UseNewContainerdConfig
		kindConfig, err := generateKindConfig(kindClusterConfig, kindFlags)
		if err != nil {
			return fmt.Errorf("could not generate kind cluster config: %w", err)
		}

		kindInstall, err := InstallKindBinary(env, vm, kindVersionConfig.KindVersion, opts...)
		if err != nil {
			return err
		}

		clusterConfigFilePath := fmt.Sprintf("/tmp/kind-cluster-%s.yaml", name)
		clusterConfig, err := vm.OS.FileManager().CopyInlineFile(
			pulumi.String(kindConfig),
			clusterConfigFilePath, opts...)
		if err != nil {
			return err
		}

		if kindVersionConfig.UseNewContainerdConfig {
			// pre-create the folder architecture for containerd overrides
			// The kind cluster configuration has a mount point
			// - host: /tmp/certs.d/
			// - cluster: /etc/containerd/certs.d/
			// this way we can place configuration override files in that 'certs.d' folder

			containerdOverrideDir := "/tmp/certs.d/docker.io/"
			containerdDir, err := vm.OS.FileManager().CreateDirectory(containerdOverrideDir, false, opts...)
			if err != nil {
				return err
			}

			// for kind versions using containerd > 2.x we need a different config format
			// kind started using containerd 2.x in kind v0.27.0
			containerdConfigFilePath := containerdOverrideDir + "hosts.toml"
			_, err = vm.OS.FileManager().CopyInlineFile(
				pulumi.String(containerdDockerioHostConfig),
				containerdConfigFilePath,
				utils.MergeOptions(opts, utils.PulumiDependsOn(containerdDir))...,
			)
			if err != nil {
				return err
			}
		}

		/*
			The internal mirror should be able to pull arbitrary kubernetes images but the sha is required
			with the tag. We also support the user supplying the url (in case we want to host
			kubernetes rc candidates in some registry, etc)
		*/
		var nodeImage string
		if env.KubeNodeURL() != "" {
			nodeImage = env.KubeNodeURL()
		} else {
			nodeImage = fmt.Sprintf("%s/%s:%s", env.InternalDockerhubMirror(), kindNodeImageName, kindVersionConfig.NodeImageVersion)
		}
		log.Printf("[INFO] Resolved node image url: %s", nodeImage)
		createCluster, err := runner.Command(
			commonEnvironment.CommonNamer().ResourceName("kind-create-cluster"),
			&command.Args{
				Create:   pulumi.Sprintf("kind create cluster --name %s --config %s --image %s --wait %s", kindClusterName, clusterConfigFilePath, nodeImage, kindReadinessWait),
				Delete:   pulumi.Sprintf("kind delete cluster --name %s", kindClusterName),
				Triggers: pulumi.Array{pulumi.String(kindConfig)},
			},
			utils.MergeOptions(opts, utils.PulumiDependsOn(clusterConfig, kindInstall), pulumi.DeleteBeforeReplace(true))...,
		)
		if err != nil {
			return err
		}

		kubeConfigCmd, err := runner.Command(
			commonEnvironment.CommonNamer().ResourceName("kind-kubeconfig"),
			&command.Args{
				Create: pulumi.Sprintf("kind get kubeconfig --name %s", kindClusterName),
			},
			utils.MergeOptions(opts, utils.PulumiDependsOn(createCluster))...,
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

func NewLocalKindCluster(env config.Env, name string, kubeVersion string, opts ...pulumi.ResourceOption) (*Cluster, error) {
	return components.NewComponent(env, name, func(clusterComp *Cluster) error {
		kindClusterName := env.CommonNamer().DisplayName(49) // We can have some issues if the name is longer than 50 characters
		opts = utils.MergeOptions[pulumi.ResourceOption](opts, pulumi.Parent(clusterComp))
		commonEnvironment := env

		kindVersionConfig, err := GetKindVersionConfig(kubeVersion)
		if err != nil {
			return err
		}

		kindConfigFlags := KindConfigFlags{
			NewContainerdRegistryConfig: kindVersionConfig.UseNewContainerdConfig,
		}
		kindConfig, err := generateKindConfig(kindClusterConfig, kindConfigFlags)
		if err != nil {
			return fmt.Errorf("could not generate kind cluster config: %w", err)
		}

		runner := command.NewLocalRunner(env, command.LocalRunnerArgs{
			// User:      user.Username,
			OSCommand: command.NewUnixOSCommand(),
		})

		clusterConfigFilePath := fmt.Sprintf("/tmp/kind-cluster-%s.yaml", name)
		clusterConfig, err := runner.Command("kind-config", &command.Args{
			Create: pulumi.Sprintf("cat - | tee %s > /dev/null", clusterConfigFilePath),
			Delete: pulumi.Sprintf("rm -f %s", clusterConfigFilePath),
			Stdin:  pulumi.String(kindConfig),
		}, opts...)
		if err != nil {
			return err
		}

		nodeImage := fmt.Sprintf("%s/%s:%s", env.InternalDockerhubMirror(), kindNodeImageName, kindVersionConfig.NodeImageVersion)
		createCluster, err := runner.Command(
			commonEnvironment.CommonNamer().ResourceName("kind-create-cluster"),
			&command.Args{
				Create:   pulumi.Sprintf("kind create cluster --name %s --config %s --image %s --wait %s", kindClusterName, clusterConfigFilePath, nodeImage, kindReadinessWait),
				Delete:   pulumi.Sprintf("kind delete cluster --name %s", kindClusterName),
				Triggers: pulumi.Array{pulumi.String(kindConfig)},
			},
			utils.MergeOptions(opts, utils.PulumiDependsOn(clusterConfig), pulumi.DeleteBeforeReplace(true))...,
		)
		if err != nil {
			return err
		}

		kubeConfigCmd, err := runner.Command(
			commonEnvironment.CommonNamer().ResourceName("kind-kubeconfig"),
			&command.Args{
				Create: pulumi.Sprintf("kind get kubeconfig --name %s", kindClusterName),
			},
			utils.MergeOptions(opts, utils.PulumiDependsOn(createCluster))...,
		)
		if err != nil {
			return err
		}

		clusterComp.KubeConfig = kubeConfigCmd.StdoutOutput()
		clusterComp.ClusterName = kindClusterName.ToStringOutput()

		return nil
	}, opts...)
}

func InstallKindBinary(env config.Env, vm *remote.Host, kindVersion string, opts ...pulumi.ResourceOption) (pulumi.Resource, error) {
	kindArch := vm.OS.Descriptor().Architecture
	if kindArch == os.AMD64Arch {
		kindArch = "amd64"
	}
	return vm.OS.Runner().Command(
		env.CommonNamer().ResourceName("kind-install"),
		&command.Args{
			Create: pulumi.Sprintf(`curl --retry 10 -fsSLo ./kind "https://kind.sigs.k8s.io/dl/%s/kind-linux-%s" && sudo install kind /usr/local/bin/kind`, kindVersion, kindArch),
		},
		opts...,
	)
}
