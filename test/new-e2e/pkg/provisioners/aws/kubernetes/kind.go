// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package awskubernetes contains the provisioner for the Kubernetes based environments
package awskubernetes

import (
	"context"
	_ "embed"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/etcd"
	csidriver "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/csi-driver"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes/argorollouts"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent/helm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentwithoperatorparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/cpustress"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/dogstatsd"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/mutatedbyadmissioncontroller"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/nginx"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/prometheus"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/redis"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/tracegen"
	dogstatsdstandalone "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dogstatsd-standalone"
	fakeintakeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/operator"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/operatorparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes/cilium"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes/vpa"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"
)

const (
	provisionerBaseID = "aws-kind-"
	defaultVMName     = "kind"
)

//go:embed agent_helm_values.yaml
var agentHelmValues string

// parseKubernetesVersion extracts the semantic version from a Kubernetes version string.
// It handles versions with image SHA suffixes (e.g., "v1.32.0@sha256:abc123") by returning
// only the version part (e.g., "v1.32.0").
func parseKubernetesVersion(version string) string {
	// Split on @ to remove any SHA suffix
	if idx := strings.Index(version, "@"); idx != -1 {
		return version[:idx]
	}
	return version
}

// envWithParsedVersion wraps an aws.Environment to override KubernetesVersion()
// with a parsed version that strips SHA suffixes
type envWithParsedVersion struct {
	aws.Environment
	parsedVersion string
}

// KubernetesVersion returns the parsed version without SHA suffix
func (e *envWithParsedVersion) KubernetesVersion() string {
	return e.parsedVersion
}

// KindDiagnoseFunc is the diagnose function for the Kind provisioner
func KindDiagnoseFunc(ctx context.Context, stackName string) (string, error) {
	dumpResult, err := dumpKindClusterState(ctx, stackName)
	if err != nil {
		return "", err
	}
	return "Dumping Kind cluster state:\n" + dumpResult, nil
}

// KindProvisioner creates a new provisioner
func KindProvisioner(opts ...ProvisionerOption) provisioners.TypedProvisioner[environments.Kubernetes] {
	// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
	// and it's easy to forget about it, leading to hard to debug issues.
	params := newProvisionerParams()
	_ = optional.ApplyOptions(params, opts)

	provisioner := provisioners.NewTypedPulumiProvisioner(provisionerBaseID+params.name, func(ctx *pulumi.Context, env *environments.Kubernetes) error {
		// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
		// and it's easy to forget about it, leading to hard to debug issues.
		params := newProvisionerParams()
		_ = optional.ApplyOptions(params, opts)

		return KindRunFunc(ctx, env, params)
	}, params.extraConfigParams)

	provisioner.SetDiagnoseFunc(KindDiagnoseFunc)

	return provisioner
}

