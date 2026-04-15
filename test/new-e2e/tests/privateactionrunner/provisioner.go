// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"fmt"
	"os"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent/helm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	awsFakeintake "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	awskubernetes "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
)

const (
	// minHelmChartVersion is the earliest Datadog chart release that includes PAR sidecar support
	// (helm-charts PR #2517). Drop this override once the e2e framework's global HelmVersion
	// default is bumped to at least this value.
	minHelmChartVersion = "3.197.2"
)

// parHelmValuesTemplate configures the agent with PAR enabled.
// Fakeintake URL wiring (DD_DD_URL, DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION) is handled
// automatically by the e2e framework's configureFakeintake when fakeintake is present.
// %s parameters: clusterName, runnerURN, privateKeyB64
const parHelmValuesTemplate = `
datadog:
  kubelet:
    tlsVerify: false
  clusterName: "%s"
  privateActionRunner:
    enabled: true
    selfEnroll: false
    urn: "%s"
    privateKey: "%s"
agents:
  useHostNetwork: true
  containers:
    privateActionRunner:
      envDict:
        DD_PRIVATE_ACTION_RUNNER_ACTIONS_ALLOWLIST: "com.datadoghq.remoteaction.rshell.runCommand"
`

// parK8sProvisioner provisions a Kind-on-EC2 cluster with:
//   - fakeintake deployed as ECS Fargate (HTTP, no load balancer) — PAR polls its OPMS endpoints
//   - Datadog Agent with PAR enabled (custom image via --agent-image CLI flag)
func parK8sProvisioner(runnerURN, privateKeyB64 string) provisioners.Provisioner {
	p := provisioners.NewTypedPulumiProvisioner[environments.Kubernetes]("par-k8s",
		func(ctx *pulumi.Context, env *environments.Kubernetes) error {
			name := "kind"
			awsEnv, err := aws.NewEnvironment(ctx)
			if err != nil {
				return fmt.Errorf("aws.NewEnvironment: %w", err)
			}

			// 1. Deploy fakeintake as ECS Fargate (HTTP, no load balancer).
			// PAR inside the Kind cluster reaches it at fakeintake's private VPC IP.
			// The test process also calls fakeintake directly for control operations (enqueue/result).
			// FAKEINTAKE_IMAGE_OVERRIDE allows using a locally-built image during development
			// (same pattern used by CI and docker_test.go).
			var fiOpts []awsFakeintake.Option
			if img := os.Getenv("FAKEINTAKE_IMAGE_OVERRIDE"); img != "" {
				fiOpts = append(fiOpts, awsFakeintake.WithImageURL(img))
			}
			fi, err := awsFakeintake.NewECSFargateInstance(awsEnv, name, fiOpts...)
			if err != nil {
				return fmt.Errorf("fakeintake.NewECSFargateInstance: %w", err)
			}
			if err = fi.Export(ctx, &env.FakeIntake.FakeintakeOutput); err != nil {
				return fmt.Errorf("fi.Export: %w", err)
			}

			// 2. Provision EC2 VM
			host, err := ec2.NewVM(awsEnv, name)
			if err != nil {
				return fmt.Errorf("ec2.NewVM: %w", err)
			}

			installEcrCmd, err := docker.InstallECRCredentialsHelper(awsEnv.Namer, host)
			if err != nil {
				return fmt.Errorf("docker.InstallECRCredentialsHelper: %w", err)
			}

			// 3. Create standard Kind cluster — also installs Docker
			kindCluster, err := kubeComp.NewKindCluster(&awsEnv, host, name,
				awsEnv.KubernetesVersion(),
				utils.PulumiDependsOn(installEcrCmd),
			)
			if err != nil {
				return fmt.Errorf("kubeComp.NewKindCluster: %w", err)
			}
			if err = kindCluster.Export(ctx, &env.KubernetesCluster.ClusterOutput); err != nil {
				return fmt.Errorf("kindCluster.Export: %w", err)
			}

			kubeProvider, err := kubernetes.NewProvider(ctx, awsEnv.Namer.ResourceName("k8s-provider"), &kubernetes.ProviderArgs{
				EnableServerSideApply: pulumi.Bool(true),
				Kubeconfig:            kindCluster.KubeConfig,
			})
			if err != nil {
				return fmt.Errorf("kubernetes.NewProvider: %w", err)
			}

			// 4. Plant test data file on the Kind node (accessible to PAR at /host/var/log/)
			_, err = host.OS.Runner().Command(
				awsEnv.CommonNamer().ResourceName("plant-testdata"),
				&command.Args{
					Create: pulumi.Sprintf(
						`kind get nodes --name %s | xargs -I{} docker exec {} bash -c "echo 'PAR_E2E_VALUE=hello_from_rshell' > /var/log/par-e2e-testdata.txt"`,
						kindCluster.ClusterName,
					),
				},
				utils.PulumiDependsOn(kindCluster),
			)
			if err != nil {
				return fmt.Errorf("plant testdata: %w", err)
			}

			// 5. Deploy Datadog agent via Helm with PAR enabled.
			// DD_DD_URL and DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION for the PAR container are
			// injected automatically by the e2e framework's configureFakeintake.
			agent, err := helm.NewKubernetesAgent(&awsEnv, name, kubeProvider,
				kubernetesagentparams.WithHelmValues(fmt.Sprintf(parHelmValuesTemplate, ctx.Stack(), runnerURN, privateKeyB64)),
				kubernetesagentparams.WithClusterName(kindCluster.ClusterName),
				kubernetesagentparams.WithTags([]string{"stackid:" + ctx.Stack()}),
				kubernetesagentparams.WithHelmChartVersion(minHelmChartVersion),
			)
			if err != nil {
				return fmt.Errorf("helm.NewKubernetesAgent: %w", err)
			}
			if err = agent.Export(ctx, &env.Agent.KubernetesAgentOutput); err != nil {
				return fmt.Errorf("agent.Export: %w", err)
			}

			return nil
		}, nil)

	p.SetDiagnoseFunc(awskubernetes.DiagnoseFunc)
	return p
}
