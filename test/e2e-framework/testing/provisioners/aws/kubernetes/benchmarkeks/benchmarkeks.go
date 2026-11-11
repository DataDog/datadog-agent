// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package benchmarkeks contains the provisioner for the benchmarkeks environment
package benchmarkeks

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/benchmarkeks"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/eks"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
)

const (
	provisionerID = "aws-benchmarkeks"
)

// Provisioner creates a new provisioner of the benchmarkeks environment. The images of the
// two Agent installations, of variants a and b, are read from the ddagent:baseline* and
// ddagent:comparison* config keys respectively (e.g. given with
// -c ddagent:baselineFullImagePath=<image>), overridden by the run options.
func Provisioner(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.BenchmarkEKS] {
	params := getProvisionerParams(opts...)

	// The benchmark workload is driven by the churn orchestrator, not by the shared
	// testing workload. Leaving the latter enabled would add an etcd config provider
	// pointing at an etcd this scenario never deploys, making both Agents log connection
	// errors for the whole run (the invoke task disables it the same way), hence set after the
	// extra config params.
	configMap := runner.ConfigMap{}
	configMap.Merge(params.extraConfigParams)
	configMap.Set(config.DDTestingWorkloadNamespace+":"+config.DDTestingWorkloadDeployParamName, "false", false)

	provisioner := provisioners.NewTypedPulumiProvisioner(provisionerID, func(ctx *pulumi.Context, env *environments.BenchmarkEKS) error {
		// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
		// and it's easy to forget about it, leading to hard to debug issues.
		params := getProvisionerParams(opts...)

		var awsEnv aws.Environment
		var err error
		if params.awsEnv != nil {
			awsEnv = *params.awsEnv
		} else {
			awsEnv, err = aws.NewEnvironment(ctx)
			if err != nil {
				return err
			}
		}

		return benchmarkeks.RunWithEnv(ctx, awsEnv, env, benchmarkeks.ParamsFromEnvironment(awsEnv, params.runOptions...))
	}, configMap)

	provisioner.SetDiagnoseFunc(eks.DiagnoseFunc)

	return provisioner
}
