// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package awskubernetes contains the provisioner for the Kubernetes based environments
package awskubernetes

import (
	"context"
	"fmt"

	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	dogstatsdstandalone "github.com/DataDog/test-infra-definitions/components/datadog/dogstatsd-standalone"
	fakeintakeComp "github.com/DataDog/test-infra-definitions/components/datadog/fakeintake"
	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/test-infra-definitions/components/kubernetes"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	localEks "github.com/DataDog/test-infra-definitions/resources/aws/eks"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	awsEks "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/eks"
	awsIam "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-eks/sdk/v2/go/eks"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func eksDiagnoseFunc(ctx context.Context, stackName string) (string, error) {
	dumpResult, err := dumpEKSClusterState(ctx, stackName)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Dumping EKS cluster state:\n%s", dumpResult), nil
}

// EKSProvisioner creates a new provisioner
func EKSProvisioner(opts ...ProvisionerOption) e2e.TypedProvisioner[environments.AwsKubernetes] {
	// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
	// and it's easy to forget about it, leading to hard to debug issues.
	params := newProvisionerParams()
	_ = optional.ApplyOptions(params, opts)

	provisioner := e2e.NewTypedPulumiProvisioner(provisionerBaseID+params.name, func(ctx *pulumi.Context, env *environments.AwsKubernetes) error {
		// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
		// and it's easy to forget about it, leading to hard to debug issues.
		params := newProvisionerParams()
		_ = optional.ApplyOptions(params, opts)

		return EKSRunFunc(ctx, env, params)
	}, params.extraConfigParams)

	provisioner.SetDiagnoseFunc(eksDiagnoseFunc)

	return provisioner
}

// EKSRunFunc deploys a EKS environment given a pulumi.Context
func EKSRunFunc(ctx *pulumi.Context, env *environments.AwsKubernetes, params *ProvisionerParams) error {
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

	clusterComp, err := components.NewComponent(&awsEnv, awsEnv.Namer.ResourceName("eks"), func(comp *kubeComp.Cluster) error {
		// Create Cluster SG
		clusterSG, err := ec2.NewSecurityGroup(ctx, awsEnv.Namer.ResourceName("eks-sg"), &ec2.SecurityGroupArgs{
			NamePrefix:  awsEnv.CommonNamer().DisplayName(255, pulumi.String("eks-sg")),
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
			Name:                         awsEnv.CommonNamer().DisplayName(100),
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

		initOnly, err := runner.GetProfile().ParamStore().GetBoolWithDefault(parameters.InitOnly, false)
		if err != nil {
			return err
		}
		if initOnly {
			return nil
		}

		// Building Kubernetes provider
		eksKubeProvider, err := kubernetes.NewProvider(awsEnv.Ctx(), awsEnv.Namer.ResourceName("k8s-provider"), &kubernetes.ProviderArgs{
			Kubeconfig:            cluster.KubeconfigJson,
			EnableServerSideApply: pulumi.BoolPtr(true),
			DeleteUnreachable:     pulumi.BoolPtr(true),
		}, awsEnv.WithProviders(config.ProviderAWS))
		if err != nil {
			return err
		}

		// Filling Kubernetes component from EKS cluster
		comp.ClusterName = cluster.EksCluster.Name()
		comp.KubeConfig = cluster.KubeconfigJson
		comp.KubeProvider = eksKubeProvider

		// Create configuration for POD subnets if any
		workloadDeps := make([]pulumi.Resource, 0)
		if podSubnets := awsEnv.EKSPODSubnets(); len(podSubnets) > 0 {
			eniConfigs, err := localEks.NewENIConfigs(awsEnv, podSubnets, awsEnv.DefaultSecurityGroups(), pulumi.Provider(eksKubeProvider))
			if err != nil {
				return err
			}

			// Setting AWS_VPC_K8S_CNI_CUSTOM_NETWORK_CFG is mandatory for EKS CNI to work with ENIConfig CRD
			dsPatch, err := appsv1.NewDaemonSetPatch(awsEnv.Ctx(), awsEnv.Namer.ResourceName("eks-custom-network"), &appsv1.DaemonSetPatchArgs{
				Metadata: metav1.ObjectMetaPatchArgs{
					Namespace: pulumi.String("kube-system"),
					Name:      pulumi.String("aws-node"),
					Annotations: pulumi.StringMap{
						"pulumi.com/patchForce": pulumi.String("true"),
					},
				},
				Spec: appsv1.DaemonSetSpecPatchArgs{
					Template: corev1.PodTemplateSpecPatchArgs{
						Spec: corev1.PodSpecPatchArgs{
							Containers: corev1.ContainerPatchArray{
								corev1.ContainerPatchArgs{
									Name: pulumi.StringPtr("aws-node"),
									Env: corev1.EnvVarPatchArray{
										corev1.EnvVarPatchArgs{
											Name:  pulumi.String("AWS_VPC_K8S_CNI_CUSTOM_NETWORK_CFG"),
											Value: pulumi.String("true"),
										},
										corev1.EnvVarPatchArgs{
											Name:  pulumi.String("ENI_CONFIG_LABEL_DEF"),
											Value: pulumi.String("topology.kubernetes.io/zone"),
										},
										corev1.EnvVarPatchArgs{
											Name:  pulumi.String("ENABLE_PREFIX_DELEGATION"),
											Value: pulumi.String("true"),
										},
										corev1.EnvVarPatchArgs{
											Name:  pulumi.String("WARM_IP_TARGET"),
											Value: pulumi.String("1"),
										},
										corev1.EnvVarPatchArgs{
											Name:  pulumi.String("MINIMUM_IP_TARGET"),
											Value: pulumi.String("1"),
										},
									},
								},
							},
						},
					},
				},
			}, pulumi.Provider(eksKubeProvider), utils.PulumiDependsOn(eniConfigs))
			if err != nil {
				return err
			}

			workloadDeps = append(workloadDeps, eniConfigs, dsPatch)
		}

		// Create managed node groups
		if params.eksLinuxNodeGroup {
			ng, err := localEks.NewLinuxNodeGroup(awsEnv, cluster, linuxNodeRole)
			if err != nil {
				return err
			}
			workloadDeps = append(workloadDeps, ng)
		}

		if params.eksLinuxARMNodeGroup {
			ng, err := localEks.NewLinuxARMNodeGroup(awsEnv, cluster, linuxNodeRole)
			if err != nil {
				return err
			}
			workloadDeps = append(workloadDeps, ng)
		}

		if params.eksBottlerocketNodeGroup {
			ng, err := localEks.NewBottlerocketNodeGroup(awsEnv, cluster, linuxNodeRole)
			if err != nil {
				return err
			}
			workloadDeps = append(workloadDeps, ng)
		}

		// Create unmanaged node groups
		if params.eksWindowsNodeGroup {
			_, err := localEks.NewWindowsNodeGroup(awsEnv, cluster, windowsNodeRole)
			if err != nil {
				return err
			}
		}

		// Applying necessary Windows configuration if Windows nodes
		// Custom networking is not available for Windows nodes, using normal subnets IPs
		if params.eksWindowsNodeGroup {
			_, err := corev1.NewConfigMapPatch(awsEnv.Ctx(), awsEnv.Namer.ResourceName("eks-cni-cm"), &corev1.ConfigMapPatchArgs{
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
			if err := fakeIntake.Export(awsEnv.Ctx(), &env.FakeIntake.FakeintakeOutput); err != nil {
				return err
			}
		}

		// Deploy the agent
		dependsOnSetup := utils.PulumiDependsOn(workloadDeps...)
		if params.agentOptions != nil {
			paramsAgent, err := kubernetesagentparams.NewParams(&awsEnv, params.agentOptions...)
			if err != nil {
				return err
			}

			helmComponent, err := agent.NewHelmInstallation(&awsEnv, agent.HelmInstallationArgs{
				KubeProvider:  eksKubeProvider,
				Namespace:     "datadog",
				ValuesYAML:    paramsAgent.HelmValues,
				Fakeintake:    fakeIntake,
				DeployWindows: params.eksWindowsNodeGroup,
			}, dependsOnSetup)
			if err != nil {
				return err
			}
			env.Agent = nil

			ctx.Export("agent-linux-helm-install-name", helmComponent.LinuxHelmReleaseName)
			ctx.Export("agent-linux-helm-install-status", helmComponent.LinuxHelmReleaseStatus)
			if params.eksWindowsNodeGroup {
				ctx.Export("agent-windows-helm-install-name", helmComponent.WindowsHelmReleaseName)
				ctx.Export("agent-windows-helm-install-status", helmComponent.WindowsHelmReleaseStatus)
			}
		}

		// Deploy standalone dogstatsd
		if params.deployDogstatsd {
			if _, err := dogstatsdstandalone.K8sAppDefinition(&awsEnv, eksKubeProvider, "dogstatsd-standalone", fakeIntake, true, ""); err != nil {
				return err
			}
		}

		// Deploy workloads
		for _, appFunc := range params.workloadAppFuncs {
			_, err := appFunc(&awsEnv, eksKubeProvider)
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	return clusterComp.Export(ctx, &env.KubernetesCluster.ClusterOutput)
}
