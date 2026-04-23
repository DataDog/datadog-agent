// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"fmt"
	"os"
	"strings"

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

// rshellOperatorConfig models the operator-side `private_action_runner.restricted_shell.*`
// allow-lists. It distinguishes "key absent" from "key present but empty list", because
// that is the whole point of the truth-table matrix.
type rshellOperatorConfig struct {
	// commandsSet reports whether allowed_commands should appear in datadog.yaml at all.
	// When false the key is omitted entirely (operator "unset"). When true the value of
	// commands is written, including the empty-list case.
	commandsSet bool
	commands    []string

	pathsSet bool
	paths    []string
}

// rshellYAML renders the `restricted_shell` subtree with 6-space indent (i.e. as a child
// of `private_action_runner`, which itself sits under `customAgentConfig` at 4 spaces).
// Returns an empty string when neither axis is configured, so the caller can skip writing
// the customAgentConfig block entirely.
func (c rshellOperatorConfig) rshellYAML() string {
	if !c.commandsSet && !c.pathsSet {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("      restricted_shell:\n")
	if c.commandsSet {
		sb.WriteString("        allowed_commands: ")
		sb.WriteString(yamlList(c.commands))
		sb.WriteString("\n")
	}
	if c.pathsSet {
		sb.WriteString("        allowed_paths: ")
		sb.WriteString(yamlList(c.paths))
		sb.WriteString("\n")
	}
	return sb.String()
}

// yamlList renders a string slice as a YAML flow sequence. An empty slice renders as `[]`,
// which is deliberately different from omitting the key (see Confluence truth table).
func yamlList(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	quoted := make([]string, len(items))
	for i, s := range items {
		quoted[i] = fmt.Sprintf("%q", s)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// parK8sProvisioner provisions a Kind-on-EC2 cluster with:
//   - fakeintake deployed as ECS Fargate (HTTP, no load balancer) — PAR polls its OPMS endpoints
//   - Datadog Agent with PAR enabled (custom image via --agent-image CLI flag)
//
// The rshellCfg controls the operator-side `private_action_runner.restricted_shell.*`
// allow-lists as written to the agent's datadog.yaml. Each suite in the truth-matrix
// coverage passes a different rshellCfg; the per-task backend lists are still set by
// the tests themselves via fakeintake.EnqueuePARTask.
func parK8sProvisioner(runnerURN, privateKeyB64 string, rshellCfg rshellOperatorConfig) provisioners.Provisioner {
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

			// 4. Plant test fixtures on the Kind node (accessible to PAR at /host/...):
			//   - /var/log/par-e2e-testdata.txt       — inside a backend-allowed dir
			//   - /var/log/par-e2e-sibling.txt        — second file in the same dir, used
			//     to exercise operator sub-path narrowing (Confluence: "narrower wins")
			//   - /var/logger/par-e2e-prefix.txt      — prefix-sibling to /var/log, used
			//     to confirm that a backend entry for /var/log does NOT admit /var/logger
			plantScript := fmt.Sprintf(
				`kind get nodes --name %%s | xargs -I{} docker exec {} bash -c '%s'`,
				strings.Join([]string{
					`echo "PAR_E2E_VALUE=hello_from_rshell" > /var/log/par-e2e-testdata.txt`,
					`echo "PAR_E2E_SIBLING=file_in_same_dir" > /var/log/par-e2e-sibling.txt`,
					`mkdir -p /var/logger && echo "PAR_E2E_PREFIX=sibling_dir" > /var/logger/par-e2e-prefix.txt`,
				}, " && "),
			)
			_, err = host.OS.Runner().Command(
				awsEnv.CommonNamer().ResourceName("plant-testdata"),
				&command.Args{
					Create: pulumi.Sprintf(plantScript, kindCluster.ClusterName),
				},
				utils.PulumiDependsOn(kindCluster),
			)
			if err != nil {
				return fmt.Errorf("plant testdata: %w", err)
			}

			// 5. Deploy Datadog agent via Helm with PAR enabled.
			// DD_DD_URL and DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION for the PAR container are
			// injected automatically by the e2e framework's configureFakeintake.
			helmValues := buildPARHelmValues(ctx.Stack(), runnerURN, privateKeyB64, rshellCfg)
			agent, err := helm.NewKubernetesAgent(&awsEnv, name, kubeProvider,
				kubernetesagentparams.WithFakeintake(fi),
				kubernetesagentparams.WithHelmValues(helmValues),
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

// buildPARHelmValues renders the Helm values YAML, conditionally attaching
// `agents.customAgentConfig.private_action_runner.restricted_shell.*` when the operator
// configures either axis. When both axes are unset, the `customAgentConfig` block is
// omitted entirely so the default datadog.yaml is used verbatim.
//
// Fakeintake URL wiring (DD_DD_URL, DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION) is handled
// automatically by the e2e framework's configureFakeintake when fakeintake is present.
func buildPARHelmValues(clusterName, runnerURN, privateKeyB64 string, rshellCfg rshellOperatorConfig) string {
	var customCfg string
	if sub := rshellCfg.rshellYAML(); sub != "" {
		// useConfigMap: true tells the chart to materialise customAgentConfig as a
		// mounted datadog.yaml rather than translating it to env vars (which would
		// lose the []/unset distinction we need to test).
		customCfg = fmt.Sprintf(`  useConfigMap: true
  customAgentConfig:
    private_action_runner:
%s`, sub)
	}

	return fmt.Sprintf(`datadog:
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
%s  containers:
    privateActionRunner:
      envDict:
        DD_PRIVATE_ACTION_RUNNER_ACTIONS_ALLOWLIST: "com.datadoghq.remoteaction.rshell.runCommand"
`, clusterName, runnerURN, privateKeyB64, customCfg)
}
