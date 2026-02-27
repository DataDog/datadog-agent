// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ssi

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	localkind "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/local/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
)

// ProvisionerOptions contains the common options for Kubernetes provisioners.
type ProvisionerOptions struct {
	AgentOptions                  []kubernetesagentparams.Option
	WorkloadAppFunc               kubeComp.WorkloadAppFunc
	AgentDependentWorkloadAppFunc kubeComp.AgentDependentWorkloadAppFunc
}

// Provisioner returns a Kubernetes provisioner based on E2E_DEV_LOCAL parameter.
// If E2E_DEV_LOCAL=true, returns a local Kind provisioner for faster development.
// Otherwise, returns an AWS Kind VM provisioner.
func Provisioner(opts ProvisionerOptions) provisioners.TypedProvisioner[environments.Kubernetes] {
	if isLocalMode() {
		return localKindProvisioner(opts)
	}
	return awsKindProvisioner(opts)
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
