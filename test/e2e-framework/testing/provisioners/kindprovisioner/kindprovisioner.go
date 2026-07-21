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
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"
)

// options are the options shared between the AWS-VM-backed and local Kind provisioners.
// awsVMOptions only applies to the AWS backend and is ignored when running locally.
type options struct {
	name                          string
	agentOptions                  []kubernetesagentparams.Option
	fakeintakeOptions             []fakeintake.Option
	workloadAppFunc               kubeComp.WorkloadAppFunc
	agentDependentWorkloadAppFunc kubeComp.AgentDependentWorkloadAppFunc
	deployDogstatsd               bool
	deployTestWorkload            bool
	deployArgoRollout             bool
	awsVMOptions                  []ec2.VMOption
}

// Option configures the Kind provisioner returned by Provisioner.
type Option func(*options) error

// WithName sets the provisioner/stack name.
func WithName(name string) Option {
	return func(o *options) error { o.name = name; return nil }
}

// WithAgentOptions sets the Agent configuration options.
func WithAgentOptions(opts ...kubernetesagentparams.Option) Option {
	return func(o *options) error { o.agentOptions = append(o.agentOptions, opts...); return nil }
}

// WithFakeintakeOptions sets the FakeIntake configuration options.
func WithFakeintakeOptions(opts ...fakeintake.Option) Option {
	return func(o *options) error { o.fakeintakeOptions = append(o.fakeintakeOptions, opts...); return nil }
}

// WithWorkloadApp deploys the given workload app.
func WithWorkloadApp(f kubeComp.WorkloadAppFunc) Option {
	return func(o *options) error { o.workloadAppFunc = f; return nil }
}

// WithAgentDependentWorkloadApp deploys the given agent-dependent workload app.
func WithAgentDependentWorkloadApp(f kubeComp.AgentDependentWorkloadAppFunc) Option {
	return func(o *options) error { o.agentDependentWorkloadAppFunc = f; return nil }
}

// WithDeployDogstatsd deploys the dogstatsd standalone workload.
func WithDeployDogstatsd() Option {
	return func(o *options) error { o.deployDogstatsd = true; return nil }
}

// WithDeployTestWorkload deploys the test workload.
func WithDeployTestWorkload() Option {
	return func(o *options) error { o.deployTestWorkload = true; return nil }
}

// WithDeployArgoRollout deploys the Argo Rollout workload.
func WithDeployArgoRollout() Option {
	return func(o *options) error { o.deployArgoRollout = true; return nil }
}

// WithAWSVMOptions sets options for the underlying EC2 VM. Only applies to the AWS-VM-backed
// provisioner; ignored when running locally (E2E_PROVISIONER=kind-local).
func WithAWSVMOptions(opts ...ec2.VMOption) Option {
	return func(o *options) error { o.awsVMOptions = append(o.awsVMOptions, opts...); return nil }
}

// Provisioner returns a Kind provisioner selected via E2E_PROVISIONER=kind-local or
// E2E_DEV_LOCAL=true (local Docker daemon), defaulting to the AWS-VM-backed Kind provisioner.
func Provisioner(opts ...Option) provisioners.TypedProvisioner[environments.Kubernetes] {
	o := &options{}
	_ = optional.ApplyOptions(o, opts)
	if isLocalMode() {
		return localProvisioner(o)
	}
	return awsProvisioner(o)
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

func awsProvisioner(o *options) provisioners.TypedProvisioner[environments.Kubernetes] {
	var runOpts []kindvm.RunOption
	if o.name != "" {
		runOpts = append(runOpts, kindvm.WithName(o.name))
	}
	if len(o.awsVMOptions) > 0 {
		runOpts = append(runOpts, kindvm.WithVMOptions(o.awsVMOptions...))
	}
	if len(o.agentOptions) > 0 {
		runOpts = append(runOpts, kindvm.WithAgentOptions(o.agentOptions...))
	}
	if len(o.fakeintakeOptions) > 0 {
		runOpts = append(runOpts, kindvm.WithFakeintakeOptions(o.fakeintakeOptions...))
	}
	if o.workloadAppFunc != nil {
		runOpts = append(runOpts, kindvm.WithWorkloadApp(o.workloadAppFunc))
	}
	if o.agentDependentWorkloadAppFunc != nil {
		runOpts = append(runOpts, kindvm.WithAgentDependentWorkloadApp(o.agentDependentWorkloadAppFunc))
	}
	if o.deployDogstatsd {
		runOpts = append(runOpts, kindvm.WithDeployDogstatsd())
	}
	if o.deployTestWorkload {
		runOpts = append(runOpts, kindvm.WithDeployTestWorkload())
	}
	if o.deployArgoRollout {
		runOpts = append(runOpts, kindvm.WithDeployArgoRollout())
	}
	return provkindvm.Provisioner(provkindvm.WithRunOptions(runOpts...))
}

func localProvisioner(o *options) provisioners.TypedProvisioner[environments.Kubernetes] {
	var localOpts []provlocal.ProvisionerOption
	if o.name != "" {
		localOpts = append(localOpts, provlocal.WithName(o.name))
	}
	if len(o.agentOptions) > 0 {
		localOpts = append(localOpts, provlocal.WithAgentOptions(o.agentOptions...))
	}
	if len(o.fakeintakeOptions) > 0 {
		localOpts = append(localOpts, provlocal.WithFakeintakeOptions(o.fakeintakeOptions...))
	}
	if o.workloadAppFunc != nil {
		localOpts = append(localOpts, provlocal.WithWorkloadApp(o.workloadAppFunc))
	}
	if o.agentDependentWorkloadAppFunc != nil {
		localOpts = append(localOpts, provlocal.WithAgentDependentWorkloadApp(o.agentDependentWorkloadAppFunc))
	}
	if o.deployDogstatsd {
		localOpts = append(localOpts, provlocal.WithDeployDogstatsd())
	}
	if o.deployTestWorkload {
		localOpts = append(localOpts, provlocal.WithDeployTestWorkload())
	}
	if o.deployArgoRollout {
		localOpts = append(localOpts, provlocal.WithDeployArgoRollout())
	}
	return provlocal.Provisioner(localOpts...)
}
