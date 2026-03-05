// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package eks

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws/iam"

	awsIam "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
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

// GetFargatePodExecutionRole creates the IAM role that Fargate nodes assume when
// launching pods. The role ARN must be added to the cluster's aws-auth ConfigMap
// so that Fargate nodes can authenticate to the Kubernetes API server. Because
// aws-auth is managed by pulumi-eks with patchForce:true (full ownership of
// mapRoles), callers must pre-create this role and include it in RoleMappings
// before cluster creation — it cannot be read back from the cluster after the
// fact without triggering a circular dependency.
func GetFargatePodExecutionRole(e aws.Environment, name string) (*awsIam.Role, error) {
	assumeRolePolicy, err := iam.GetAWSPrincipalAssumeRole(e, []string{"eks-fargate-pods.amazonaws.com"})
	if err != nil {
		return nil, err
	}

	return awsIam.NewRole(e.Ctx(), e.Namer.ResourceName(name), &awsIam.RoleArgs{
		Name:                e.CommonNamer().DisplayName(64, pulumi.String(name)),
		Description:         pulumi.StringPtr("Fargate pod execution role for EKS Cluster: " + e.Ctx().Stack()),
		ForceDetachPolicies: pulumi.BoolPtr(true),
		ManagedPolicyArns: pulumi.ToStringArray([]string{
			"arn:aws:iam::aws:policy/AmazonEKSFargatePodExecutionRolePolicy",
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
