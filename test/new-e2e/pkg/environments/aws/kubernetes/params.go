// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package awskubernetes contains the provisioner for the Kubernetes based environments
package awskubernetes

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"

	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/test-infra-definitions/components/kubernetes"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/eks"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
)

// ProvisionerParams contains all the parameters needed to create the environment
type ProvisionerParams struct {
	name              string
	vmOptions         []ec2.VMOption
	agentOptions      []kubernetesagentparams.Option
	fakeintakeOptions []fakeintake.Option
	eksOptions        []eks.Option
	extraConfigParams runner.ConfigMap
	workloadAppFuncs  []WorkloadAppFunc

	eksLinuxNodeGroup        bool
	eksLinuxARMNodeGroup     bool
	eksBottlerocketNodeGroup bool
	eksWindowsNodeGroup      bool
	awsEnv                   *aws.Environment
	deployDogstatsd          bool
	deployTestWorkload       bool
}

func newProvisionerParams() *ProvisionerParams {
	return &ProvisionerParams{
		name:              defaultVMName,
		vmOptions:         []ec2.VMOption{},
		agentOptions:      []kubernetesagentparams.Option{},
		fakeintakeOptions: []fakeintake.Option{},
		eksOptions:        []eks.Option{},
		extraConfigParams: runner.ConfigMap{},
		workloadAppFuncs:  []WorkloadAppFunc{},

		eksLinuxNodeGroup:        false,
		eksLinuxARMNodeGroup:     false,
		eksBottlerocketNodeGroup: false,
		eksWindowsNodeGroup:      false,
		deployDogstatsd:          false,
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

// ProvisionerOption is a function that modifies the ProvisionerParams
type ProvisionerOption func(*ProvisionerParams) error

// WithName sets the name of the provisioner
func WithName(name string) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.name = name
		return nil
	}
}

// WithEC2VMOptions adds options to the EC2 VM
func WithEC2VMOptions(opts ...ec2.VMOption) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.vmOptions = opts
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

// WithEKSOptions adds options to the EKS cluster
func WithEKSOptions(opts ...eks.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.eksOptions = opts
		return nil
	}
}

// WithDeployDogstatsd deploy standalone dogstatd
func WithDeployDogstatsd() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.deployDogstatsd = true
		return nil
	}
}

// WithDeployTestWorkload deploy a test workload
func WithDeployTestWorkload() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.deployTestWorkload = true
		return nil
	}
}

// WithoutFakeIntake removes the fake intake
func WithoutFakeIntake() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.fakeintakeOptions = nil
		return nil
	}
}

// WithoutAgent removes the agent
func WithoutAgent() ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.agentOptions = nil
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

// WorkloadAppFunc is a function that deploys a workload app to a kube provider
type WorkloadAppFunc func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error)

// WithWorkloadApp adds a workload app to the environment
func WithWorkloadApp(appFunc WorkloadAppFunc) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.workloadAppFuncs = append(params.workloadAppFuncs, appFunc)
		return nil
	}
}

// WithAwsEnv asks the provisioner to use the given environment, it is created otherwise
func WithAwsEnv(env *aws.Environment) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.awsEnv = env
		return nil
	}
}
