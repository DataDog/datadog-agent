// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ecs

import (
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/ecsagentparams"
	fakeintakeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	ecsComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/ecs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	scenfi "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"
)

const defaultECSName = "ecs"

// RunParams collects parameters for the ECS scenario
type RunParams struct {
	Name                    string
	agentOptions            []ecsagentparams.Option
	fakeintakeOptions       []scenfi.Option
	ecsOptions              []Option
	testingWorkload         bool
	workloadAppFuncs        []WorkloadAppFunc
	fargateWorkloadAppFuncs []FargateWorkloadAppFunc
}

// WorkloadAppFunc deploys a workload app to an ECS cluster
type WorkloadAppFunc func(e aws.Environment, clusterArn pulumi.StringInput) (*ecsComp.Workload, error)

// FargateWorkloadAppFunc deploys a Fargate workload app to an ECS cluster
type FargateWorkloadAppFunc func(e aws.Environment, clusterArn pulumi.StringInput, apiKeySSMParamName pulumi.StringInput, fakeIntake *fakeintakeComp.Fakeintake) (*ecsComp.Workload, error)

type RunOption = func(*RunParams) error

func GetRunParams(opts ...RunOption) *RunParams {
	p := &RunParams{
		Name:                    defaultECSName,
		agentOptions:            []ecsagentparams.Option{},
		fakeintakeOptions:       []scenfi.Option{scenfi.WithLoadBalancer()},
		ecsOptions:              []Option{},
		workloadAppFuncs:        []WorkloadAppFunc{},
		fargateWorkloadAppFuncs: []FargateWorkloadAppFunc{},
	}
	if err := optional.ApplyOptions(p, opts); err != nil {
		panic(fmt.Errorf("unable to apply RunOption, err: %w", err))
	}
	return p
}

// ParamsFromEnvironment maps environment to ECS scenario params
func ParamsFromEnvironment(e aws.Environment) *RunParams {
	p := &RunParams{Name: defaultECSName}
	// cluster options
	p.ecsOptions = buildClusterOptionsFromConfigMap(e)
	// fakeintake default
	if e.AgentDeploy() && e.AgentUseFakeintake() {
		fi := []scenfi.Option{}
		if e.InfraShouldDeployFakeintakeWithLB() {
			fi = append(fi, scenfi.WithLoadBalancer())
		}
		if retention := e.AgentFakeintakeRetentionPeriod(); retention != "" {
			fi = append(fi, scenfi.WithRetentionPeriod(retention))
		}
		p.fakeintakeOptions = fi
	}
	p.testingWorkload = e.TestingWorkloadDeploy()
	return p
}

// WithAgentOptions sets agent options
func WithAgentOptions(opts ...ecsagentparams.Option) RunOption {
	return func(p *RunParams) error { p.agentOptions = append(p.agentOptions, opts...); return nil }
}

// WithFakeIntakeOptions sets fakeintake options
func WithFakeIntakeOptions(opts ...scenfi.Option) RunOption {
	return func(p *RunParams) error { p.fakeintakeOptions = append(p.fakeintakeOptions, opts...); return nil }
}

// WithECSOptions sets ECS cluster options
func WithECSOptions(opts ...Option) RunOption {
	return func(p *RunParams) error { p.ecsOptions = append(p.ecsOptions, opts...); return nil }
}

// WithTestingWorkload enables testing workloads
func WithTestingWorkload() RunOption {
	return func(p *RunParams) error { p.testingWorkload = true; return nil }
}

// WithoutFakeIntake disables fakeintake
func WithoutFakeIntake() RunOption {
	return func(p *RunParams) error { p.fakeintakeOptions = nil; return nil }
}

// WithWorkloadApp adds an EC2 workload app
func WithWorkloadApp(appFunc WorkloadAppFunc) RunOption {
	return func(p *RunParams) error { p.workloadAppFuncs = append(p.workloadAppFuncs, appFunc); return nil }
}

// WithFargateWorkloadApp adds a Fargate workload app
func WithFargateWorkloadApp(appFunc FargateWorkloadAppFunc) RunOption {
	return func(p *RunParams) error {
		p.fargateWorkloadAppFuncs = append(p.fargateWorkloadAppFuncs, appFunc)
		return nil
	}
}
