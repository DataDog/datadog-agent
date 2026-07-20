// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package kindprovisioner selects between the AWS-VM-backed and local Kind provisioners
// based on E2E_PROVISIONER / E2E_DEV_LOCAL, so test suites don't have to duplicate the
// switch logic. See test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm and
// test/e2e-framework/testing/provisioners/local/kubernetes for the underlying provisioners.
package kindprovisioner

import (
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	provlocal "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/local/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
)

// Options are the options shared between the AWS-VM-backed and local Kind provisioners.
// AWSVMOptions only applies to the AWS backend and is ignored when running locally.
type Options struct {
	Name                          string
	AgentOptions                  []kubernetesagentparams.Option
	FakeintakeOptions             []fakeintake.Option
	WorkloadAppFunc               kubeComp.WorkloadAppFunc
	AgentDependentWorkloadAppFunc kubeComp.AgentDependentWorkloadAppFunc
	DeployDogstatsd               bool
	DeployTestWorkload            bool
	DeployArgoRollout             bool
	AWSVMOptions                  []ec2.VMOption
}

// Provisioner returns a Kind provisioner selected via E2E_PROVISIONER=kind-local or
// E2E_DEV_LOCAL=true (local Docker daemon), defaulting to the AWS-VM-backed Kind provisioner.
func Provisioner(opts Options) provisioners.TypedProvisioner[environments.Kubernetes] {
	if isLocalMode() {
		return localProvisioner(opts)
	}
	return awsProvisioner(opts)
}

// isLocalMode returns true when E2E_PROVISIONER=kind-local or E2E_DEV_LOCAL=true.
func isLocalMode() bool {
	provisioner, err := runner.GetProfile().ParamStore().GetWithDefault(parameters.Provisioner, "")
	if err == nil && strings.ToLower(provisioner) == "kind-local" {
		return true
	}

	devLocal, err := runner.GetProfile().ParamStore().GetBoolWithDefault(parameters.DevLocal, false)
	return err == nil && devLocal
}

func awsProvisioner(opts Options) provisioners.TypedProvisioner[environments.Kubernetes] {
	var runOpts []kindvm.RunOption
	if opts.Name != "" {
		runOpts = append(runOpts, kindvm.WithName(opts.Name))
	}
	if len(opts.AWSVMOptions) > 0 {
		runOpts = append(runOpts, kindvm.WithVMOptions(opts.AWSVMOptions...))
	}
	if len(opts.AgentOptions) > 0 {
		runOpts = append(runOpts, kindvm.WithAgentOptions(opts.AgentOptions...))
	}
	if len(opts.FakeintakeOptions) > 0 {
		runOpts = append(runOpts, kindvm.WithFakeintakeOptions(opts.FakeintakeOptions...))
	}
	if opts.WorkloadAppFunc != nil {
		runOpts = append(runOpts, kindvm.WithWorkloadApp(opts.WorkloadAppFunc))
	}
	if opts.AgentDependentWorkloadAppFunc != nil {
		runOpts = append(runOpts, kindvm.WithAgentDependentWorkloadApp(opts.AgentDependentWorkloadAppFunc))
	}
	if opts.DeployDogstatsd {
		runOpts = append(runOpts, kindvm.WithDeployDogstatsd())
	}
	if opts.DeployTestWorkload {
		runOpts = append(runOpts, kindvm.WithDeployTestWorkload())
	}
	if opts.DeployArgoRollout {
		runOpts = append(runOpts, kindvm.WithDeployArgoRollout())
	}
	return provkindvm.Provisioner(provkindvm.WithRunOptions(runOpts...))
}

func localProvisioner(opts Options) provisioners.TypedProvisioner[environments.Kubernetes] {
	var localOpts []provlocal.ProvisionerOption
	if opts.Name != "" {
		localOpts = append(localOpts, provlocal.WithName(opts.Name))
	}
	if len(opts.AgentOptions) > 0 {
		localOpts = append(localOpts, provlocal.WithAgentOptions(opts.AgentOptions...))
	}
	if len(opts.FakeintakeOptions) > 0 {
		localOpts = append(localOpts, provlocal.WithFakeintakeOptions(opts.FakeintakeOptions...))
	}
	if opts.WorkloadAppFunc != nil {
		localOpts = append(localOpts, provlocal.WithWorkloadApp(opts.WorkloadAppFunc))
	}
	if opts.AgentDependentWorkloadAppFunc != nil {
		localOpts = append(localOpts, provlocal.WithAgentDependentWorkloadApp(opts.AgentDependentWorkloadAppFunc))
	}
	if opts.DeployDogstatsd {
		localOpts = append(localOpts, provlocal.WithDeployDogstatsd())
	}
	if opts.DeployTestWorkload {
		localOpts = append(localOpts, provlocal.WithDeployTestWorkload())
	}
	if opts.DeployArgoRollout {
		localOpts = append(localOpts, provlocal.WithDeployArgoRollout())
	}
	return provlocal.Provisioner(localOpts...)
}
