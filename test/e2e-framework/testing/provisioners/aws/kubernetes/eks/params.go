// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package awskubernetes contains the provisioner for the Kubernetes based environments
package eks

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/eks"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"
)

// ProvisionerParams contains all the parameters needed to create the environment
type ProvisionerParams struct {
	awsEnv            *aws.Environment
	runOptions        []eks.RunOption
	extraConfigParams runner.ConfigMap
}

// GetProvisionerParams return ProvisionerParams from options opts setup
func getProvisionerParams(opts ...ProvisionerOption) *ProvisionerParams {
	params := &ProvisionerParams{
		awsEnv:            nil,
		runOptions:        []eks.RunOption{},
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

func WithAwsEnv(env *aws.Environment) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.awsEnv = env
		return nil
	}
}

func WithRunOptions(opts ...eks.RunOption) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.runOptions = append(params.runOptions, opts...)
		return nil
	}
}

func WithExtraConfigParams(configMap runner.ConfigMap) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.extraConfigParams = configMap
		return nil
	}
}
