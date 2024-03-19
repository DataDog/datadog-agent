// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ecs contains the definition of the AWS ECS environment.
package eks

import (
	"fmt"

	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	dogstatsdstandalone "github.com/DataDog/test-infra-definitions/components/datadog/dogstatsd-standalone"
	"github.com/DataDog/test-infra-definitions/components/datadog/ecsagentparams"
	fakeintakeComp "github.com/DataDog/test-infra-definitions/components/datadog/fakeintake"
	kubeComp "github.com/DataDog/test-infra-definitions/components/kubernetes"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	localEks "github.com/DataDog/test-infra-definitions/resources/aws/eks"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	awsEks "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/eks"
	awsIam "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-eks/sdk/v2/go/eks"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// eksNodeSelector is the label used to select the node group for tracegen
const eksNodeSelector = "eks.amazonaws.com/nodegroup"

// ProvisionerParams contains all the parameters needed to create the environment
type ProvisionerParams struct {
	agentOptions      []ecsagentparams.Option
	fakeintakeOptions []fakeintake.Option

	extraConfigParams        runner.ConfigMap
	eksLinuxNodeGroup        bool
	eksLinuxARMNodeGroup     bool
	eksBottlerocketNodeGroup bool
	eksWindowsNodeGroup      bool

	deployDogstatsd  bool
	workloadAppFuncs []WorkloadAppFunc
}

func newProvisionerParams() *ProvisionerParams {
	// We use nil arrays to decide if we should create or not
	return &ProvisionerParams{
		agentOptions:      []ecsagentparams.Option{},
		fakeintakeOptions: []fakeintake.Option{},

		extraConfigParams:        runner.ConfigMap{},
		eksLinuxNodeGroup:        false,
		eksLinuxARMNodeGroup:     false,
		eksBottlerocketNodeGroup: false,
		eksWindowsNodeGroup:      false,
		deployDogstatsd:          false,
	}
}

// GetProvisionerParams return ProvisionerParams from options opts setup
func GetProvisionerParams(opts ...ProvisionerOption) *ProvisionerParams {
	params := newProvisionerParams()
	err := optional.ApplyOptions(params, opts)
	if err != nil {
		panic(fmt.Errorf("unable to apply ProvisionerOption, err: %w", err))
	}
	return params
}

// ProvisionerOption is a function that modifies the ProvisionerParams
type ProvisionerOption func(*ProvisionerParams) error

// WithAgentOptions sets the options for the Docker Agent
func WithAgentOptions(opts ...ecsagentparams.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.agentOptions = append(params.agentOptions, opts...)
		return nil
	}
}

// WithExtraConfigParams sets the extra config params for the environment
func WithExtraConfigParams(configMap runner.ConfigMap) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.extraConfigParams = configMap
		return nil
	}
}

// WithFakeIntakeOptions sets the options for the FakeIntake
func WithFakeIntakeOptions(opts ...fakeintake.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.fakeintakeOptions = append(params.fakeintakeOptions, opts...)
		return nil
	}
}

// WithEKSLinuxNodeGroup enable aws/ecs/linuxECSOptimizedNodeGroup
func WithEKSLinuxNodeGroup() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.eksLinuxNodeGroup = true
		return nil
	}
}

// WithEKSLinuxARMNodeGroup enable aws/ecs/linuxECSOptimizedNodeGroup
func WithEKSLinuxARMNodeGroup() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.eksLinuxARMNodeGroup = true
		return nil
	}
}

// WithEKSBottlerocketNodeGroup enable aws/ecs/linuxECSOptimizedNodeGroup
func WithEKSBottlerocketNodeGroup() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.eksBottlerocketNodeGroup = true
		return nil
	}
}

// WithECSWindowsNodeGroup enable aws/ecs/windowsLTSCNodeGroup
func WithEKSWindowsNodeGroup() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.eksWindowsNodeGroup = true
		return nil
	}
}

// WithDeployDogstatsd deploy standalone dogstatd
func WithDeployDogstatsd() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.deployDogstatsd = true
		return nil
	}
}

// WithoutFakeIntake deactivates the creation of the FakeIntake
func WithoutFakeIntake() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.fakeintakeOptions = nil
		return nil
	}
}

// WithoutAgent deactivates the creation of the Docker Agent
func WithoutAgent() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.agentOptions = nil
		return nil
	}
}

// WorkloadAppFunc is a function that deploys a workload app to a kube provider
type WorkloadAppFunc func(e config.CommonEnvironment, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error)

// WithWorkloadApp adds a workload app to the environment
func WithWorkloadApp(appFunc WorkloadAppFunc) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.workloadAppFuncs = append(params.workloadAppFuncs, appFunc)
		return nil
	}
}

