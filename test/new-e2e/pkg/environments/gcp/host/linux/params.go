// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gcphost

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclientparams"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/resources/gcp"
	"github.com/DataDog/test-infra-definitions/scenarios/gcp/compute"
	"github.com/DataDog/test-infra-definitions/scenarios/gcp/fakeintake"
)

// ProvisionerParams is a set of parameters for the Provisioner.
type ProvisionerParams struct {
	name string

	instanceOptions    []compute.VMOption
	agentOptions       []agentparams.Option
	agentClientOptions []agentclientparams.Option
	fakeintakeOptions  []fakeintake.Option
	extraConfigParams  runner.ConfigMap
	installUpdater     bool
}

func newProvisionerParams() *ProvisionerParams {
	// We use nil arrays to decide if we should create or not
	return &ProvisionerParams{
		name:               defaultVMName,
		instanceOptions:    []compute.VMOption{},
		agentOptions:       []agentparams.Option{},
		agentClientOptions: []agentclientparams.Option{},
		fakeintakeOptions:  []fakeintake.Option{},
		extraConfigParams:  runner.ConfigMap{},
	}
}

// GetProvisionerParams return ProvisionerParams from options opts setup
func GetProvisionerParams(opts ...ProvisionerOption) *ProvisionerParams {
	params := newProvisionerParams()
	err := optional.ApplyOptions(params, opts)
	if err != nil {
		panic(fmt.Errorf("unable to apply ProvisionerOption, err: %w", err))
	}
	return params
}

// ProvisionerOption is a provisioner option.
type ProvisionerOption func(*ProvisionerParams) error

// WithName sets the name of the provisioner.
func WithName(name string) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.name = name
		return nil
	}
}

// WithInstanceOptions adds options to the EC2 VM.
func WithInstanceOptions(opts ...compute.VMOption) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.instanceOptions = append(params.instanceOptions, opts...)
		return nil
	}
}

// WithAgentOptions adds options to the Agent.
func WithAgentOptions(opts ...agentparams.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.agentOptions = append(params.agentOptions, opts...)
		return nil
	}
}

// WithAgentClientOptions adds options to the Agent client.
func WithAgentClientOptions(opts ...agentclientparams.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.agentClientOptions = append(params.agentClientOptions, opts...)
		return nil
	}
}

// WithFakeIntakeOptions adds options to the FakeIntake.
func WithFakeIntakeOptions(opts ...fakeintake.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.fakeintakeOptions = append(params.fakeintakeOptions, opts...)
		return nil
	}
}

// WithExtraConfigParams adds extra config parameters to the ConfigMap.
func WithExtraConfigParams(configMap runner.ConfigMap) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.extraConfigParams = configMap
		return nil
	}
}

// WithoutFakeIntake disables the creation of the FakeIntake.
func WithoutFakeIntake() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.fakeintakeOptions = nil
		return nil
	}
}

// WithoutAgent disables the creation of the Agent.
func WithoutAgent() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.agentOptions = nil
		return nil
	}
}

// WithUpdater installs the agent through the updater.
func WithUpdater() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.installUpdater = true
		return nil
	}
}

// ProvisionerNoAgentNoFakeIntake wraps Provisioner with hardcoded WithoutAgent and WithoutFakeIntake options.
func ProvisionerNoAgentNoFakeIntake(opts ...ProvisionerOption) e2e.TypedProvisioner[environments.Host] {
	mergedOpts := make([]ProvisionerOption, 0, len(opts)+2)
	mergedOpts = append(mergedOpts, opts...)
	mergedOpts = append(mergedOpts, WithoutAgent(), WithoutFakeIntake())

	return Provisioner(mergedOpts...)
}

// ProvisionerNoFakeIntake wraps Provisioner with hardcoded WithoutFakeIntake option.
func ProvisionerNoFakeIntake(opts ...ProvisionerOption) e2e.TypedProvisioner[environments.Host] {
	mergedOpts := make([]ProvisionerOption, 0, len(opts)+1)
	mergedOpts = append(mergedOpts, opts...)
	mergedOpts = append(mergedOpts, WithoutFakeIntake())

	return Provisioner(mergedOpts...)
}

// RunParams is a set of parameters for the Run function.
type RunParams struct {
	Environment       *gcp.Environment
	ProvisionerParams *ProvisionerParams
}
