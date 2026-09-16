// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package kubeadm contains the provisioner for the kubeadm-on-VM Kubernetes based environments
package kubeadm

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kubeadm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	awskubernetes "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	provisionerBaseID = "aws-kubeadm-"
)

// DiagnoseFunc dumps the kubeadm cluster state (nodes, pods, events) on failure.
func DiagnoseFunc(_ context.Context, stackName string) (string, error) {
	// The framework may invoke Diagnose with an already-expired context after a long
	// provisioning failure; use a fresh one so the SSH dump can actually run.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	dumpResult, err := awskubernetes.DumpKubeadmClusterState(ctx, stackName)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Dumping Kubeadm cluster state:\n%s", dumpResult), nil
}

// Provisioner creates a kubeadm-on-VM Kubernetes provisioner.
func Provisioner(opts ...provisionerOption) provisioners.TypedProvisioner[environments.Kubernetes] {
	params := getProvisionerParams(opts...)
	runParams := kubeadm.GetRunParams(params.runOptions...)

	provisioner := provisioners.NewTypedPulumiProvisioner(provisionerBaseID+runParams.Name, func(ctx *pulumi.Context, env *environments.Kubernetes) error {
		params := getProvisionerParams(opts...)
		runParams := kubeadm.GetRunParams(params.runOptions...)

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

		return kubeadm.RunWithEnv(ctx, awsEnv, env, runParams)
	}, params.extraConfigParams)

	provisioner.SetDiagnoseFunc(DiagnoseFunc)
	return provisioner
}
