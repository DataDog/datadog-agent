// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package eks

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentwithoperatorparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/operatorparams"
	kubecomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes/cilium"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"
)

const (
	defaultEKSName = "eks"
)

// RunParams collects high-level scenario parameters for the EKS scenario.
type RunParams struct {
	Name               string
	vmOptions          []ec2.VMOption
	agentOptions       []kubernetesagentparams.Option
	fakeintakeOptions  []fakeintake.Option
	eksOptions         []Option
	extraConfigParams  runner.ConfigMap
	workloadAppFuncs   []kubecomp.WorkloadAppFunc
	operatorOptions    []operatorparams.Option
	operatorDDAOptions []agentwithoperatorparams.Option
	ciliumOptions      []cilium.Option

	eksLinuxNodeGroup        bool
	eksLinuxARMNodeGroup     bool
	eksBottlerocketNodeGroup bool
	eksWindowsNodeGroup      bool
	awsEnv                   *aws.Environment
	deployDogstatsd          bool
	deployTestWorkload       bool
	deployOperator           bool
	deployArgoRollout        bool
}

type RunOption = func(*RunParams) error

func GetRunParams(opts ...RunOption) *RunParams {
	params := &RunParams{
		Name:               defaultEKSName,
		vmOptions:          []ec2.VMOption{},
		agentOptions:       []kubernetesagentparams.Option{},
		fakeintakeOptions:  []fakeintake.Option{},
		eksOptions:         []Option{},
		extraConfigParams:  runner.ConfigMap{},
		workloadAppFuncs:   []kubecomp.WorkloadAppFunc{},
		operatorOptions:    []operatorparams.Option{},
		operatorDDAOptions: []agentwithoperatorparams.Option{},
		ciliumOptions:      []cilium.Option{},
	}
	err := optional.ApplyOptions(params, opts)
	if err != nil {
		panic(fmt.Errorf("unable to apply RunOption, err: %w", err))
	}
	return params
}

// ParamsFromEnvironment maps the Cloud environment ConfigMap to EKS scenario parameters.
func ParamsFromEnvironment(e aws.Environment) *RunParams {
	rp := &RunParams{
		eksOptions: buildClusterOptionsFromConfigMap(e),
	}

	// Agent options: set defaults not depending on resources
	if e.AgentDeploy() {
		rp.agentOptions = append(rp.agentOptions, kubernetesagentparams.WithNamespace("datadog"))
	}

	// Fakeintake options
	if e.AgentDeploy() && e.AgentUseFakeintake() {
		fiOpts := []fakeintake.Option{fakeintake.WithMemory(2048)}
		if e.InfraShouldDeployFakeintakeWithLB() {
			fiOpts = append(fiOpts, fakeintake.WithLoadBalancer())
		}
		if e.AgentUseDualShipping() {
			fiOpts = append(fiOpts, fakeintake.WithoutDDDevForwarding())
		}
		if retention := e.AgentFakeintakeRetentionPeriod(); retention != "" {
			fiOpts = append(fiOpts, fakeintake.WithRetentionPeriod(retention))
		}
		rp.fakeintakeOptions = fiOpts
	}

	rp.deployDogstatsd = e.DogstatsdDeploy()
	rp.deployTestWorkload = e.TestingWorkloadDeploy()

	return rp
}

// WithName sets the name of the provisioner
func WithName(name string) RunOption {
	return func(params *RunParams) error {
		params.Name = name
		return nil
	}
}

// WithEC2VMOptions adds options to the EC2 VM
func WithEC2VMOptions(opts ...ec2.VMOption) RunOption {
	return func(params *RunParams) error {
		params.vmOptions = opts
		return nil
	}
}

// WithAgentOptions adds options to the agent
func WithAgentOptions(opts ...kubernetesagentparams.Option) RunOption {
	return func(params *RunParams) error {
		params.agentOptions = opts
		return nil
	}
}

// WithFakeIntakeOptions adds options to the fake intake
func WithFakeIntakeOptions(opts ...fakeintake.Option) RunOption {
	return func(params *RunParams) error {
		params.fakeintakeOptions = opts
		return nil
	}
}

// WithEKSOptions adds options to the EKS cluster
func WithEKSOptions(opts ...Option) RunOption {
	return func(params *RunParams) error {
		params.eksOptions = opts
		return nil
	}
}

// WithDeployDogstatsd deploy standalone dogstatd
func WithDeployDogstatsd() RunOption {
	return func(params *RunParams) error {
		params.deployDogstatsd = true
		return nil
	}
}

// WithDeployTestWorkload deploy a test workload
func WithDeployTestWorkload() RunOption {
	return func(params *RunParams) error {
		params.deployTestWorkload = true
		return nil
	}
}

// WithoutFakeIntake removes the fake intake
func WithoutFakeIntake() RunOption {
	return func(params *RunParams) error {
		params.fakeintakeOptions = nil
		return nil
	}
}

// WithoutAgent removes the agent
func WithoutAgent() RunOption {
	return func(params *RunParams) error {
		params.agentOptions = nil
		return nil
	}
}

// WithExtraConfigParams adds extra config parameters to the environment
func WithExtraConfigParams(configMap runner.ConfigMap) RunOption {
	return func(params *RunParams) error {
		params.extraConfigParams = configMap
		return nil
	}
}

// WithWorkloadApp adds a workload app to the environment
func WithWorkloadApp(appFunc kubecomp.WorkloadAppFunc) RunOption {
	return func(params *RunParams) error {
		params.workloadAppFuncs = append(params.workloadAppFuncs, appFunc)
		return nil
	}
}

// WithAwsEnv asks the provisioner to use the given environment, it is created otherwise
func WithAwsEnv(env *aws.Environment) RunOption {
	return func(params *RunParams) error {
		params.awsEnv = env
		return nil
	}
}

// WithOperator Deploys the Datadog Operator
func WithOperator() RunOption {
	return func(params *RunParams) error {
		params.deployOperator = true
		return nil
	}
}

// WithOperatorOptions Configures the Datadog Operator
func WithOperatorOptions(opts ...operatorparams.Option) RunOption {
	return func(params *RunParams) error {
		params.operatorOptions = opts
		return nil
	}
}

// WithOperatorDDAOptions Configures the DatadogAgent custom resource
func WithOperatorDDAOptions(opts ...agentwithoperatorparams.Option) RunOption {
	return func(params *RunParams) error {
		params.operatorDDAOptions = opts
		return nil
	}
}

// WithoutDDA removes the DatadogAgent custom resource
func WithoutDDA() RunOption {
	return func(params *RunParams) error {
		params.operatorDDAOptions = nil
		params.agentOptions = nil
		return nil
	}
}

// WithCiliumOptions adds a cilium installation option
func WithCiliumOptions(opts ...cilium.Option) RunOption {
	return func(params *RunParams) error {
		params.ciliumOptions = opts
		return nil
	}
}

func WithDeployArgoRollout() RunOption {
	return func(params *RunParams) error {
		params.deployArgoRollout = true
		return nil
	}
}
