// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package appsecinjection

import (
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	provlocal "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/local/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
)

// ProvisionerType selects the Kubernetes backend for the test.
type ProvisionerType string

const (
	// ProvisionerKindAWS runs KinD on an AWS EC2 VM (default).
	ProvisionerKindAWS ProvisionerType = "kind"
	// ProvisionerKindLocal runs KinD on the local machine.
	ProvisionerKindLocal ProvisionerType = "kind-local"
)

// ProvisionerOptions configures both agent Helm values and the workload deployer.
type ProvisionerOptions struct {
	AgentOptions []kubernetesagentparams.Option

	// WorkloadFunc is called after the agent is deployed.  It installs Envoy
	// Gateway and applies the AppSec UDS manifests.
	WorkloadFunc kubeComp.AgentDependentWorkloadAppFunc
}

// Provisioner returns a Kubernetes provisioner driven by the E2E_PROVISIONER
// environment variable / parameter.  Defaults to KinD on EC2.
func Provisioner(opts ProvisionerOptions) provisioners.TypedProvisioner[environments.Kubernetes] {
	switch getProvisionerType() {
	case ProvisionerKindLocal:
		return kindLocalProvisioner(opts)
	default:
		return kindProvisioner(opts)
	}
}

func getProvisionerType() ProvisionerType {
	val, err := runner.GetProfile().ParamStore().GetWithDefault(parameters.Provisioner, "")
	if err == nil && strings.ToLower(val) == string(ProvisionerKindLocal) {
		return ProvisionerKindLocal
	}
	devLocal, _ := runner.GetProfile().ParamStore().GetBoolWithDefault(parameters.DevLocal, false)
	if devLocal {
		return ProvisionerKindLocal
	}
	return ProvisionerKindAWS
}

func kindProvisioner(opts ProvisionerOptions) provisioners.TypedProvisioner[environments.Kubernetes] {
	var runOpts []kindvm.RunOption
	if len(opts.AgentOptions) > 0 {
		runOpts = append(runOpts, kindvm.WithAgentOptions(opts.AgentOptions...))
	}
	if opts.WorkloadFunc != nil {
		runOpts = append(runOpts, kindvm.WithAgentDependentWorkloadApp(opts.WorkloadFunc))
	}
	return provkindvm.Provisioner(provkindvm.WithRunOptions(runOpts...))
}

func kindLocalProvisioner(opts ProvisionerOptions) provisioners.TypedProvisioner[environments.Kubernetes] {
	var localOpts []provlocal.ProvisionerOption
	if len(opts.AgentOptions) > 0 {
		localOpts = append(localOpts, provlocal.WithAgentOptions(opts.AgentOptions...))
	}
	if opts.WorkloadFunc != nil {
		localOpts = append(localOpts, provlocal.WithAgentDependentWorkloadApp(opts.WorkloadFunc))
	}
	return provlocal.Provisioner(localOpts...)
}
