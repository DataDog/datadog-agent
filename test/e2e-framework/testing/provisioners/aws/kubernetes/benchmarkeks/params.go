// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package benchmarkeks

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/benchmarkeks"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"
)

// ProvisionerParams contains all the parameters needed to create the environment
type ProvisionerParams struct {
	awsEnv            *aws.Environment
	runOptions        []benchmarkeks.RunOption
	extraConfigParams runner.ConfigMap
}

func getProvisionerParams(opts ...ProvisionerOption) *ProvisionerParams {
	params := &ProvisionerParams{
		awsEnv:            nil,
		runOptions:        []benchmarkeks.RunOption{},
		extraConfigParams: runner.ConfigMap{},
	}
	err := optional.ApplyOptions(params, opts)
	if err != nil {
		panic(fmt.Errorf("unable to apply ProvisionerOption, err: %w", err))
	}
	return params
}

// ProvisionerOption is a function that modifies the ProvisionerParams
type ProvisionerOption func(*ProvisionerParams) error

// WithAwsEnv asks the provisioner to use the given environment, it is created otherwise
func WithAwsEnv(env *aws.Environment) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.awsEnv = env
		return nil
	}
}

// WithRunOptions adds options to the benchmarkeks scenario. They take precedence over the
// ddagent:baseline* and ddagent:comparison* config keys.
func WithRunOptions(opts ...benchmarkeks.RunOption) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.runOptions = append(params.runOptions, opts...)
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
