// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package admissioncontroller

import (
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	scenarioeks "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/eks"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	proveks "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/eks"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	provgke "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/gcp/kubernetes"
	localkind "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/local/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
)

// ProvisionerType represents the type of Kubernetes provisioner to use.
type ProvisionerType string

const (
	// ProvisionerKindAWS uses Kind running on an AWS VM (default).
	ProvisionerKindAWS ProvisionerType = "kind"
	// ProvisionerKindLocal uses Kind running locally.
	ProvisionerKindLocal ProvisionerType = "kind-local"
	// ProvisionerEKS uses Amazon EKS.
	ProvisionerEKS ProvisionerType = "eks"
	// ProvisionerGKE uses Google Kubernetes Engine.
	ProvisionerGKE ProvisionerType = "gke"
)

// ProvisionerOptions contains common options for Kubernetes provisioners.
type ProvisionerOptions struct {
	AgentOptions                  []kubernetesagentparams.Option
	WorkloadAppFunc               kubeComp.WorkloadAppFunc
	AgentDependentWorkloadAppFunc kubeComp.AgentDependentWorkloadAppFunc
}

// Provisioner returns a Kubernetes provisioner based on E2E_PROVISIONER and E2E_DEV_LOCAL parameters.
func Provisioner(opts ProvisionerOptions) provisioners.TypedProvisioner[environments.Kubernetes] {
	switch getProvisionerType() {
	case ProvisionerKindLocal:
		return localKindProvisioner(opts)
	case ProvisionerEKS:
		return eksProvisioner(opts)
	case ProvisionerGKE:
		return gkeProvisioner(opts)
	default:
		return awsKindProvisioner(opts)
	}
}

func getProvisionerType() ProvisionerType {
	provisioner, err := runner.GetProfile().ParamStore().GetWithDefault(parameters.Provisioner, "")
	if err == nil && provisioner != "" {
		return ProvisionerType(strings.ToLower(provisioner))
	}
	devLocal, err := runner.GetProfile().ParamStore().GetBoolWithDefault(parameters.DevLocal, false)
	if err == nil && devLocal {
		return ProvisionerKindLocal
	}
	return ProvisionerKindAWS
}

func localKindProvisioner(opts ProvisionerOptions) provisioners.TypedProvisioner[environments.Kubernetes] {
	var localOpts []localkind.ProvisionerOption
	if len(opts.AgentOptions) > 0 {
		localOpts = append(localOpts, localkind.WithAgentOptions(opts.AgentOptions...))
	}
	if opts.WorkloadAppFunc != nil {
		localOpts = append(localOpts, localkind.WithWorkloadApp(opts.WorkloadAppFunc))
	}
	if opts.AgentDependentWorkloadAppFunc != nil {
		localOpts = append(localOpts, localkind.WithAgentDependentWorkloadApp(opts.AgentDependentWorkloadAppFunc))
	}
	return localkind.Provisioner(localOpts...)
}

func awsKindProvisioner(opts ProvisionerOptions) provisioners.TypedProvisioner[environments.Kubernetes] {
	var runOpts []kindvm.RunOption
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

func eksProvisioner(opts ProvisionerOptions) provisioners.TypedProvisioner[environments.Kubernetes] {
	var runOpts []scenarioeks.RunOption
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

func gkeProvisioner(opts ProvisionerOptions) provisioners.TypedProvisioner[environments.Kubernetes] {
	var gkeOpts []provgke.ProvisionerOption
	if len(opts.AgentOptions) > 0 {
		gkeOpts = append(gkeOpts, provgke.WithAgentOptions(opts.AgentOptions...))
	}
	if opts.WorkloadAppFunc != nil {
		gkeOpts = append(gkeOpts, provgke.WithWorkloadApp(provgke.WorkloadAppFunc(opts.WorkloadAppFunc)))
	}
	if opts.AgentDependentWorkloadAppFunc != nil {
		gkeOpts = append(gkeOpts, provgke.WithAgentDependentWorkloadApp(opts.AgentDependentWorkloadAppFunc))
	}
	return provgke.GKEProvisioner(gkeOpts...)
}
