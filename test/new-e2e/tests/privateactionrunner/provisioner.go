// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"fmt"
	"os"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	awsFakeintake "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/helmagent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	awskubernetes "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
)

const (
	// minHelmChartVersion is the earliest Datadog chart release that includes PAR sidecar support
	// (helm-charts PR #2517). Drop this override once the e2e framework's global HelmVersion
	// default is bumped to at least this value.
	minHelmChartVersion = "3.197.2"
)

// parHelmValuesTemplate configures the agent with PAR enabled.
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
//   - fakeintake deployed as ECS Fargate (HTTP, no load balancer)
//   - Datadog Agent with PAR enabled, installed via Helm in PostProvision
func parK8sProvisioner(runnerURN, privateKeyB64 string) provisioners.Provisioner {
	p := provisioners.NewTypedPulumiProvisioner[environments.Kubernetes]("par-k8s",
		func(ctx *pulumi.Context, env *environments.Kubernetes) error {
			name := "kind"
			awsEnv, err := aws.NewEnvironment(ctx)
			if err != nil {
				return fmt.Errorf("aws.NewEnvironment: %w", err)
			}

			// 1. Deploy fakeintake as ECS Fargate (HTTP, no load balancer).
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

			// 4. Plant test data file on the Kind node
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

			// Agent installation moved to PostProvision.
			env.Agent = nil
			return nil
		}, nil)

	p.SetDiagnoseFunc(awskubernetes.DiagnoseFunc)

	return provisioners.WithPostProvision(p, func(t *testing.T, env *environments.Kubernetes) {
		clusterName := env.KubernetesCluster.ClusterName
		helmagent.Install(installers.FromT(t), env, runner.CloudAWS,
			kubernetesagentparams.WithHelmValues(fmt.Sprintf(parHelmValuesTemplate, clusterName, runnerURN, privateKeyB64)),
			kubernetesagentparams.WithTags([]string{"stackid:" + clusterName}),
			kubernetesagentparams.WithHelmChartVersion(minHelmChartVersion),
		)
	})
}
