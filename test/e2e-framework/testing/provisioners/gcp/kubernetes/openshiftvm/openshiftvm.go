// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package gcpopenshiftvm contains the provisioner for OpenShift VM on GCP
package gcpopenshiftvm

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	kubernetesNewProvider "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	agentComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent/helm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/cpustress"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/dogstatsd"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/etcd"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/mutatedbyadmissioncontroller"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/nginx"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/prometheus"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/redis"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/tracegen"
	dogstatsdstandalone "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dogstatsd-standalone"
	fakeintakeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes/argorollouts"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes/vpa"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/gcp"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/gcp/compute"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/gcp/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"
)

const (
	provisionerBaseID = "gcp-openshiftvm"
)

// OpenshiftVMProvisioner creates a new provisioner for OpenShift VM on GCP
func OpenshiftVMProvisioner(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.Kubernetes] {
	// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
	// and it's easy to forget about it, leading to hard to debug issues.
	params := newProvisionerParams()
	_ = optional.ApplyOptions(params, opts)

	provisioner := provisioners.NewTypedPulumiProvisioner(provisionerBaseID+params.name, func(ctx *pulumi.Context, env *environments.Kubernetes) error {
		// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
		// and it's easy to forget about it, leading to hard to debug issues.
		params := newProvisionerParams()
		_ = optional.ApplyOptions(params, opts)

		return OpenShiftVMRunFunc(ctx, env, params)
	}, params.extraConfigParams)

	return provisioner
}

