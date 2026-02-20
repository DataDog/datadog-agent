// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package gcpopenshiftvm contains the provisioner for OpenShift VM on GCP
package gcpopenshiftvm

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/gcp/fakeintake"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// ProvisionerParams contains all the parameters needed to create the environment
type ProvisionerParams struct {
	name              string
	fakeintakeOptions []fakeintake.Option
	agentOptions      []kubernetesagentparams.Option
	openshiftOptions  []pulumi.ResourceOption
	deployArgoRollout bool
	extraConfigParams runner.ConfigMap
}

func newProvisionerParams(opts ...ProvisionerOption) *ProvisionerParams {
	params := &ProvisionerParams{
		name:              "openshiftvm",
		fakeintakeOptions: []fakeintake.Option{},
		agentOptions:      []kubernetesagentparams.Option{},
	}
	err := optional.ApplyOptions(params, opts)
	if err != nil {
		panic(fmt.Sprintf("failed to apply options: %v", err))
	}
	return params
}

// ProvisionerOption is a function that modifies the ProvisionerParams
type ProvisionerOption func(*ProvisionerParams) error

// WithName sets the name of the provisioner
func WithName(name string) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.name = name
		return nil
	}
}

// WithAgentOptions adds options to the agent
func WithAgentOptions(opts ...kubernetesagentparams.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.agentOptions = opts
		return nil
	}
}

// WithFakeIntakeOptions adds options to the fake intake
func WithFakeIntakeOptions(opts ...fakeintake.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.fakeintakeOptions = opts
		return nil
	}
}

// WithOpenShiftOptions adds options to the OpenShift cluster
func WithOpenShiftOptions(opts ...pulumi.ResourceOption) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.openshiftOptions = opts
		return nil
	}
}

// WithExtraConfigParams adds extra config parameters to the environment
func WithExtraConfigParams(configMap runner.ConfigMap) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.extraConfigParams = configMap
		return nil
	}
}

func WithDeployArgoRollout() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.deployArgoRollout = true
		return nil
	}
}
