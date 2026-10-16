// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package eks

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws/iam"

	awsIam "github.com/pulumi/pulumi-aws/sdk/v7/go/aws/iam"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func GetNodeRole(e aws.Environment, name string) (*awsIam.Role, error) {
	assumeRolePolicy, err := iam.GetAWSPrincipalAssumeRole(e, []string{iam.EC2ServicePrincipal})
	if err != nil {
		return nil, err
	}

	return awsIam.NewRole(e.Ctx(), e.Namer.ResourceName(name), &awsIam.RoleArgs{
		Name:                e.CommonNamer().DisplayName(64, pulumi.String(name)),
		Description:         pulumi.StringPtr("Node role for EKS Cluster: " + e.Ctx().Stack()),
		ForceDetachPolicies: pulumi.BoolPtr(true),
		ManagedPolicyArns: pulumi.ToStringArray([]string{
			"arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy",
			"arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly",
			"arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy",
		}),
		AssumeRolePolicy: pulumi.String(assumeRolePolicy.Json),
	}, e.WithProviders(config.ProviderAWS))
}

func GetClusterRole(e aws.Environment, name string) (*awsIam.Role, error) {
	assumeRolePolicy, err := iam.GetAWSPrincipalAssumeRole(e, []string{iam.EKSServicePrincipal})
	if err != nil {
		return nil, err
	}

	return awsIam.NewRole(e.Ctx(), e.Namer.ResourceName(name), &awsIam.RoleArgs{
		Name:        e.CommonNamer().DisplayName(64, pulumi.String(name)),
		Description: pulumi.StringPtr("Service role for EKS Cluster: " + e.Ctx().Stack()),
		ManagedPolicyArns: pulumi.ToStringArray([]string{
			"arn:aws:iam::aws:policy/AmazonEKSClusterPolicy",
			"arn:aws:iam::aws:policy/AmazonEKSVPCResourceController",
		}),
		AssumeRolePolicy: pulumi.String(assumeRolePolicy.Json),
	}, e.WithProviders(config.ProviderAWS))
}

// GetAutoModeClusterRole returns an EKS cluster service role for use with EKS Auto
// Mode. Auto Mode requires additional managed policies on top of the standard cluster
// policies, and a trust policy that grants "sts:TagSession" in addition to
// "sts:AssumeRole". See https://docs.aws.amazon.com/eks/latest/userguide/automode-permissions.html
func GetAutoModeClusterRole(e aws.Environment, name string) (*awsIam.Role, error) {
	assumeRolePolicy, err := iam.GetAWSPrincipalAssumeRoleWithActions(e, []string{iam.EKSServicePrincipal}, []string{"sts:AssumeRole", "sts:TagSession"})
	if err != nil {
		return nil, err
	}

	return awsIam.NewRole(e.Ctx(), e.Namer.ResourceName(name), &awsIam.RoleArgs{
		Name:        e.CommonNamer().DisplayName(64, pulumi.String(name)),
		Description: pulumi.StringPtr("Auto Mode service role for EKS Cluster: " + e.Ctx().Stack()),
		ManagedPolicyArns: pulumi.ToStringArray([]string{
			"arn:aws:iam::aws:policy/AmazonEKSClusterPolicy",
			"arn:aws:iam::aws:policy/AmazonEKSVPCResourceController",
			"arn:aws:iam::aws:policy/AmazonEKSComputePolicy",
			"arn:aws:iam::aws:policy/AmazonEKSBlockStoragePolicyV2",
			"arn:aws:iam::aws:policy/AmazonEKSLoadBalancingPolicy",
			"arn:aws:iam::aws:policy/AmazonEKSNetworkingPolicy",
		}),
		AssumeRolePolicy: pulumi.String(assumeRolePolicy.Json),
	}, e.WithProviders(config.ProviderAWS))
}
