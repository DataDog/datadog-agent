// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package gcpkubernetes contains the provisioner for Google Kubernetes Engine (GKE)
package gcpkubernetes

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/gcp"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/gcp/gke"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/gcp/fakeintake"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/helmagent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
)

const (
	provisionerBaseID = "gcp-gke"
)

// GKEProvisioner creates a new provisioner for GKE on GCP.
//
// Agent installation is performed via Helm after Pulumi provisions the GKE
// cluster and FakeIntake (PostProvision), rather than inside Pulumi itself.
func GKEProvisioner(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.Kubernetes] {
	// Capture user-provided agent options outside the closure so PostProvision
	// receives clean options (before Pulumi would add the fakeintake resource).
	params := newProvisionerParams(opts...)
	agentOpts := params.agentOptions
	usePostProvision := agentOpts != nil

	pulumiProv := provisioners.NewTypedPulumiProvisioner(provisionerBaseID+params.name, func(ctx *pulumi.Context, env *environments.Kubernetes) error {
		// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
		// and it's easy to forget about it, leading to hard to debug issues.
		params := newProvisionerParams(opts...)
		if usePostProvision {
			params.agentOptions = nil
		}
		return GKERunFunc(ctx, env, params)
	}, params.extraConfigParams)

	if !usePostProvision {
		return pulumiProv
	}

	return provisioners.WithPostProvision(pulumiProv, func(t *testing.T, env *environments.Kubernetes) {
		helmagent.Install(installers.FromT(t), env, runner.CloudGCP, agentOpts...)
	})
}

// GKERunFunc is the run function for GKE provisioner.
// When agentOptions is nil (PostProvision handles the install), it only
// provisions the cluster and FakeIntake.
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

	// If InitOnly is set, return after creating the cluster without deploying the agent.
	if gcpEnv.InitOnly() {
		return nil
	}

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
	} else {
		env.FakeIntake = nil
	}

	// Agent installation is handled by PostProvision via helmagent.Install.
	env.Agent = nil

	// Deploy Pulumi-based workloads (workloadAppFuncs use the Pulumi k8s provider).
	// These are separate from the workloads.Deploy installer path.
	for _, appFunc := range params.workloadAppFuncs {
		_, err := appFunc(&gcpEnv, cluster.KubeProvider)
		if err != nil {
			return err
		}
	}

	return nil
}