// KindRunFunc is the Pulumi run function that runs the provisioner
func KindRunFunc(ctx *pulumi.Context, env *environments.Kubernetes, params *ProvisionerParams) error {
	awsEnv, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	var fakeIntake *fakeintakeComp.Fakeintake
	if params.fakeintakeOptions != nil {
		fakeintakeOpts := []fakeintake.Option{fakeintake.WithLoadBalancer()}
		params.fakeintakeOptions = append(fakeintakeOpts, params.fakeintakeOptions...)
		fakeIntake, err = fakeintake.NewECSFargateInstance(awsEnv, params.name, params.fakeintakeOptions...)
		if err != nil {
			return err
		}
		err = fakeIntake.Export(ctx, &env.FakeIntake.FakeintakeOutput)
		if err != nil {
			return err
		}

		if params.agentOptions != nil {
			newOpts := []kubernetesagentparams.Option{kubernetesagentparams.WithFakeintake(fakeIntake)}
			params.agentOptions = append(newOpts, params.agentOptions...)
		}
		if params.operatorDDAOptions != nil {
			newDdaOpts := []agentwithoperatorparams.Option{agentwithoperatorparams.WithFakeIntake(fakeIntake)}
			params.operatorDDAOptions = append(newDdaOpts, params.operatorDDAOptions...)
		}
		params.vmOptions = append(params.vmOptions, ec2.WithPulumiResourceOptions(utils.PulumiDependsOn(fakeIntake)))
	} else {
		env.FakeIntake = nil
	}

	// Parse the Kubernetes version to handle SHA suffixes (e.g., "v1.32.0@sha256:...")
	// The full version (with SHA) is used for Kind cluster creation
	// The parsed version (without SHA) is used for app deployments that use semver parsing
	kubernetesVersion := awsEnv.KubernetesVersion()
	parsedKubernetesVersion := parseKubernetesVersion(kubernetesVersion)

	// Create a wrapped environment that returns the parsed version
	// This is used by nginx/redis which need to parse the version with semver
	awsEnvParsed := &envWithParsedVersion{
		Environment:   awsEnv,
		parsedVersion: parsedKubernetesVersion,
	}

	host, err := ec2.NewVM(awsEnv, params.name, params.vmOptions...)
	if err != nil {
		return err
	}

	installEcrCredsHelperCmd, err := ec2.InstallECRCredentialsHelper(awsEnv, host)
	if err != nil {
		return err
	}

	var kindCluster *kubeComp.Cluster
	if len(params.ciliumOptions) > 0 {
		kindCluster, err = cilium.NewKindCluster(&awsEnv, host, params.name, kubernetesVersion, params.ciliumOptions, utils.PulumiDependsOn(installEcrCredsHelperCmd))
	} else {
		kindCluster, err = kubeComp.NewKindCluster(&awsEnv, host, params.name, kubernetesVersion, utils.PulumiDependsOn(installEcrCredsHelperCmd))
	}

	if err != nil {
		return err
	}

	err = kindCluster.Export(ctx, &env.KubernetesCluster.ClusterOutput)
	if err != nil {
		return err
	}

	kubeProvider, err := kubernetes.NewProvider(ctx, awsEnv.Namer.ResourceName("k8s-provider"), &kubernetes.ProviderArgs{
		EnableServerSideApply: pulumi.Bool(true),
		Kubeconfig:            kindCluster.KubeConfig,
	})
	if err != nil {
		return err
	}

	vpaCrd, err := vpa.DeployCRD(&awsEnv, kubeProvider)
	if err != nil {
		return err
	}
	dependsOnVPA := utils.PulumiDependsOn(vpaCrd)

	if len(params.ciliumOptions) > 0 {
		// deploy cilium
		ciliumParams, err := cilium.NewParams(params.ciliumOptions...)
		if err != nil {
			return err
		}

		_, err = cilium.NewHelmInstallation(&awsEnv, kindCluster, ciliumParams, pulumi.Provider(kubeProvider))
		if err != nil {
			return err
		}
	}

	var dependsOnArgoRollout pulumi.ResourceOption
	if params.deployArgoRollout {
		argoParams, err := argorollouts.NewParams()
		if err != nil {
			return err
		}
		argoHelm, err := argorollouts.NewHelmInstallation(&awsEnv, argoParams, kubeProvider)
		if err != nil {
			return err
		}
		dependsOnArgoRollout = utils.PulumiDependsOn(argoHelm)
	}

	var dependsOnDDAgent pulumi.ResourceOption
	if params.agentOptions != nil && !params.deployOperator {
		newOpts := []kubernetesagentparams.Option{kubernetesagentparams.WithHelmValues(agentHelmValues), kubernetesagentparams.WithClusterName(kindCluster.ClusterName), kubernetesagentparams.WithTags([]string{"stackid:" + ctx.Stack()})}
		params.agentOptions = append(newOpts, params.agentOptions...)
		agent, err := helm.NewKubernetesAgent(&awsEnv, "kind", kubeProvider, params.agentOptions...)
		if err != nil {
			return err
		}
		err = agent.Export(ctx, &env.Agent.KubernetesAgentOutput)
		if err != nil {
			return err
		}
		dependsOnDDAgent = utils.PulumiDependsOn(agent)
	}

	if params.deployOperator {
		operatorOpts := make([]operatorparams.Option, 0)
		operatorOpts = append(
			operatorOpts,
			params.operatorOptions...,
		)

		operatorComp, err := operator.NewOperator(&awsEnv, awsEnv.Namer.ResourceName("dd-operator"), kubeProvider, operatorOpts...)
		if err != nil {
			return err
		}
		err = operatorComp.Export(ctx, nil)
		if err != nil {
			return err
		}
	}

	if params.deployDogstatsd {
		if _, err := dogstatsdstandalone.K8sAppDefinition(&awsEnv, kubeProvider, "dogstatsd-standalone", fakeIntake, false, ctx.Stack()); err != nil {
			return err
		}
	}

	// Deploy testing workload
	if params.deployTestWorkload {
		// dogstatsd clients that report to the Agent
		if _, err := dogstatsd.K8sAppDefinition(&awsEnv, kubeProvider, "workload-dogstatsd", 8125, "/var/run/datadog/dsd.socket", dependsOnDDAgent /* for admission */); err != nil {
			return err
		}

		if params.deployDogstatsd {
			// dogstatsd clients that report to the dogstatsd standalone deployment
			if _, err := dogstatsd.K8sAppDefinition(&awsEnv, kubeProvider, "workload-dogstatsd-standalone", dogstatsdstandalone.HostPort, dogstatsdstandalone.Socket, dependsOnDDAgent /* for admission */); err != nil {
				return err
			}
		}

		if _, err := tracegen.K8sAppDefinition(&awsEnv, kubeProvider, "workload-tracegen"); err != nil {
			return err
		}

		if _, err := prometheus.K8sAppDefinition(&awsEnv, kubeProvider, "workload-prometheus"); err != nil {
			return err
		}

		if _, err := mutatedbyadmissioncontroller.K8sAppDefinition(&awsEnv, kubeProvider, "workload-mutated", "workload-mutated-lib-injection", dependsOnDDAgent /* for admission */); err != nil {
			return err
		}

		// Get CoreDNS Deployment to use as dependency for etcd which needs DNS
		coreDNS, err := appsv1.GetDeployment(ctx, "coredns", pulumi.ID("kube-system/coredns"), nil, pulumi.Provider(kubeProvider))
		if err != nil {
			return err
		}
		if _, err := etcd.K8sAppDefinition(&awsEnv, kubeProvider, utils.PulumiDependsOn(coreDNS)); err != nil {
			return err
		}

		// These workloads can be deployed only if the agent is installed, they rely on CRDs installed by Agent helm chart
		if params.agentOptions != nil {
			if _, err := nginx.K8sAppDefinition(awsEnvParsed, kubeProvider, "workload-nginx", "", true, dependsOnDDAgent /* for DDM */, dependsOnVPA); err != nil {
				return err
			}

			if _, err := redis.K8sAppDefinition(awsEnvParsed, kubeProvider, "workload-redis", true, dependsOnDDAgent /* for DDM */, dependsOnVPA); err != nil {
				return err
			}

			if _, err := cpustress.K8sAppDefinition(&awsEnv, kubeProvider, "workload-cpustress"); err != nil {
				return err
			}
		}

		if params.deployArgoRollout {
			if _, err := nginx.K8sRolloutAppDefinition(&awsEnv, kubeProvider, "workload-argo-rollout-nginx", dependsOnDDAgent, dependsOnArgoRollout); err != nil {
				return err
			}
		}
	}
	for _, appFunc := range params.workloadAppFuncs {
		_, err := appFunc(&awsEnv, kubeProvider)
		if err != nil {
			return err
		}
	}

	if params.deployOperator && params.operatorDDAOptions != nil {
		// Deploy the datadog CSI driver
		if err := csidriver.NewDatadogCSIDriver(&awsEnv, kubeProvider, csiDriverCommitSHA); err != nil {
			return err
		}
		ddaWithOperatorComp, err := agent.NewDDAWithOperator(&awsEnv, awsEnv.CommonNamer().ResourceName("kind-with-operator"), kubeProvider, params.operatorDDAOptions...)
		if err != nil {
			return err
		}

		if err := ddaWithOperatorComp.Export(ctx, &env.Agent.KubernetesAgentOutput); err != nil {
			return err
		}

	}

	if params.agentOptions == nil || (params.operatorDDAOptions == nil) {
		env.Agent = nil
	}

	return nil
}
