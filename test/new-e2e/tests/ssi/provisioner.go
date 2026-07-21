// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ssi

import (
	"strings"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	scenarioeks "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/eks"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	scenariogke "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/gcp/gke"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	proveks "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/eks"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	provaks "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/azure/kubernetes"
	provgke "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/gcp/kubernetes"
	provopenshift "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/gcp/kubernetes/openshiftvm"
	provlocal "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/local/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
)

// ProvisionerType represents the type of Kubernetes provisioner to use.
type ProvisionerType string

// PreAgentHook lets tests inject setup after the provider is ready and before agent installation.
type PreAgentHook func(e config.Env, kubeProvider *kubernetes.Provider) error

const (
	// ProvisionerKindAWS uses Kind running on an AWS VM (default).
	ProvisionerKindAWS ProvisionerType = "kind"
	// ProvisionerKindLocal uses Kind running locally.
	ProvisionerKindLocal ProvisionerType = "kind-local"
	// ProvisionerEKS uses Amazon EKS.
	ProvisionerEKS ProvisionerType = "eks"
	// ProvisionerAKS uses Azure Kubernetes Service.
	ProvisionerAKS ProvisionerType = "aks"
	// ProvisionerGKE uses Google Kubernetes Engine.
	ProvisionerGKE ProvisionerType = "gke"
	// ProvisionerGKEAutopilot uses Google Kubernetes Engine in Autopilot mode.
	ProvisionerGKEAutopilot ProvisionerType = "gke-autopilot"
	// ProvisionerOpenShift uses OpenShift VM on GCP.
	ProvisionerOpenShift ProvisionerType = "openshift"
	// ProvisionerOpenShiftLocal uses local OpenShift (CRC).
	ProvisionerOpenShiftLocal ProvisionerType = "openshift-local"
)

// ProvisionerOptions contains the common options for Kubernetes provisioners.
type ProvisionerOptions struct {
	AgentOptions                  []kubernetesagentparams.Option
	PreAgentHook                  PreAgentHook
	WorkloadAppFunc               kubeComp.WorkloadAppFunc
	AgentDependentWorkloadAppFunc kubeComp.AgentDependentWorkloadAppFunc
}

// Provisioner returns a Kubernetes provisioner based on E2E_PROVISIONER and E2E_DEV_LOCAL parameters.
// Supported provisioners: "kind" (default), "kind-local", "eks", "gke", "gke-autopilot", "aks", "openshift", "openshift-local".
// E2E_DEV_LOCAL=true is a shortcut for E2E_PROVISIONER=kind-local.
func Provisioner(opts ProvisionerOptions) provisioners.TypedProvisioner[environments.Kubernetes] {
	provisionerType := getProvisionerType()
	switch provisionerType {
	case ProvisionerKindLocal:
		return kindLocalProvisioner(opts)
	case ProvisionerEKS:
		return eksProvisioner(opts)
	case ProvisionerAKS:
		return aksProvisioner(opts)
	case ProvisionerGKE:
		return gkeProvisioner(opts, false)
	case ProvisionerGKEAutopilot:
		return gkeProvisioner(opts, true)
	case ProvisionerOpenShift:
		return openShiftProvisioner(opts)
	case ProvisionerOpenShiftLocal:
		return openShiftLocalProvisioner(opts)
	default:
		return kindProvisioner(opts)
	}
}

// getProvisionerType returns the provisioner type from E2E_PROVISIONER parameter or E2E_DEV_LOCAL.
func getProvisionerType() ProvisionerType {
	name := "kind"

	// Check E2E_PROVISIONER first (via env var or config file).
	provisioner, err := runner.GetProfile().ParamStore().GetWithDefault(parameters.Provisioner, "")
	if err == nil && provisioner != "" {
		name = strings.ToLower(provisioner)
	}

	// Check E2E_DEV_LOCAL for compatible provisioners
	if isLocalMode() {
		switch name {
		case "kind":
			return ProvisionerKindLocal
		case "openshift":
			return ProvisionerOpenShiftLocal
		}
	}

	return ProvisionerType(name)
}

// isLocalMode returns true if E2E_DEV_LOCAL is set to "true" (via env var or config file)
func isLocalMode() bool {
	devLocal, err := runner.GetProfile().ParamStore().GetBoolWithDefault(parameters.DevLocal, false)
	if err != nil {
		return false
	}
	return devLocal
}