// Run deploys a EKS environment given a pulumi.Context
func Run(ctx *pulumi.Context, env *environments.EKS, params *ProvisionerParams) error {
	var awsEnv aws.Environment
	var err error
	if env.AwsEnvironment != nil {
		awsEnv = *env.AwsEnvironment
	} else {
		awsEnv, err = aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}
	}

	clusterComp, err := components.NewComponent(*awsEnv.CommonEnvironment, awsEnv.Namer.ResourceName("eks"), func(comp *kubeComp.Cluster) error {
		// Create Cluster SG
		clusterSG, err := ec2.NewSecurityGroup(ctx, awsEnv.Namer.ResourceName("eks-sg"), &ec2.SecurityGroupArgs{
			NamePrefix:  awsEnv.CommonNamer.DisplayName(255, pulumi.String("eks-sg")),
			Description: pulumi.StringPtr("EKS Cluster sg for stack: " + ctx.Stack()),
			Ingress: ec2.SecurityGroupIngressArray{
				ec2.SecurityGroupIngressArgs{
					SecurityGroups: pulumi.ToStringArray(awsEnv.EKSAllowedInboundSecurityGroups()),
					PrefixListIds:  pulumi.ToStringArray(awsEnv.EKSAllowedInboundPrefixLists()),
					ToPort:         pulumi.Int(22),
					FromPort:       pulumi.Int(22),
					Protocol:       pulumi.String("tcp"),
				},
				ec2.SecurityGroupIngressArgs{
					SecurityGroups: pulumi.ToStringArray(awsEnv.EKSAllowedInboundSecurityGroups()),
					PrefixListIds:  pulumi.ToStringArray(awsEnv.EKSAllowedInboundPrefixLists()),
					ToPort:         pulumi.Int(443),
					FromPort:       pulumi.Int(443),
					Protocol:       pulumi.String("tcp"),
				},
			},
			VpcId: pulumi.StringPtr(awsEnv.DefaultVPCID()),
		}, awsEnv.WithProviders(config.ProviderAWS))
		if err != nil {
			return err
		}

		// Cluster role
		clusterRole, err := localEks.GetClusterRole(awsEnv, "eks-cluster-role")
		if err != nil {
			return err
		}

		// IAM Node role
		linuxNodeRole, err := localEks.GetNodeRole(awsEnv, "eks-linux-node-role")
		if err != nil {
			return err
		}

		windowsNodeRole, err := localEks.GetNodeRole(awsEnv, "eks-windows-node-role")
		if err != nil {
			return err
		}

		// Fargate Configuration
		var fargateProfile pulumi.Input
		if fargateNamespace := awsEnv.EKSFargateNamespace(); fargateNamespace != "" {
			fargateProfile = pulumi.Any(
				eks.FargateProfile{
					Selectors: []awsEks.FargateProfileSelector{
						{
							Namespace: fargateNamespace,
						},
					},
				},
			)
		}

		// Create an EKS cluster with the default configuration.
		cluster, err := eks.NewCluster(ctx, awsEnv.Namer.ResourceName("eks"), &eks.ClusterArgs{
			Name:                         awsEnv.CommonNamer.DisplayName(100),
			Version:                      pulumi.StringPtr(awsEnv.KubernetesVersion()),
			EndpointPrivateAccess:        pulumi.BoolPtr(true),
			EndpointPublicAccess:         pulumi.BoolPtr(false),
			Fargate:                      fargateProfile,
			ClusterSecurityGroup:         clusterSG,
			NodeAssociatePublicIpAddress: pulumi.BoolRef(false),
			PrivateSubnetIds:             awsEnv.RandomSubnets(),
			VpcId:                        pulumi.StringPtr(awsEnv.DefaultVPCID()),
			SkipDefaultNodeGroup:         pulumi.BoolRef(true),
			// The content of the aws-auth map is the merge of `InstanceRoles` and `RoleMappings`.
			// For managed node groups, we push the value in `InstanceRoles`.
			// For unmanaged node groups, we push the value in `RoleMappings`
			RoleMappings: eks.RoleMappingArray{
				eks.RoleMappingArgs{
					Groups:   pulumi.ToStringArray([]string{"system:bootstrappers", "system:nodes", "eks:kube-proxy-windows"}),
					Username: pulumi.String("system:node:{{EC2PrivateDNSName}}"),
					RoleArn:  windowsNodeRole.Arn,
				},
			},
			InstanceRoles: awsIam.RoleArray{
				linuxNodeRole,
			},
			ServiceRole: clusterRole,
		}, pulumi.Timeouts(&pulumi.CustomTimeouts{
			Create: "30m",
			Update: "30m",
			Delete: "30m",
		}), awsEnv.WithProviders(config.ProviderEKS, config.ProviderAWS))
		if err != nil {
			return err
		}

		// Filling Kubernetes component from EKS cluster
		comp.ClusterName = cluster.EksCluster.Name()
		comp.KubeConfig = cluster.KubeconfigJson

		nodeGroups := make([]pulumi.Resource, 0)
		// Create managed node groups
		if params.eksLinuxNodeGroup {
			ng, err := localEks.NewLinuxNodeGroup(awsEnv, cluster, linuxNodeRole)
			if err != nil {
				return err
			}
			nodeGroups = append(nodeGroups, ng)
		}

		if params.eksLinuxARMNodeGroup {
			ng, err := localEks.NewLinuxARMNodeGroup(awsEnv, cluster, linuxNodeRole)
			if err != nil {
				return err
			}
			nodeGroups = append(nodeGroups, ng)
		}

		if params.eksBottlerocketNodeGroup {
			ng, err := localEks.NewBottlerocketNodeGroup(awsEnv, cluster, linuxNodeRole)
			if err != nil {
				return err
			}
			nodeGroups = append(nodeGroups, ng)
		}

		// Create unmanaged node groups
		if params.eksWindowsNodeGroup {
			_, err := localEks.NewWindowsUnmanagedNodeGroup(awsEnv, cluster, windowsNodeRole)
			if err != nil {
				return err
			}
		}

		// Building Kubernetes provider
		eksKubeProvider, err := kubernetes.NewProvider(awsEnv.Ctx, awsEnv.Namer.ResourceName("k8s-provider"), &kubernetes.ProviderArgs{
			EnableServerSideApply: pulumi.BoolPtr(true),
			Kubeconfig:            utils.KubeconfigToJSON(cluster.Kubeconfig),
		}, awsEnv.WithProviders(config.ProviderAWS), pulumi.DependsOn(nodeGroups))
		if err != nil {
			return err
		}

		// Applying necessary Windows configuration if Windows nodes
		if params.eksWindowsNodeGroup {
			_, err := corev1.NewConfigMapPatch(awsEnv.Ctx, awsEnv.Namer.ResourceName("eks-cni-cm"), &corev1.ConfigMapPatchArgs{
				Metadata: metav1.ObjectMetaPatchArgs{
					Namespace: pulumi.String("kube-system"),
					Name:      pulumi.String("amazon-vpc-cni"),
					Annotations: pulumi.StringMap{
						"pulumi.com/patchForce": pulumi.String("true"),
					},
				},
				Data: pulumi.StringMap{
					"enable-windows-ipam": pulumi.String("true"),
				},
			}, pulumi.Provider(eksKubeProvider))
			if err != nil {
				return err
			}
		}

		var fakeIntake *fakeintakeComp.Fakeintake
		if params.fakeintakeOptions != nil {
			fakeIntakeOptions := []fakeintake.Option{
				fakeintake.WithCPU(1024),
				fakeintake.WithMemory(6144),
			}
			if awsEnv.GetCommonEnvironment().InfraShouldDeployFakeintakeWithLB() {
				fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithLoadBalancer())
			}

			if fakeIntake, err = fakeintake.NewECSFargateInstance(awsEnv, "ecs", fakeIntakeOptions...); err != nil {
				return err
			}
			if err := fakeIntake.Export(awsEnv.Ctx, &env.FakeIntake.FakeintakeOutput); err != nil {
				return err
			}
		}

		// Deploy the agent
		if params.agentOptions != nil {
			helmComponent, err := agent.NewHelmInstallation(*awsEnv.CommonEnvironment, agent.HelmInstallationArgs{
				KubeProvider:  eksKubeProvider,
				Namespace:     "datadog",
				Fakeintake:    fakeIntake,
				DeployWindows: awsEnv.EKSWindowsNodeGroup(),
			}, nil)
			if err != nil {
				return err
			}

			ctx.Export("agent-linux-helm-install-name", helmComponent.LinuxHelmReleaseName)
			ctx.Export("agent-linux-helm-install-status", helmComponent.LinuxHelmReleaseStatus)
			if awsEnv.EKSWindowsNodeGroup() {
				ctx.Export("agent-windows-helm-install-name", helmComponent.WindowsHelmReleaseName)
				ctx.Export("agent-windows-helm-install-status", helmComponent.WindowsHelmReleaseStatus)
			}
		}

		// Deploy standalone dogstatsd
		if params.deployDogstatsd {
			if _, err := dogstatsdstandalone.K8sAppDefinition(*awsEnv.CommonEnvironment, eksKubeProvider, "dogstatsd-standalone", fakeIntake, true, ""); err != nil {
				return err
			}
		}

		// Deploy workloads
		for _, appFunc := range params.workloadAppFuncs {
			_, err := appFunc(*awsEnv.CommonEnvironment, eksKubeProvider)
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	return clusterComp.Export(ctx, nil)
}
