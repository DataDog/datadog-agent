// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package eks

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
)

type Params struct {
	LinuxNodeGroup        bool
	LinuxARMNodeGroup     bool
	BottleRocketNodeGroup bool
	WindowsNodeGroup      bool
	GPUNodeGroup          bool
	GPUInstanceType       string
}

type Option = func(*Params) error

func NewParams(options ...Option) (*Params, error) {
	version := &Params{}
	return common.ApplyOption(version, options)
}

func WithLinuxNodeGroup() Option {
	return func(p *Params) error {
		p.LinuxNodeGroup = true
		return nil
	}
}

func WithLinuxARMNodeGroup() Option {
	return func(p *Params) error {
		p.LinuxARMNodeGroup = true
		return nil
	}
}

func WithBottlerocketNodeGroup() Option {
	return func(p *Params) error {
		p.BottleRocketNodeGroup = true
		return nil
	}
}

func WithWindowsNodeGroup() Option {
	return func(p *Params) error {
		p.WindowsNodeGroup = true
		return nil
	}
}

// WithGPUNodeGroup enables creation of a GPU-enabled node group.
// instanceType should be a GPU instance type (e.g., "g4dn.xlarge", "g4dn.12xlarge", "g5.xlarge").
// If instanceType is empty, it defaults to "g4dn.xlarge" (1x NVIDIA T4 GPU, cheapest option).
func WithGPUNodeGroup(instanceType string) Option {
	return func(p *Params) error {
		p.GPUNodeGroup = true
		if instanceType == "" {
			instanceType = "g4dn.xlarge" // Default: 1x T4 GPU, ~$0.526/hr on-demand
		}
		p.GPUInstanceType = instanceType
		return nil
	}
}

func buildClusterOptionsFromConfigMap(e aws.Environment) []Option {
	clusterOptions := []Option{}
	// Add the cluster options from the config map
	if e.EKSWindowsNodeGroup() {
		clusterOptions = append(clusterOptions, WithWindowsNodeGroup())
	}
	if e.EKSLinuxARMNodeGroup() {
		clusterOptions = append(clusterOptions, WithLinuxARMNodeGroup())
	}
	if e.EKSLinuxNodeGroup() {
		clusterOptions = append(clusterOptions, WithLinuxNodeGroup())
	}
	if e.EKSBottlerocketNodeGroup() {
		clusterOptions = append(clusterOptions, WithBottlerocketNodeGroup())
	}
	if e.EKSGPUNodeGroup() {
		clusterOptions = append(clusterOptions, WithGPUNodeGroup(e.EKSGPUInstanceType()))
	}
	return clusterOptions
}
