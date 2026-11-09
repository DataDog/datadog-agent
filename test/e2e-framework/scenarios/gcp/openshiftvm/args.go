// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package openshiftvm

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	resGcp "github.com/DataDog/datadog-agent/test/e2e-framework/resources/gcp"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/gcp/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/openshift"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"
)

// Params is a set of parameters for the GCP OpenShift VM environment.
type Params struct {
	openshift.Params
	fakeintakeOptions []fakeintake.Option
}

func newParams() *Params {
	return &Params{
		Params:            openshift.Params{AgentOptions: []kubernetesagentparams.Option{}},
		fakeintakeOptions: []fakeintake.Option{fakeintake.WithMemory(2048)},
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

// WithFakeIntakeOptions adds options to the FakeIntake.
func WithFakeIntakeOptions(opts ...fakeintake.Option) Option {
	return func(p *Params) error {
		p.fakeintakeOptions = append(p.fakeintakeOptions, opts...)
		return nil
	}
}

// WithoutFakeIntake disables the creation of the FakeIntake.
func WithoutFakeIntake() Option {
	return func(p *Params) error {
		p.fakeintakeOptions = nil
		return nil
	}
}

// WithoutAgent disables the creation of the Agent.
func WithoutAgent() Option {
	return func(p *Params) error {
		return openshift.WithoutAgent()(&p.Params)
	}
}

// ParamsFromEnvironment builds Params by reading configuration from the given GCP environment.
func ParamsFromEnvironment(e resGcp.Environment) *Params {
	p := newParams()

	p.PullSecretPath = e.OpenShiftPullSecretPath()
	p.CPUs = e.OpenShiftCPUs()
	p.Memory = e.OpenShiftMemory()
	p.Disk = e.OpenShiftDisk()

	openshift.ApplyAgentEnvironment(&p.Params, &e)
	if p.AgentOptions != nil && e.AgentUseDualShipping() {
		p.AgentOptions = append(p.AgentOptions, kubernetesagentparams.WithDualShipping())
	}

	if e.AgentUseFakeintake() {
		if e.InfraShouldDeployFakeintakeWithLB() {
			p.fakeintakeOptions = append(p.fakeintakeOptions, fakeintake.WithLoadBalancer())
		}
		if e.AgentUseDualShipping() {
			p.fakeintakeOptions = append(p.fakeintakeOptions, fakeintake.WithoutDDDevForwarding())
		}
		if retention := e.AgentFakeintakeRetentionPeriod(); retention != "" {
			p.fakeintakeOptions = append(p.fakeintakeOptions, fakeintake.WithRetentionPeriod(retention))
		}
	} else {
		p.fakeintakeOptions = nil
	}

	return p
}
