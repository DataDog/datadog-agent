// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package iam

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
)

const (
	EC2ServicePrincipal = "ec2.amazonaws.com"
	EKSServicePrincipal = "eks.amazonaws.com"
)

func GetAWSPrincipalAssumeRole(e aws.Environment, serviceName []string) (*iam.GetPolicyDocumentResult, error) {
	return iam.GetPolicyDocument(e.Ctx(), &iam.GetPolicyDocumentArgs{
		Statements: []iam.GetPolicyDocumentStatement{
			{
				Actions: []string{
					"sts:AssumeRole",
				},
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