// kindLocalProvisioner returns a local Kind provisioner
func kindLocalProvisioner(opts ProvisionerOptions) provisioners.TypedProvisioner[environments.Kubernetes] {
	var localOpts []provlocal.ProvisionerOption
	if len(opts.AgentOptions) > 0 {
		localOpts = append(localOpts, provlocal.WithAgentOptions(opts.AgentOptions...))
	}
	if opts.WorkloadAppFunc != nil {
		localOpts = append(localOpts, provlocal.WithWorkloadApp(opts.WorkloadAppFunc))
	}
	if opts.AgentDependentWorkloadAppFunc != nil {
		localOpts = append(localOpts, provlocal.WithAgentDependentWorkloadApp(opts.AgentDependentWorkloadAppFunc))
	}
	return provlocal.Provisioner(localOpts...)
}

// kindProvisioner returns an AWS Kind VM provisioner
func kindProvisioner(opts ProvisionerOptions) provisioners.TypedProvisioner[environments.Kubernetes] {
	runOpts := []kindvm.RunOption{
		kindvm.WithFakeintakeOptions(fakeintake.WithMemory(4096)),
	}
	if len(opts.AgentOptions) > 0 {
		runOpts = append(runOpts, kindvm.WithAgentOptions(opts.AgentOptions...))
	}
	if opts.WorkloadAppFunc != nil {
		runOpts = append(runOpts, kindvm.WithWorkloadApp(opts.WorkloadAppFunc))
	}
	if opts.AgentDependentWorkloadAppFunc != nil {
		runOpts = append(runOpts, kindvm.WithAgentDependentWorkloadApp(opts.AgentDependentWorkloadAppFunc))
	}
	return provkindvm.Provisioner(provkindvm.WithRunOptions(runOpts...))
}

// eksProvisioner returns an Amazon EKS provisioner.
func eksProvisioner(opts ProvisionerOptions) provisioners.TypedProvisioner[environments.Kubernetes] {
	var runOpts []scenarioeks.RunOption

	// Add a Linux node group by default.
	runOpts = append(runOpts, scenarioeks.WithEKSOptions(scenarioeks.WithLinuxNodeGroup()))

	if len(opts.AgentOptions) > 0 {
		runOpts = append(runOpts, scenarioeks.WithAgentOptions(opts.AgentOptions...))
	}
	if opts.WorkloadAppFunc != nil {
		runOpts = append(runOpts, scenarioeks.WithWorkloadApp(opts.WorkloadAppFunc))
	}
	if opts.AgentDependentWorkloadAppFunc != nil {
		runOpts = append(runOpts, scenarioeks.WithAgentDependentWorkloadApp(opts.AgentDependentWorkloadAppFunc))
	}
	return proveks.Provisioner(proveks.WithRunOptions(runOpts...))
}

// autopilotHelmValues adapts the agent install to GKE Autopilot's constraints, following
// https://docs.datadoghq.com/containers/kubernetes/distributions/#autopilot:
//   - admissionController.configMode "service": the default "hostip" mode points workloads at
//     the node IP, which requires the agent DaemonSet to expose the trace-agent on a fixed
//     hostPort, and the "socket" mode mounts the UDS through a writable hostPath volume.
//     Autopilot disallows both (fixed hostPorts and write-mode hostPath volumes, on top of
//     hostNetwork), so traces injected by SSI can only reach the trace-agent through cluster
//     networking via the agent's service DNS name.
//   - apm.portEnabled: expose the trace-agent TCP port (8126) on the agent's local service so
//     that the service DNS name injected in "service" mode actually resolves to an open port.
//     This uses a container port (no hostPort), so it stays Autopilot-compatible.
//   - kubelet.useApiServer: query the pod list from the API server instead of the deprecated
//     read-only kubelet port, which Datadog recommends on Autopilot.
//   - container resources requests: Autopilot's low default limit (50m CPU, 100Mi memory) can
//     OOMKill the agent and trace-agent, which breaks trace ingestion in the SSI tests.
//   - priorityClassCreate: ensure the agent gets scheduled on Autopilot.
const autopilotHelmValues = `
datadog:
  apm:
    portEnabled: true
  kubelet:
    useApiServer: true
clusterAgent:
  admissionController:
    configMode: service
agents:
  priorityClassCreate: true
  containers:
    agent:
      resources:
        requests:
          cpu: 200m
          memory: 256Mi
    traceAgent:
      resources:
        requests:
          cpu: 100m
          memory: 200Mi
    processAgent:
      resources:
        requests:
          cpu: 100m
          memory: 200Mi
    systemProbe:
      resources:
        requests:
          cpu: 100m
          memory: 400Mi
`

