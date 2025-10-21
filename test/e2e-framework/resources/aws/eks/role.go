package eks

import (
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/resources/aws/iam"

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
