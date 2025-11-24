// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package aks

import (
	"encoding/base64"
	"math"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/azure"

	"github.com/pulumi/pulumi-azure-native-sdk/authorization/v2"
	"github.com/pulumi/pulumi-azure-native-sdk/containerservice/v2"
	"github.com/pulumi/pulumi-azure-native-sdk/managedidentity/v2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	adminUsername = "azureuser"

	kataNodePoolName = "kata"

	// Kata runtime constants
	kataSku     = "AzureLinux"
	kataRuntime = "KataMshvVmIsolation"
)

func NewCluster(e azure.Environment, name string, kataNodePoolEnabled bool, opts ...pulumi.ResourceOption) (*containerservice.ManagedCluster, pulumi.StringOutput, error) {
	sshPublicKey, err := utils.GetSSHPublicKey(e.DefaultPublicKeyPath())
	if err != nil {
		return nil, pulumi.StringOutput{}, err
	}

	// Warning: we're modifying passed array as it should normally never be used anywhere else
	nodePool := containerservice.ManagedClusterAgentPoolProfileArray{systemNodePool(e, "system")}

	if kataNodePoolEnabled {
		nodePool = append(nodePool, kataNodePool(e))
	}

	opts = append(opts, e.WithProviders(config.ProviderAzure))

	// create a user assigned identity to use for the cluster
	identity, err := managedidentity.NewUserAssignedIdentity(e.Ctx(), "identity", &managedidentity.UserAssignedIdentityArgs{
		ResourceGroupName: pulumi.String(e.DefaultResourceGroup()),
	}, opts...)
	if err != nil {
		return nil, pulumi.StringOutput{}, err
	}

	// assign Network Contributor role to the identity
	nwcontributorRoleAssignment, err := authorization.NewRoleAssignment(e.Ctx(), "role-assignment", &authorization.RoleAssignmentArgs{
		PrincipalId:      identity.PrincipalId,
		PrincipalType:    pulumi.String("ServicePrincipal"),
		Scope:            pulumi.Sprintf("/subscriptions/%s", e.DefaultSubscriptionID()),
		RoleDefinitionId: pulumi.Sprintf("/subscriptions/%s/providers/Microsoft.Authorization/roleDefinitions/4d97b98b-1d4f-4787-a291-c67834d212e7", e.DefaultSubscriptionID()), // Network Contributor built-in role
	}, opts...)
	if err != nil {
		return nil, pulumi.StringOutput{}, err
	}

	// assign ACR Pull role to the identity
	acrPullRoleAssignment, err := authorization.NewRoleAssignment(e.Ctx(), "role-assignment-acr", &authorization.RoleAssignmentArgs{
		PrincipalId:      identity.PrincipalId,
		Scope:            pulumi.String(e.DefaultContainerRegistry()),
		PrincipalType:    pulumi.String("ServicePrincipal"),
		RoleDefinitionId: pulumi.Sprintf("/subscriptions/%s/providers/Microsoft.Authorization/roleDefinitions/7f951dda-4ed3-4680-a7ca-43fe172d538d", e.DefaultSubscriptionID()), // AcrPull built-in role
	}, opts...)
	if err != nil {
		return nil, pulumi.StringOutput{}, err
	}

	cluster, err := containerservice.NewManagedCluster(e.Ctx(), e.Namer.ResourceName(name), &containerservice.ManagedClusterArgs{
		ResourceName:      e.CommonNamer().DisplayName(math.MaxInt, pulumi.String(name)),
		ResourceGroupName: pulumi.String(e.DefaultResourceGroup()),
		KubernetesVersion: pulumi.String(e.KubernetesVersion()),
		AgentPoolProfiles: nodePool,
		LinuxProfile: &containerservice.ContainerServiceLinuxProfileArgs{
			AdminUsername: pulumi.String(adminUsername),
			Ssh: containerservice.ContainerServiceSshConfigurationArgs{
				PublicKeys: containerservice.ContainerServiceSshPublicKeyArray{
					containerservice.ContainerServiceSshPublicKeyArgs{
						KeyData: sshPublicKey,
					},
				},
			},
		},
		AutoUpgradeProfile: containerservice.ManagedClusterAutoUpgradeProfileArgs{
			// Disabling upgrading as this a temporary cluster, we don't want any change after creation
			UpgradeChannel: pulumi.String(containerservice.UpgradeChannelNone),
		},
		DnsPrefix: pulumi.Sprintf("%s-dns", name),
		ApiServerAccessProfile: containerservice.ManagedClusterAPIServerAccessProfileArgs{
			EnablePrivateCluster: pulumi.BoolPtr(false),
		},
		NetworkProfile: containerservice.ContainerServiceNetworkProfileArgs{
			NetworkPlugin: pulumi.String(containerservice.NetworkPluginKubenet),
		},
		Identity: &containerservice.ManagedClusterIdentityArgs{
			Type: containerservice.ResourceIdentityTypeUserAssigned,
			UserAssignedIdentities: pulumi.StringArray{
				identity.ID(),
			},
		},
		Tags: e.ResourcesTags(),
	}, append(opts, pulumi.DependsOn([]pulumi.Resource{nwcontributorRoleAssignment, acrPullRoleAssignment}))...)
	if err != nil {
		return nil, pulumi.StringOutput{}, err
	}

	creds := containerservice.ListManagedClusterUserCredentialsOutput(e.Ctx(),
		containerservice.ListManagedClusterUserCredentialsOutputArgs{
			ResourceGroupName: pulumi.String(e.DefaultResourceGroup()),
			ResourceName:      cluster.Name,
		}, e.WithProvider(config.ProviderAzure),
	)

	kubeconfig := creds.Kubeconfigs().Index(pulumi.Int(0)).Value().
		ApplyT(func(encoded string) string {
			kubeconfig, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				return ""
			}
			return string(kubeconfig)
		}).(pulumi.StringOutput)

	return cluster, kubeconfig, nil
}