// gkeProvisioner returns a Google Kubernetes Engine provisioner.
// When autopilot is true, the cluster is created in Autopilot mode and the agent is
// installed with Autopilot-compatible Helm values.
func gkeProvisioner(opts ProvisionerOptions, autopilot bool) provisioners.TypedProvisioner[environments.Kubernetes] {
	var gkeOpts []provgke.ProvisionerOption

	agentOptions := opts.AgentOptions
	if autopilot {
		gkeOpts = append(gkeOpts, provgke.WithGKEOptions(scenariogke.WithAutopilot()))
		// Prepend the Autopilot agent option so the Helm install only keeps Autopilot-compatible values.
		agentOptions = append([]kubernetesagentparams.Option{kubernetesagentparams.WithGKEAutopilot()}, agentOptions...)
		// Append an override last so it wins over the suite's configMode (which uses "hostip").
		agentOptions = append(agentOptions, kubernetesagentparams.WithHelmValues(autopilotHelmValues))
	}

	if len(agentOptions) > 0 {
		gkeOpts = append(gkeOpts, provgke.WithAgentOptions(agentOptions...))
	}
	if opts.WorkloadAppFunc != nil {
		gkeOpts = append(gkeOpts, provgke.WithWorkloadApp(provgke.WorkloadAppFunc(opts.WorkloadAppFunc)))
	}
	if opts.AgentDependentWorkloadAppFunc != nil {
		gkeOpts = append(gkeOpts, provgke.WithAgentDependentWorkloadApp(opts.AgentDependentWorkloadAppFunc))
	}
	return provgke.GKEProvisioner(gkeOpts...)
}

// openShiftProvisioner returns an OpenShift VM provisioner on GCP.
func openShiftProvisioner(opts ProvisionerOptions) provisioners.TypedProvisioner[environments.Kubernetes] {
	var openShiftOpts []provopenshift.ProvisionerOption

	agentOpts := append([]kubernetesagentparams.Option{}, opts.AgentOptions...)
	openShiftOpts = append(openShiftOpts, provopenshift.WithAgentOptions(agentOpts...))
	if opts.PreAgentHook != nil {
		openShiftOpts = append(openShiftOpts, provopenshift.WithPreAgentHook(provopenshift.PreAgentHook(opts.PreAgentHook)))
	}
	if opts.WorkloadAppFunc != nil {
		openShiftOpts = append(openShiftOpts, provopenshift.WithWorkloadApp(provopenshift.WorkloadAppFunc(opts.WorkloadAppFunc)))
	}
	if opts.AgentDependentWorkloadAppFunc != nil {
		openShiftOpts = append(openShiftOpts, provopenshift.WithAgentDependentWorkloadApp(opts.AgentDependentWorkloadAppFunc))
	}
	return provopenshift.OpenshiftVMProvisioner(openShiftOpts...)
}

// openShiftLocalProvisioner returns a local OpenShift (CRC) provisioner.
func openShiftLocalProvisioner(opts ProvisionerOptions) provisioners.TypedProvisioner[environments.Kubernetes] {
	var openShiftOpts []provlocal.ProvisionerOption

	agentOpts := append([]kubernetesagentparams.Option{}, opts.AgentOptions...)
	openShiftOpts = append(openShiftOpts, provlocal.WithAgentOptions(agentOpts...))
	if opts.PreAgentHook != nil {
		openShiftOpts = append(openShiftOpts, provlocal.WithPreAgentHook(provlocal.PreAgentHook(opts.PreAgentHook)))
	}
	if opts.WorkloadAppFunc != nil {
		openShiftOpts = append(openShiftOpts, provlocal.WithWorkloadApp(opts.WorkloadAppFunc))
	}
	if opts.AgentDependentWorkloadAppFunc != nil {
		openShiftOpts = append(openShiftOpts, provlocal.WithAgentDependentWorkloadApp(opts.AgentDependentWorkloadAppFunc))
	}
	return provlocal.OpenShiftLocalProvisioner(openShiftOpts...)
}

// aksProvisioner returns an Azure Kubernetes Service provisioner.
func aksProvisioner(opts ProvisionerOptions) provisioners.TypedProvisioner[environments.Kubernetes] {
	var aksOpts []provaks.ProvisionerOption

	if len(opts.AgentOptions) > 0 {
		aksOpts = append(aksOpts, provaks.WithAgentOptions(opts.AgentOptions...))
	}
	if opts.WorkloadAppFunc != nil {
		aksOpts = append(aksOpts, provaks.WithWorkloadApp(provaks.WorkloadAppFunc(opts.WorkloadAppFunc)))
	}
	if opts.AgentDependentWorkloadAppFunc != nil {
		aksOpts = append(aksOpts, provaks.WithAgentDependentWorkloadApp(opts.AgentDependentWorkloadAppFunc))
	}
	return provaks.AKSProvisioner(aksOpts...)
}
