// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package openshiftvm

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/local"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/openshift"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"
)

// Params is a set of parameters for the local OpenShift VM environment.
type Params struct {
	openshift.Params
	deployFakeIntake bool
}

func newParams() *Params {
	return &Params{
		Params: openshift.Params{
			AgentOptions: []kubernetesagentparams.Option{
				kubernetesagentparams.WithHelmValues(localOpenShiftAgentHelmValues),
				kubernetesagentparams.WithOpenShiftControlPlaneMonitoring(),
			},
		},
		deployFakeIntake: true,
	}
}

// GetParams builds Params from the given options.
func GetParams(opts ...Option) *Params {
	params := newParams()
	err := optional.ApplyOptions(params, opts)
	if err != nil {
		panic(fmt.Errorf("unable to apply Option, err: %w", err))
	}
	return params
}

// Option is a provisioner option.
type Option func(*Params) error

// WithAgentOptions adds options to the Agent.
func WithAgentOptions(opts ...kubernetesagentparams.Option) Option {
	return func(p *Params) error {
		return openshift.WithAgentOptions(opts...)(&p.Params)
	}
}

// WithoutFakeIntake disables the creation of the FakeIntake.
func WithoutFakeIntake() Option {
	return func(p *Params) error {
		p.deployFakeIntake = false
		return nil
	}
}

// WithoutAgent disables the creation of the Agent.
func WithoutAgent() Option {
	return func(p *Params) error {
		return openshift.WithoutAgent()(&p.Params)
	}
}

// ParamsFromEnvironment builds Params by reading configuration from the given local environment.
func ParamsFromEnvironment(e local.Environment) *Params {
	p := newParams()
	p.PullSecretPath = e.OpenShiftPullSecretPath()
	p.CPUs = e.OpenShiftCPUs()
	p.Memory = e.OpenShiftMemory()
	p.Disk = e.OpenShiftDisk()
	openshift.ApplyAgentEnvironment(&p.Params, &e)
	if p.AgentOptions != nil && e.AgentUseDualShipping() {
		p.AgentOptions = append(p.AgentOptions, kubernetesagentparams.WithDualShipping())
	}
	p.deployFakeIntake = e.AgentUseFakeintake()
	return p
}