func OpenShiftVMRunFunc(ctx *pulumi.Context, env *environments.Kubernetes, params *ProvisionerParams) error {
	gcpEnv, err := gcp.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	osDesc := os.DescriptorFromString("redhat:9", os.RedHat9)
	vm, err := compute.NewVM(gcpEnv, "openshift",
		compute.WithOS(osDesc),
		compute.WithInstancetype("n2-standard-16"),
		compute.WithNestedVirt(true),
	)
	if err != nil {
		return err
	}
	if err := vm.Export(ctx, nil); err != nil {
		return err
	}

	// Create the OpenShift cluster
	openshiftCluster, err := kubernetes.NewOpenShiftCluster(&gcpEnv, vm, "openshift", gcpEnv.OpenShiftPullSecretPath(), params.openshiftOptions...)
	if err != nil {
		return err
	}
	err = openshiftCluster.Export(ctx, &env.KubernetesCluster.ClusterOutput)
	if err != nil {
		return err
	}

	if gcpEnv.InitOnly() {
		return nil
	}

	// Building Kubernetes provider for OpenShift
	openshiftKubeProvider, err := kubernetesNewProvider.NewProvider(ctx, gcpEnv.Namer.ResourceName("openshift-k8s-provider"), &kubernetesNewProvider.ProviderArgs{
		Kubeconfig:            openshiftCluster.KubeConfig,
		EnableServerSideApply: pulumi.BoolPtr(true),
		DeleteUnreachable:     pulumi.BoolPtr(true),
	})
	if err != nil {
		return err
	}

	// Deploy a fakeintake
	var fakeIntake *fakeintakeComp.Fakeintake
	if params.fakeintakeOptions != nil {
		fakeIntakeOptions := []fakeintake.Option{
			fakeintake.WithMemory(6144),
		}
		if gcpEnv.InfraShouldDeployFakeintakeWithLB() {
			fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithLoadBalancer())
		}
		fakeIntake, err = fakeintake.NewVMInstance(gcpEnv, fakeIntakeOptions...)
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

	// Deploy the agent
	var agent *agentComp.KubernetesAgent
	if params.agentOptions != nil {
		params.agentOptions = append(params.agentOptions,
			func(p *kubernetesagentparams.Params) error {
				p.HelmValues = append(p.HelmValues, agentComp.BuildOpenShiftHelmValues().ToYAMLPulumiAssetOutput())
				return nil
			},
			kubernetesagentparams.WithClusterName(openshiftCluster.ClusterName),
			kubernetesagentparams.WithNamespace("datadog"),
			// OpenShift deployments need more time due to security context constraints and slower startup
			kubernetesagentparams.WithTimeout(900), // 15 minutes
			// Use the cluster name (DisplayName(49)) for the stackid tag instead of ctx.Stack(),
			// because the cluster name may be truncated
			kubernetesagentparams.WithStackIDTag(openshiftCluster.ClusterName),
		)

		if fakeIntake != nil {
			params.agentOptions = append(params.agentOptions, kubernetesagentparams.WithFakeintake(fakeIntake))
		}

		agent, err = helm.NewKubernetesAgent(&gcpEnv, params.name, openshiftKubeProvider, params.agentOptions...)
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

	if gcpEnv.TestingWorkloadDeploy() {
		// Deploy the VPA CRD
		vpaCrd, err := vpa.DeployCRD(&gcpEnv, openshiftKubeProvider)
		if err != nil {
			return err
		}

		var argoHelm *argorollouts.HelmComponent
		if params.deployArgoRollout {
			argoParams, err := argorollouts.NewParams()
			if err != nil {
				return err
			}
			argoHelm, err = argorollouts.NewHelmInstallation(&gcpEnv, argoParams, openshiftKubeProvider)
			if err != nil {
				return err
			}
		}
		// Add the Argo Rollout to the dependencies
		dependsOnArgoRollout := utils.PulumiDependsOn(argoHelm)

		// Add the VPA CRD to the dependencies
		dependsOnVPA := utils.PulumiDependsOn(vpaCrd)

		// Add the Agent to the dependencies
		dependsOnDDAgent := utils.PulumiDependsOn(agent)

		// Deploy the testing workloads
		if _, err := redis.K8sAppDefinition(&gcpEnv, openshiftKubeProvider, "workload-redis", true, dependsOnDDAgent /* for DDM */, dependsOnVPA); err != nil {
			return err
		}

		if _, err := prometheus.K8sAppDefinition(&gcpEnv, openshiftKubeProvider, "workload-prometheus"); err != nil {
			return err
		}

		if _, err := etcd.K8sAppDefinition(&gcpEnv, openshiftKubeProvider); err != nil {
			return err
		}

		if _, err := cpustress.K8sAppDefinition(&gcpEnv, openshiftKubeProvider, "workload-cpustress"); err != nil {
			return err
		}

		if _, err := tracegen.K8sAppDefinition(&gcpEnv, openshiftKubeProvider, "workload-tracegen"); err != nil {
			return err
		}

		if _, err := nginx.K8sAppDefinition(&gcpEnv, openshiftKubeProvider, "workload-nginx", 8080, "", true, dependsOnDDAgent /* for DDM */, dependsOnVPA); err != nil {
			return err
		}

		if _, err := mutatedbyadmissioncontroller.K8sAppDefinition(&gcpEnv, openshiftKubeProvider, "workload-mutated", "workload-mutated-lib-injection", dependsOnDDAgent /* for admission */); err != nil {
			return err
		}

		if gcpEnv.DogstatsdDeploy() {
			// Standalone dogstatsd
			if _, err := dogstatsdstandalone.K8sAppDefinition(&gcpEnv, openshiftKubeProvider, "dogstatsd-standalone", "/var/run/crio/crio.sock", fakeIntake, false, ""); err != nil {
				return err
			}

			// Dogstatsd clients that report to the standalone dogstatsd deployment
			if _, err := dogstatsd.K8sAppDefinition(&gcpEnv, openshiftKubeProvider, "workload-dogstatsd-standalone", dogstatsdstandalone.HostPort, "/run/datadog/dsd.socket", dependsOnDDAgent /* for admission */); err != nil {
				return err
			}

			// Dogstatsd clients that report to the Agent
			if _, err := dogstatsd.K8sAppDefinition(&gcpEnv, openshiftKubeProvider, "workload-dogstatsd", 8125, "/var/run/datadog/dsd.socket", dependsOnDDAgent /* for admission */); err != nil {
				return err
			}
		}

		if params.deployArgoRollout {
			if _, err := nginx.K8sRolloutAppDefinition(&gcpEnv, openshiftKubeProvider, "workload-argo-rollout-nginx", 8080, dependsOnDDAgent, dependsOnArgoRollout); err != nil {
				return err
			}
		}
	}

	return nil
}
