// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package gcpkubernetes contains the provisioner for Google Kubernetes Engine (GKE)
package gcpkubernetes

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/gcp"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/gcp/gke"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent/helm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/gcp/fakeintake"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"
)

const (
	provisionerBaseID = "gcp-gke"
)

// GKEProvisioner creates a new provisioner for GKE on GCP
func GKEProvisioner(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.Kubernetes] {
	// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
	// and it's easy to forget about it, leading to hard to debug issues.
	params := newProvisionerParams()
	_ = optional.ApplyOptions(params, opts)

	provisioner := provisioners.NewTypedPulumiProvisioner(provisionerBaseID+params.name, func(ctx *pulumi.Context, env *environments.Kubernetes) error {
		// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
		// and it's easy to forget about it, leading to hard to debug issues.
		params := newProvisionerParams()
		_ = optional.ApplyOptions(params, opts)

		return GKERunFunc(ctx, env, params)
	}, params.extraConfigParams)

	return provisioner
}

// GKERunFunc is the run function for GKE provisioner
func GKERunFunc(ctx *pulumi.Context, env *environments.Kubernetes, params *ProvisionerParams) error {
	gcpEnv, err := gcp.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	// Create the cluster
	cluster, err := gke.NewGKECluster(gcpEnv, params.gkeOptions...)
	if err != nil {
		return err
	}
	err = cluster.Export(ctx, &env.KubernetesCluster.ClusterOutput)
	if err != nil {
		return err
	}

	agentOptions := params.agentOptions

	// Deploy a fakeintake
	if params.fakeintakeOptions != nil {
		fakeIntake, err := fakeintake.NewVMInstance(gcpEnv, params.fakeintakeOptions...)
		if err != nil {
			return err
		}
		err = fakeIntake.Export(ctx, &env.FakeIntake.FakeintakeOutput)
		if err != nil {
			return err
		}
		agentOptions = append(agentOptions, kubernetesagentparams.WithFakeintake(fakeIntake))

	} else {
		env.FakeIntake = nil
	}

	if params.agentOptions != nil {
		agent, err := helm.NewKubernetesAgent(&gcpEnv, params.name, cluster.KubeProvider, agentOptions...)
		if err != nil {
			return err
		}
		err = agent.Export(ctx, &env.Agent.KubernetesAgentOutput)
		if err != nil {
			return err
		}
	} else {
		env.Agent = nil
	}

	// Deploy workloads
	for _, appFunc := range params.workloadAppFuncs {
		_, err := appFunc(&gcpEnv, cluster.KubeProvider)
		if err != nil {
			return err
		}
	}

	return nil
}