func systemNodePool(e azure.Environment, name string) containerservice.ManagedClusterAgentPoolProfileInput {
	return BuildNodePool(NodePoolParams{
		Environment:  e,
		Name:         name,
		Mode:         string(containerservice.AgentPoolModeSystem),
		InstanceType: e.DefaultInstanceType(),
		OSType:       string(containerservice.OSTypeLinux),
		NodeCount:    1,
	})
}

func kataNodePool(e azure.Environment) containerservice.ManagedClusterAgentPoolProfileInput {
	return BuildNodePool(NodePoolParams{
		Environment:     e,
		Name:            kataNodePoolName,
		Mode:            string(containerservice.AgentPoolModeSystem),
		InstanceType:    e.DefaultInstanceType(),
		OSType:          string(containerservice.OSTypeLinux),
		NodeCount:       1,
		WorkloadRuntime: kataRuntime,
		OsSku:           kataSku,
	})
}

type NodePoolParams struct {
	Environment     azure.Environment
	Name            string
	Mode            string
	InstanceType    string
	OSType          string
	OsSku           string
	NodeCount       int
	WorkloadRuntime string
}

func BuildNodePool(params NodePoolParams) containerservice.ManagedClusterAgentPoolProfileInput {
	e := params.Environment
	return containerservice.ManagedClusterAgentPoolProfileArgs{
		Name:               pulumi.String(params.Name),
		OsDiskSizeGB:       pulumi.IntPtr(200),
		Count:              pulumi.IntPtr(params.NodeCount),
		EnableAutoScaling:  pulumi.BoolPtr(false),
		Mode:               pulumi.String(params.Mode),
		EnableNodePublicIP: pulumi.BoolPtr(false),
		Tags:               e.ResourcesTags(),
		OsType:             pulumi.String(params.OSType),
		Type:               pulumi.String(containerservice.AgentPoolTypeVirtualMachineScaleSets),
		VmSize:             pulumi.String(params.InstanceType),
		WorkloadRuntime:    pulumi.String(params.WorkloadRuntime),
		OsSKU:              pulumi.String(params.OsSku),
		VnetSubnetID:       pulumi.String(e.DefaultSubnet()),
	}
}
