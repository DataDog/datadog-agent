package eks

import (
	"fmt"

	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components"
	kubecomp "github.com/DataDog/test-infra-definitions/components/kubernetes"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	localEks "github.com/DataDog/test-infra-definitions/resources/aws/eks"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	awsEks "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/eks"
	awsIam "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-eks/sdk/v3/go/eks"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	v1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/rbac/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/samber/lo"
)

func NewCluster(e aws.Environment, name string, opts ...Option) (*kubecomp.Cluster, error) {
	params, err := NewParams(opts...)
	if err != nil {
		return nil, err
	}
	fmt.Printf("params: %+v\n", params)
	return components.NewComponent(&e, name, func(comp *kubecomp.Cluster) error {
		// Create Cluster SG
		clusterSG, err := ec2.NewSecurityGroup(e.Ctx(), e.Namer.ResourceName("eks-sg"), &ec2.SecurityGroupArgs{
			NamePrefix:  e.CommonNamer().DisplayName(255, pulumi.String("eks-sg")),
			Description: pulumi.StringPtr("EKS Cluster sg for stack: " + e.Ctx().Stack()),
			Ingress: ec2.SecurityGroupIngressArray{
				ec2.SecurityGroupIngressArgs{
					CidrBlocks:     pulumi.ToStringArray(e.EKSAllowedInboundCIDRs()),
					SecurityGroups: pulumi.ToStringArray(e.EKSAllowedInboundSecurityGroups()),
					PrefixListIds:  pulumi.ToStringArray(e.EKSAllowedInboundPrefixLists()),
					ToPort:         pulumi.Int(22),
					FromPort:       pulumi.Int(22),
					Protocol:       pulumi.String("tcp"),
				},
				ec2.SecurityGroupIngressArgs{
					CidrBlocks:     pulumi.ToStringArray(e.EKSAllowedInboundCIDRs()),
					SecurityGroups: pulumi.ToStringArray(e.EKSAllowedInboundSecurityGroups()),
					PrefixListIds:  pulumi.ToStringArray(e.EKSAllowedInboundPrefixLists()),
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
			if params.UseAL2023Nodes {
				_, err := localEks.NewAL2023LinuxNodeGroup(e, cluster, linuxNodeRole, utils.PulumiDependsOn(nodeDeps...), pulumi.Parent(comp))
				if err != nil {
					return err
				}
			} else {
				_, err := localEks.NewLinuxNodeGroup(e, cluster, linuxNodeRole, utils.PulumiDependsOn(nodeDeps...), pulumi.Parent(comp))
				if err != nil {
					return err
				}
			}
		}

		if params.LinuxARMNodeGroup {
			if params.UseAL2023Nodes {
				_, err := localEks.NewAL2023LinuxARMNodeGroup(e, cluster, linuxNodeRole, utils.PulumiDependsOn(nodeDeps...), pulumi.Parent(comp))
				if err != nil {
					return err
				}
			} else {
				_, err := localEks.NewLinuxARMNodeGroup(e, cluster, linuxNodeRole, utils.PulumiDependsOn(nodeDeps...), pulumi.Parent(comp))
				if err != nil {
					return err
				}
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
		return nil
	})
}
