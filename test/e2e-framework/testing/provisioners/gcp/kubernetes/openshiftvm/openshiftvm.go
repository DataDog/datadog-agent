// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package gcpopenshiftvm contains the provisioner for OpenShift VM on GCP
package gcpopenshiftvm

import (
	"context"
	"fmt"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	kubernetesNewProvider "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
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
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/helmagent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	gcpkubernetes "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/gcp/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
)

const (
	provisionerBaseID = "gcp-openshiftvm"
)

var openShiftPrivilegedPSSLabels = pulumi.StringMap{
	"pod-security.kubernetes.io/enforce": pulumi.String("privileged"),
	"pod-security.kubernetes.io/warn":    pulumi.String("privileged"),
	"pod-security.kubernetes.io/audit":   pulumi.String("privileged"),
}

func openshiftDiagnoseFunc(ctx context.Context, name string) (string, error) {
	dumpResult, err := gcpkubernetes.DumpOpenshiftClusterState(ctx, name)
	if err != nil {
		return dumpResult, err
	}
	return fmt.Sprintf("Dumping OpenShift cluster state:\n%s", dumpResult), nil
}

// OpenshiftVMProvisioner creates a new provisioner for OpenShift VM on GCP.
//
// Agent installation is performed via Helm after Pulumi provisions the
// OpenShift cluster and FakeIntake (PostProvision). OpenShift-specific Helm
// overrides are prepended automatically. The preAgentHooks (SCC, namespace
// labels) continue to run inside Pulumi since they use the Pulumi k8s
// provider; only the Helm agent install moves outside.
func OpenshiftVMProvisioner(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.Kubernetes] {
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
		return OpenShiftVMRunFunc(ctx, env, params)
	}, params.extraConfigParams)

	pulumiProv.SetDiagnoseFunc(openshiftDiagnoseFunc)

	if !usePostProvision {
		return pulumiProv
	}

	// Prepend OpenShift-specific Helm values then user options.
	postProvisionOpts := append(
		[]kubernetesagentparams.Option{kubernetesagentparams.WithHelmValues(helmagent.OpenShiftHelmValues)},
		agentOpts...,
	)

	return provisioners.WithPostProvision(pulumiProv, func(t *testing.T, env *environments.Kubernetes) {
		helmagent.Install(installers.FromT(t), env, runner.CloudGCP, postProvisionOpts...)
	})
}

func OpenShiftVMRunFunc(ctx *pulumi.Context, env *environments.Kubernetes, params *ProvisionerParams) error {
	gcpEnv, err := gcp.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	osDesc := os.DescriptorFromString("redhat:9", os.RedHat9)
	vm, err := compute.NewVM(gcpEnv, "openshift",
		compute.WithOS(osDesc),
		compute.WithInstancetype("n2-standard-32"),
		compute.WithNestedVirt(true),
		// this is used by the dumpCluster debug function
		compute.WithLabels(map[string]string{"kube-provider": "openshift"}),
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

	// Run preAgentHooks (SCC setup, namespace labels) via the Pulumi k8s provider.
	// These configure OpenShift-specific cluster state that must exist before
	// the agent is installed. Agent installation itself is handled by PostProvision.
	for _, hook := range params.preAgentHooks {
		if err := hook(&gcpEnv, openshiftKubeProvider); err != nil {
			return err
		}
	}

	// Agent installation is handled by PostProvision via helmagent.Install.
	env.Agent = nil
	var dependsOnDDAgent pulumi.ResourceOption

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
			if _, err := dogstatsd.K8sAppDefinitionWithOptions(
				&gcpEnv,
				openshiftKubeProvider,
				"workload-dogstatsd-standalone",
				dogstatsdstandalone.HostPort,
				"/run/datadog/dsd.socket",
				[]dogstatsd.K8sAppOption{dogstatsd.WithNamespaceLabels(openShiftPrivilegedPSSLabels)},
				dependsOnDDAgent, /* for admission */
			); err != nil {
				return err
			}

			// Dogstatsd clients that report to the Agent
			if _, err := dogstatsd.K8sAppDefinitionWithOptions(
				&gcpEnv,
				openshiftKubeProvider,
				"workload-dogstatsd",
				8125,
				"/var/run/datadog/dsd.socket",
				[]dogstatsd.K8sAppOption{dogstatsd.WithNamespaceLabels(openShiftPrivilegedPSSLabels)},
				dependsOnDDAgent, /* for admission */
			); err != nil {
				return err
			}
		}

		if params.deployArgoRollout {
			if _, err := nginx.K8sRolloutAppDefinition(&gcpEnv, openshiftKubeProvider, "workload-argo-rollout-nginx", 8080, dependsOnDDAgent, dependsOnArgoRollout); err != nil {
				return err
			}
		}
	}

	if dependsOnDDAgent != nil {
		for _, appFunc := range params.agentDependentWorkloadAppFuncs {
			_, err := appFunc(&gcpEnv, openshiftKubeProvider, dependsOnDDAgent)
			if err != nil {
				return err
			}
		}
	}

	for _, appFunc := range params.workloadAppFuncs {
		_, err := appFunc(&gcpEnv, openshiftKubeProvider)
		if err != nil {
			return err
		}
	}

	return nil
}
