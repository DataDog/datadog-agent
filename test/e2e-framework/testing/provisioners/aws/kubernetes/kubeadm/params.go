// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package kubeadm

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kubeadm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"
)

type provisionerParams struct {
	awsEnv            *aws.Environment
	runOptions        []kubeadm.RunOption
	extraConfigParams runner.ConfigMap
}

type provisionerOption func(*provisionerParams) error

func getProvisionerParams(opts ...provisionerOption) *provisionerParams {
	p := &provisionerParams{awsEnv: nil, runOptions: []kubeadm.RunOption{}, extraConfigParams: runner.ConfigMap{}}
	_ = optional.ApplyOptions(p, opts)
	return p
}

// WithAwsEnv sets a pre-built AWS environment.
func WithAwsEnv(env *aws.Environment) provisionerOption {
	return func(p *provisionerParams) error { p.awsEnv = env; return nil }
}

// WithRunOptions sets the scenario run options.
func WithRunOptions(opts ...kubeadm.RunOption) provisionerOption {
	return func(p *provisionerParams) error { p.runOptions = append(p.runOptions, opts...); return nil }
}

// WithExtraConfigParams sets extra Pulumi config params.
func WithExtraConfigParams(cm runner.ConfigMap) provisionerOption {
	return func(p *provisionerParams) error { p.extraConfigParams = cm; return nil }
}
