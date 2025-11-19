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
	UseAL2023Nodes        bool
}

type Option = func(*Params) error

func NewParams(options ...Option) (*Params, error) {
	version := &Params{
		UseAL2023Nodes: true,
	}
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

func WithUseAL2023Nodes() Option {
	return func(p *Params) error {
		p.UseAL2023Nodes = true
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
	return clusterOptions
}
