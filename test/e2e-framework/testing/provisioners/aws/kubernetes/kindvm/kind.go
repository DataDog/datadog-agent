// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package kindvm contains the provisioner for the Kind-on-VM Kubernetes based environments
package kindvm

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	awskubernetes "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	provisionerBaseID = "aws-kind-"
)

// DiagnoseFunc is the diagnose function for the Kind provisioner
func DiagnoseFunc(ctx context.Context, stackName string) (string, error) {
	dumpResult, err := awskubernetes.DumpKindClusterState(ctx, stackName)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Dumping Kind cluster state:\n%s", dumpResult), nil
}

func Provisioner(opts ...provisionerOption) provisioners.TypedProvisioner[environments.Kubernetes] {
	params := getProvisionerParams(opts...)
	runParams := kindvm.GetRunParams(params.runOptions...)

	provisioner := provisioners.NewTypedPulumiProvisioner(provisionerBaseID+runParams.Name, func(ctx *pulumi.Context, env *environments.Kubernetes) error {
		params := getProvisionerParams(opts...)
		runParams := kindvm.GetRunParams(params.runOptions...)

		var awsEnv aws.Environment
		var err error
		if params.awsEnv != nil {
			awsEnv = *params.awsEnv
		} else {
			awsEnv, err = aws.NewEnvironment(ctx)
			if err != nil {
				return err
			}
			params.awsEnv = &awsEnv
		}

		return kindvm.RunWithEnv(ctx, awsEnv, env, runParams)
	}, params.extraConfigParams)

	provisioner.SetDiagnoseFunc(DiagnoseFunc)
	return provisioner
}
