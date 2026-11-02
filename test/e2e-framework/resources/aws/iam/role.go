// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package iam

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/iam"
)

const (
	EC2ServicePrincipal = "ec2.amazonaws.com"
	EKSServicePrincipal = "eks.amazonaws.com"
)

func GetAWSPrincipalAssumeRole(e aws.Environment, serviceName []string) (*iam.GetPolicyDocumentResult, error) {
	return GetAWSPrincipalAssumeRoleWithActions(e, serviceName, []string{"sts:AssumeRole"})
}

// GetAWSPrincipalAssumeRoleWithActions builds an assume-role trust policy for the given
// service principals with a custom set of trust actions. EKS Auto Mode, for example,
// requires "sts:TagSession" in addition to "sts:AssumeRole" on the cluster service role.
func GetAWSPrincipalAssumeRoleWithActions(e aws.Environment, serviceName []string, actions []string) (*iam.GetPolicyDocumentResult, error) {
	return iam.GetPolicyDocument(e.Ctx(), &iam.GetPolicyDocumentArgs{
		Statements: []iam.GetPolicyDocumentStatement{
			{
				Actions: actions,
				Principals: []iam.GetPolicyDocumentStatementPrincipal{
					{
						Type:        "Service",
						Identifiers: serviceName,
					},
				},
			},
		},
	}, nil, e.WithProvider(config.ProviderAWS))
}
