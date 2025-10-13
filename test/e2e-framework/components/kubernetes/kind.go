package kubernetes

import (
	_ "embed"
	"fmt"
	"regexp"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components"
	"github.com/DataDog/test-infra-definitions/components/command"
	"github.com/DataDog/test-infra-definitions/components/docker"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/components/remote"
)

const (
	kindReadinessWait = "60s"
	kindNodeImageName = "kindest/node"
)

//go:embed kind-cluster.yaml
var kindClusterConfig string

// Install Kind on a Linux virtual machine.
func NewKindCluster(env config.Env, vm *remote.Host, name string, kubeVersion string, opts ...pulumi.ResourceOption) (*Cluster, error) {
	return NewKindClusterWithConfig(env, vm, name, kubeVersion, kindClusterConfig, opts...)
}

func NewKindClusterWithConfig(env config.Env, vm *remote.Host, name string, kubeVersion, kindConfig string, opts ...pulumi.ResourceOption) (*Cluster, error) {
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

		kindVersionConfig, err := GetKindVersionConfig(kubeVersion)
		if err != nil {
			return err
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

		nodeImage := fmt.Sprintf("%s/%s:%s", env.InternalDockerhubMirror(), kindNodeImageName, kindVersionConfig.NodeImageVersion)
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

		runner := command.NewLocalRunner(env, command.LocalRunnerArgs{
			// User:      user.Username,
			OSCommand: command.NewUnixOSCommand(),
		})

		clusterConfigFilePath := fmt.Sprintf("/tmp/kind-cluster-%s.yaml", name)
		clusterConfig, err := runner.Command("kind-config", &command.Args{
			Create: pulumi.Sprintf("cat - | tee %s > /dev/null", clusterConfigFilePath),
			Delete: pulumi.Sprintf("rm -f %s", clusterConfigFilePath),
			Stdin:  pulumi.String(kindClusterConfig),
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
				Triggers: pulumi.Array{pulumi.String(kindClusterConfig)},
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
