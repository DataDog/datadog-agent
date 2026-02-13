// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ssi

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
	localkind "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/local/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
)

// ProvisionerType represents the type of Kubernetes provisioner to use
type ProvisionerType string

const (
	// ProvisionerKindAWS uses Kind running on an AWS VM (default)
	ProvisionerKindAWS ProvisionerType = "kind"
	// ProvisionerKindLocal uses Kind running locally
	ProvisionerKindLocal ProvisionerType = "kind-local"
	// ProvisionerEKS uses Amazon EKS
	ProvisionerEKS ProvisionerType = "eks"
)

// ProvisionerOptions contains the common options for Kubernetes provisioners
type ProvisionerOptions struct {
	AgentOptions                  []kubernetesagentparams.Option
	WorkloadAppFunc               kubeComp.WorkloadAppFunc
	AgentDependentWorkloadAppFunc kubeComp.AgentDependentWorkloadAppFunc
}

// Provisioner returns a Kubernetes provisioner based on E2E_PROVISIONER and E2E_DEV_LOCAL parameters.
// Supported provisioners: "kind" (default), "kind-local", "eks"
// E2E_DEV_LOCAL=true is a shortcut for E2E_PROVISIONER=kind-local
func Provisioner(opts ProvisionerOptions) provisioners.TypedProvisioner[environments.Kubernetes] {
	provisionerType := getProvisionerType()
	switch provisionerType {
	case ProvisionerKindLocal:
		return localKindProvisioner(opts)
	case ProvisionerEKS:
		return eksProvisioner(opts)
	default:
		return awsKindProvisioner(opts)
	}
}

// getProvisionerType returns the provisioner type from E2E_PROVISIONER parameter or E2E_DEV_LOCAL
func getProvisionerType() ProvisionerType {
	// Check E2E_PROVISIONER first (via env var or config file)
	provisioner, err := runner.GetProfile().ParamStore().GetWithDefault(parameters.Provisioner, "")
	if err == nil && provisioner != "" {
		return ProvisionerType(strings.ToLower(provisioner))
	}
	// Fall back to E2E_DEV_LOCAL for backward compatibility
	if isLocalMode() {
		return ProvisionerKindLocal
	}
	return ProvisionerKindAWS
}

// isLocalMode returns true if E2E_DEV_LOCAL is set to "true" (via env var or config file)
func isLocalMode() bool {
	devLocal, err := runner.GetProfile().ParamStore().GetBoolWithDefault(parameters.DevLocal, false)
	if err != nil {
		return false
	}
	return devLocal
}

// localKindProvisioner returns a local Kind provisioner
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

// awsKindProvisioner returns an AWS Kind VM provisioner
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

// eksProvisioner returns an Amazon EKS provisioner
func eksProvisioner(opts ProvisionerOptions) provisioners.TypedProvisioner[environments.Kubernetes] {
	var runOpts []scenarioeks.RunOption

	// Add a Linux node group by default
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
