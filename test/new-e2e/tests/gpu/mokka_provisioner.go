// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package gpu

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes/mokka"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	awskubernetes "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
)

// mokkaHelmValues are the Agent values of the Mokka suites, on top of mokka.AgentHelmValues.
// gpuMonitoring.privilegedMode is left off, so the system-probe GPU module (eBPF probes) is not
// enabled: Mokka does not execute CUDA, so there would be nothing for it to observe.
const mokkaHelmValues = `
datadog:
  kubelet:
    tlsVerify: false
  logLevel: DEBUG
`

// mokkaProfileParams describes the simulated GPUs of a Mokka suite, and any extra kindvm
// options (e.g. a scheduler such as kindvm.WithKAIScheduler, or a bigger VM).
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
		),
		kindvm.WithAgentOptions(agentOptions...),
	}
	runOptions = append(runOptions, params.runOptions...)

	return awskubernetes.Provisioner(awskubernetes.WithRunOptions(runOptions...))
}
