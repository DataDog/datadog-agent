// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package gpu

import (
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes/mokka"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	awskubernetes "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
)

// busyboxTag is the busybox tag used by the GPU test pods.
const busyboxTag = "1.36"

// busyboxImage returns the busybox image for the GPU test pods. It goes through the ECR
// pull-through cache of DockerHub when the runner has an image pull registry, so the pull is not
// subject to DockerHub anonymous rate limits, and falls back to DockerHub otherwise (local runs).
func busyboxImage() string {
	reg, _ := runner.GetProfile().ParamStore().GetWithDefault(parameters.ImagePullRegistry, "")
	if reg != "" {
		return strings.SplitN(reg, ",", 2)[0] + "/dockerhub/library/busybox:" + busyboxTag
	}
	return "busybox:" + busyboxTag
}

// mokkaHelmValues are the Agent values of the Mokka suites, on top of mokka.AgentHelmValues.
// gpuMonitoring.privilegedMode is left off, so the system-probe GPU module (eBPF probes) is not
// enabled: Mokka does not execute CUDA, so there would be nothing for it to observe.
const mokkaHelmValues = `
datadog:
  kubelet:
    tlsVerify: false
  logLevel: DEBUG
`

// chartMirrorEnvVar names the environment variable holding the registry that mirrors ghcr.io and
// registry.k8s.io, e.g. the internal ECR, whose pull-through caches use the ghcr and k8s prefixes.
// When it is unset, the charts are pulled from the public registries, which the CI runners can't
// reach (ghcr.io is Forbidden), so the Mokka suites only run in a manual CI job until it is set.
const chartMirrorEnvVar = "E2E_GPU_CHART_MIRROR"

// ghcrChartRepository returns the OCI repository of a chart published on ghcr.io under path,
// through the chart mirror when one is configured.
func ghcrChartRepository(path string) string {
	if mirror := os.Getenv(chartMirrorEnvVar); mirror != "" {
		return "oci://" + mirror + "/ghcr/" + path
	}
	return "oci://ghcr.io/" + path
}

// mokkaProfileParams describes the simulated GPUs of a Mokka suite, and any extra kindvm
// options (e.g. a bigger VM).
type mokkaProfileParams struct {
	profile    string
	gpuCount   int
	runOptions []kindvm.RunOption
}

// gpuMokkaK8sProvisioner provisions a kind cluster on a CPU-only EC2 instance, with GPUs
// simulated by Mokka, and the Agent deployed with Helm.
func gpuMokkaK8sProvisioner(params mokkaProfileParams, kubernetesAgentOptions ...kubernetesagentparams.Option) provisioners.TypedProvisioner[environments.Kubernetes] {
	agentOptions := append([]kubernetesagentparams.Option{kubernetesagentparams.WithHelmValues(mokkaHelmValues)}, kubernetesAgentOptions...)

	runOptions := []kindvm.RunOption{
		// TODO: mirror the Mokka (ghcr.io) and device plugin (nvcr.io) images in the internal registry.
		kindvm.WithInternetAccess(),
		kindvm.WithMokka(
			mokka.WithProfile(params.profile),
			mokka.WithGPUCount(params.gpuCount),
			mokka.WithChartRepository(ghcrChartRepository("nvidia/k8s-test-infra/chart")),
		),
		kindvm.WithAgentOptions(agentOptions...),
	}
	runOptions = append(runOptions, params.runOptions...)

	return awskubernetes.Provisioner(awskubernetes.WithRunOptions(runOptions...))
}
