// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package eks

import (
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	awsEks "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/eks"
	awsIam "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-eks/sdk/v3/go/eks"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv4 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v4"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	v1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/rbac/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/samber/lo"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	kubecomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	localEks "github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws/eks"
)

func NewCluster(e aws.Environment, name string, opts ...Option) (*kubecomp.Cluster, error) {
	params, err := NewParams(opts...)
	if err != nil {
		return nil, err
	}
	return components.NewComponent(&e, name, func(comp *kubecomp.Cluster) error {
		// Create Cluster SG
		prefixLists := make([]string, 0, len(e.EKSAllowedInboundManagedPrefixListNames()))
		for _, plName := range e.EKSAllowedInboundManagedPrefixListNames() {
			pl, err := ec2.LookupManagedPrefixList(e.Ctx(), &ec2.LookupManagedPrefixListArgs{
				Name: &plName,
			}, e.WithProvider(config.ProviderAWS))
			if err != nil {
				return err
			}
			if pl != nil {
				prefixLists = append(prefixLists, pl.Id)
			}
		}
		clusterSG, err := ec2.NewSecurityGroup(e.Ctx(), e.Namer.ResourceName("eks-sg"), &ec2.SecurityGroupArgs{
			NamePrefix:  e.CommonNamer().DisplayName(255, pulumi.String("eks-sg")),
			Description: pulumi.StringPtr("EKS Cluster sg for stack: " + e.Ctx().Stack()),
			Ingress: ec2.SecurityGroupIngressArray{
				ec2.SecurityGroupIngressArgs{
					SecurityGroups: pulumi.ToStringArray(e.EKSAllowedInboundSecurityGroups()),
					PrefixListIds:  pulumi.ToStringArray(append(e.EKSAllowedInboundPrefixLists(), prefixLists...)),
					ToPort:         pulumi.Int(22),
					FromPort:       pulumi.Int(22),
					Protocol:       pulumi.String("tcp"),
				},
				ec2.SecurityGroupIngressArgs{
					SecurityGroups: pulumi.ToStringArray(e.EKSAllowedInboundSecurityGroups()),
					PrefixListIds:  pulumi.ToStringArray(append(e.EKSAllowedInboundPrefixLists(), prefixLists...)),
					ToPort:         pulumi.Int(443),
					FromPort:       pulumi.Int(443),
					Protocol:       pulumi.String("tcp"),
				},
			},
			VpcId: pulumi.StringPtr(e.DefaultVPCID()),
		}, e.WithProviders(config.ProviderAWS))
		if err != nil {
			return err
		}

		// Cluster role
		clusterRole, err := localEks.GetClusterRole(e, "eks-cluster-role")
		if err != nil {
			return err
		}

		// IAM Node role
		linuxNodeRole, err := localEks.GetNodeRole(e, "eks-linux-node-role")
		if err != nil {
			return err
		}

		windowsNodeRole, err := localEks.GetNodeRole(e, "eks-windows-node-role")
		if err != nil {
			return err
		}

		// Fargate Configuration
		var fargateProfileSelectors awsEks.FargateProfileSelectorArray
		if fargateNamespace := e.EKSFargateNamespace(); fargateNamespace != "" {
			fargateProfileSelectors = awsEks.FargateProfileSelectorArray{
				awsEks.FargateProfileSelectorArgs{
					Namespace: pulumi.String(fargateNamespace),
				},
			}
		}

		// Create an EKS cluster with the default configuration.
		cluster, err := eks.NewCluster(e.Ctx(), e.Namer.ResourceName("eks"), &eks.ClusterArgs{
			Name:                  e.CommonNamer().DisplayName(100),
			Version:               pulumi.StringPtr(e.KubernetesVersion()),
			EndpointPrivateAccess: pulumi.BoolPtr(true),
			EndpointPublicAccess:  pulumi.BoolPtr(false),
			Fargate: eks.FargateProfileArgs{
				Selectors: append(awsEks.FargateProfileSelectorArray{
					// Put CoreDNS pods on Fargate because this addon needs to be deployed
					// before the node groups are created.
					awsEks.FargateProfileSelectorArgs{
						Namespace: pulumi.String("kube-system"),
					},
					// Automatically schedule on Fargate pods on which the Fargate agent sidecar
					// container injection is requested.
					awsEks.FargateProfileSelectorArgs{
						Labels: pulumi.StringMap{
							"agent.datadoghq.com/sidecar": pulumi.String("fargate"),
						},
						Namespace: pulumi.String("*"),
					},
				}, fargateProfileSelectors...),
				SubnetIds: pulumi.ToStringArray(lo.Map(e.EKSPODSubnets(), func(subnet aws.DDInfraEKSPodSubnets, _ int) string { return subnet.SubnetID })),
			},
			ClusterSecurityGroup:         clusterSG,
			NodeAssociatePublicIpAddress: pulumi.BoolRef(false),
			PrivateSubnetIds:             pulumi.ToStringArray(e.DefaultSubnets()),
			VpcId:                        pulumi.StringPtr(e.DefaultVPCID()),
			SkipDefaultNodeGroup:         pulumi.BoolRef(true),
			InstanceRoles: awsIam.RoleArray{
				linuxNodeRole,
				windowsNodeRole,
			},
			ServiceRole: clusterRole,
			ProviderCredentialOpts: &eks.KubeconfigOptionsArgs{
				ProfileName: pulumi.String(e.Profile()),
			},
			// Add account-admin role mapping to the cluster, which make investigations on cluster created in the CI easier.
			RoleMappings: eks.RoleMappingArray{
				eks.RoleMappingArgs{
					RoleArn: pulumi.String(e.EKSAccountAdminSSORole()),
					Groups: pulumi.StringArray{
						pulumi.String("system:masters"),
					},
				},
				eks.RoleMappingArgs{
					RoleArn: pulumi.String(e.EKSReadOnlySSORole()),
					Groups: pulumi.StringArray{
						pulumi.String("read-only"),
					},
				},
			},
		}, pulumi.Timeouts(&pulumi.CustomTimeouts{
			Create: "30m",
			Update: "30m",
			Delete: "30m",
		}), e.WithProviders(config.ProviderEKS, config.ProviderAWS), pulumi.Parent(comp))
		if err != nil {
			return err
		}

		clusterKubeConfig, err := cluster.GetKubeconfig(e.Ctx(), &eks.ClusterGetKubeconfigArgs{
			ProfileName: pulumi.String(e.Profile()),
		})
		if err != nil {
			return err
		}

		// Building Kubernetes provider
		eksKubeProvider, err := kubernetes.NewProvider(e.Ctx(), e.Namer.ResourceName("k8s-provider"), &kubernetes.ProviderArgs{
			Kubeconfig:            clusterKubeConfig,
			EnableServerSideApply: pulumi.BoolPtr(true),
			DeleteUnreachable:     pulumi.BoolPtr(true),
		}, e.WithProviders(config.ProviderAWS), pulumi.Parent(comp))
		if err != nil {
			return err
		}

		// Filling Kubernetes component from EKS cluster
		comp.ClusterName = cluster.EksCluster.Name()
		comp.KubeConfig = clusterKubeConfig
		comp.KubeProvider = eksKubeProvider

		// Deps for nodes and workloads
		nodeDeps := make([]pulumi.Resource, 0)

		_, err = v1.NewClusterRoleBinding(e.Ctx(), e.Namer.ResourceName("eks-cluster-role-binding-read-only"), &v1.ClusterRoleBindingArgs{
			RoleRef: v1.RoleRefArgs{
				ApiGroup: pulumi.String("rbac.authorization.k8s.io"),
				Kind:     pulumi.String("ClusterRole"),
				Name:     pulumi.String("view"),
			},
			Subjects: v1.SubjectArray{
				v1.SubjectArgs{
					Kind:      pulumi.String("Group"),
					Name:      pulumi.String("read-only"),
					Namespace: pulumi.String(""),
				},
			},
		}, pulumi.Provider(eksKubeProvider), pulumi.Parent(comp))
		if err != nil {
			return err
		}

		// Create configuration for POD subnets if any
		if podSubnets := e.EKSPODSubnets(); len(podSubnets) > 0 {
			eniConfigs, err := localEks.NewENIConfigs(e, podSubnets, append(lo.Map(e.DefaultSecurityGroups(), func(sg string, _ int) pulumi.StringInput { return pulumi.String(sg) }), cluster.EksCluster.VpcConfig().ClusterSecurityGroupId().Elem()), pulumi.Provider(eksKubeProvider), pulumi.Parent(comp))
			if err != nil {
				return err
			}

			// Setting AWS_VPC_K8S_CNI_CUSTOM_NETWORK_CFG is mandatory for EKS CNI to work with ENIConfig CRD
			dsPatch, err := appsv1.NewDaemonSetPatch(e.Ctx(), e.Namer.ResourceName("eks-custom-network"), &appsv1.DaemonSetPatchArgs{
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
			}, pulumi.Provider(eksKubeProvider), utils.PulumiDependsOn(eniConfigs), pulumi.Parent(comp))
			if err != nil {
				return err
			}

			nodeDeps = append(nodeDeps, eniConfigs, dsPatch)
		}

		// Create managed node groups
		if params.LinuxNodeGroup {
			_, err = localEks.NewAL2023LinuxNodeGroup(e, cluster, linuxNodeRole, utils.PulumiDependsOn(nodeDeps...), pulumi.Parent(comp))
			if err != nil {
				return err
			}
		}

		if params.LinuxARMNodeGroup {
			_, err := localEks.NewAL2023LinuxARMNodeGroup(e, cluster, linuxNodeRole, utils.PulumiDependsOn(nodeDeps...), pulumi.Parent(comp))
			if err != nil {
				return err
			}
		}

		if params.BottleRocketNodeGroup {
			_, err := localEks.NewBottlerocketNodeGroup(e, cluster, linuxNodeRole, utils.PulumiDependsOn(nodeDeps...), pulumi.Parent(comp))
			if err != nil {
				return err
			}
		}

		if params.WindowsNodeGroup {
			// Applying necessary Windows configuration if Windows nodes
			// Custom networking is not available for Windows nodes, using normal subnets IPs
			winCNIPatch, err := corev1.NewConfigMapPatch(e.Ctx(), e.Namer.ResourceName("eks-cni-cm"), &corev1.ConfigMapPatchArgs{
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
			}, pulumi.Provider(eksKubeProvider), pulumi.Parent(comp))
			if err != nil {
				return err
			}

			nodeDeps = append(nodeDeps, winCNIPatch)
			_, err = localEks.NewWindowsNodeGroup(e, cluster, windowsNodeRole, utils.PulumiDependsOn(nodeDeps...), pulumi.Parent(comp))
			if err != nil {
				return err
			}
		}

		if params.GPUNodeGroup {
			// Create GPU node group first so the node exists for the device plugin to schedule on
			gpuNodeGroup, err := localEks.NewGPULinuxNodeGroup(e, cluster, linuxNodeRole, params.GPUInstanceType, utils.PulumiDependsOn(nodeDeps...), pulumi.Parent(comp))
			if err != nil {
				return err
			}

			// Deploy NVIDIA device plugin for GPU support AFTER the GPU node group is created
			// The EKS GPU AMI already has NVIDIA drivers pre-installed, so we only need the device plugin
			// We set nvidia.com/gpu.present=true on GPU nodes to mimic NFD (Node Feature Discovery)
			// This allows the device plugin to schedule on GPU nodes using standard GPU labels
			_, err = helmv4.NewChart(e.Ctx(), e.Namer.ResourceName("nvidia-device-plugin"), &helmv4.ChartArgs{
				Chart:     pulumi.String("nvidia-device-plugin"),
				Namespace: pulumi.String("kube-system"),
				RepositoryOpts: helmv4.RepositoryOptsArgs{
					Repo: pulumi.String("https://nvidia.github.io/k8s-device-plugin"),
				},
				Values: pulumi.Map{
					// Configure device plugin with default values:
					// - failOnInitError: false (default) - Don't crash on non-GPU nodes
					// - deviceListStrategy: envvar (default) - Sets NVIDIA_VISIBLE_DEVICES env var
					//   in container specs, enabling GPU-to-container mapping for Datadog GPU monitoring
					"config": pulumi.Map{
						"default": pulumi.String("eks-gpu-config"),
						"map": pulumi.Map{
							"eks-gpu-config": pulumi.String(`version: v1
flags:
  failOnInitError: false
  plugin:
    deviceListStrategy:
      - envvar
`),
						},
					},
					"affinity": pulumi.Map{
						"nodeAffinity": pulumi.Map{
							"requiredDuringSchedulingIgnoredDuringExecution": pulumi.Map{
								"nodeSelectorTerms": pulumi.Array{
									pulumi.Map{
										"matchExpressions": pulumi.Array{
											pulumi.Map{
												"key":      pulumi.String("nvidia.com/gpu.present"),
												"operator": pulumi.String("In"),
												"values": pulumi.Array{
													pulumi.String("true"),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}, pulumi.Provider(eksKubeProvider), utils.PulumiDependsOn(gpuNodeGroup), pulumi.Parent(comp))
			if err != nil {
				return err
			}
		}
		return nil
	})
}
