// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kindvm

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"
)

type provisionerParams struct {
	awsEnv            *aws.Environment
	runOptions        []kindvm.RunOption
	extraConfigParams runner.ConfigMap
}

type provisionerOption func(*provisionerParams) error

func getProvisionerParams(opts ...provisionerOption) *provisionerParams {
	p := &provisionerParams{awsEnv: nil, runOptions: []kindvm.RunOption{}, extraConfigParams: runner.ConfigMap{}}
	_ = optional.ApplyOptions(p, opts)
	return p
}

func WithAwsEnv(env *aws.Environment) provisionerOption {
	return func(p *provisionerParams) error { p.awsEnv = env; return nil }
}
func WithRunOptions(opts ...kindvm.RunOption) provisionerOption {
	return func(p *provisionerParams) error { p.runOptions = append(p.runOptions, opts...); return nil }
}
func WithExtraConfigParams(cm runner.ConfigMap) provisionerOption {
	return func(p *provisionerParams) error { p.extraConfigParams = cm; return nil }
}
