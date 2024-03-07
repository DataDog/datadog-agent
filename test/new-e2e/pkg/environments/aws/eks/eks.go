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
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/cpustress"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/dogstatsd"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/mutatedbyadmissioncontroller"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/nginx"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/prometheus"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/redis"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/tracegen"
	dogstatsdstandalone "github.com/DataDog/test-infra-definitions/components/datadog/dogstatsd-standalone"
	fakeintakeComp "github.com/DataDog/test-infra-definitions/components/datadog/fakeintake"
	kubeComp "github.com/DataDog/test-infra-definitions/components/kubernetes"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	localEks "github.com/DataDog/test-infra-definitions/resources/aws/eks"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"

	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ec2"
	awsEks "github.com/pulumi/pulumi-aws/sdk/v5/go/aws/eks"
	awsIam "github.com/pulumi/pulumi-aws/sdk/v5/go/aws/iam"
	"github.com/pulumi/pulumi-eks/sdk/go/eks"
	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// eksNodeSelector is the label used to select the node group for tracegen
const eksNodeSelector = "eks.amazonaws.com/nodegroup"

// ProvisionerParams contains all the parameters needed to create the environment
type ProvisionerParams struct {
}

func newProvisionerParams() *ProvisionerParams {
	// We use nil arrays to decide if we should create or not
	return &ProvisionerParams{}
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
		var cgroupV1Ng pulumi.StringOutput
		var cgroupV2Ng pulumi.StringOutput
		// Create managed node groups
		if awsEnv.EKSLinuxNodeGroup() {
			ng, err := localEks.NewLinuxNodeGroup(awsEnv, cluster, linuxNodeRole)
			if err != nil {
				return err
			}
			nodeGroups = append(nodeGroups, ng)
			// The default AMI used for Amazon Linux 2 is using cgroupv1
			cgroupV1Ng = ng.NodeGroup.NodeGroupName()
		}

		if awsEnv.EKSLinuxARMNodeGroup() {
			ng, err := localEks.NewLinuxARMNodeGroup(awsEnv, cluster, linuxNodeRole)
			if err != nil {
				return err
			}
			nodeGroups = append(nodeGroups, ng)
			cgroupV1Ng = ng.NodeGroup.NodeGroupName()
		}

		if awsEnv.EKSBottlerocketNodeGroup() {
			ng, err := localEks.NewBottlerocketNodeGroup(awsEnv, cluster, linuxNodeRole)
			if err != nil {
				return err
			}
			nodeGroups = append(nodeGroups, ng)
			// Bottlerocket uses cgroupv2
			cgroupV2Ng = ng.NodeGroup.NodeGroupName()
		}

		// Create unmanaged node groups
		if awsEnv.EKSWindowsNodeGroup() {
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
		if awsEnv.EKSWindowsNodeGroup() {
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

		var dependsOnCrd pulumi.ResourceOption

		var fakeIntake *fakeintakeComp.Fakeintake
		if awsEnv.GetCommonEnvironment().AgentUseFakeintake() {
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
			if err := fakeIntake.Export(awsEnv.Ctx, nil); err != nil {
				return err
			}
		}

		// Deploy the agent
		if awsEnv.AgentDeploy() {
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

			dependsOnCrd = utils.PulumiDependsOn(helmComponent)
		}

		// Deploy standalone dogstatsd
		if awsEnv.DogstatsdDeploy() {
			if _, err := dogstatsdstandalone.K8sAppDefinition(*awsEnv.CommonEnvironment, eksKubeProvider, "dogstatsd-standalone", fakeIntake, true, ""); err != nil {
				return err
			}
		}

		// Deploy testing workload
		if awsEnv.TestingWorkloadDeploy() {
			if _, err := nginx.K8sAppDefinition(*awsEnv.CommonEnvironment, eksKubeProvider, "workload-nginx", dependsOnCrd); err != nil {
				return err
			}

			if _, err := redis.K8sAppDefinition(*awsEnv.CommonEnvironment, eksKubeProvider, "workload-redis", dependsOnCrd); err != nil {
				return err
			}

			if _, err := cpustress.K8sAppDefinition(*awsEnv.CommonEnvironment, eksKubeProvider, "workload-cpustress"); err != nil {
				return err
			}

			// dogstatsd clients that report to the Agent
			if _, err := dogstatsd.K8sAppDefinition(*awsEnv.CommonEnvironment, eksKubeProvider, "workload-dogstatsd", 8125, "/var/run/datadog/dsd.socket"); err != nil {
				return err
			}

			// dogstatsd clients that report to the dogstatsd standalone deployment
			if _, err := dogstatsd.K8sAppDefinition(*awsEnv.CommonEnvironment, eksKubeProvider, "workload-dogstatsd-standalone", dogstatsdstandalone.HostPort, dogstatsdstandalone.Socket); err != nil {
				return err
			}

			if _, err := tracegen.K8sAppDefinition(*awsEnv.CommonEnvironment, eksKubeProvider, "workload-tracegen-cgroupv1", pulumi.StringMap{eksNodeSelector: cgroupV1Ng}); err != nil {
				return err
			}

			if _, err := tracegen.K8sAppDefinition(*awsEnv.CommonEnvironment, eksKubeProvider, "workload-tracegen-cgroupv2", pulumi.StringMap{eksNodeSelector: cgroupV2Ng}); err != nil {
				return err
			}

			if _, err := prometheus.K8sAppDefinition(*awsEnv.CommonEnvironment, eksKubeProvider, "workload-prometheus"); err != nil {
				return err
			}

			if _, err := mutatedbyadmissioncontroller.K8sAppDefinition(*awsEnv.CommonEnvironment, eksKubeProvider, "workload-mutated"); err != nil {
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
