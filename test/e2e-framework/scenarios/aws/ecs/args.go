// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ecs

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
)

type Params struct {
	FargateCapacityProvider    bool
	LinuxNodeGroup             bool
	LinuxARMNodeGroup          bool
	LinuxBottleRocketNodeGroup bool
	WindowsNodeGroup           bool
	ManagedInstanceNodeGroup   bool
}

type Option = func(*Params) error

func NewParams(options ...Option) (*Params, error) {
	version := &Params{}
	return common.ApplyOption(version, options)
}

func WithFargateCapacityProvider() Option {
	return func(p *Params) error {
		p.FargateCapacityProvider = true
		return nil
	}
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

func WithLinuxBottleRocketNodeGroup() Option {
	return func(p *Params) error {
		p.LinuxBottleRocketNodeGroup = true
		return nil
	}
}

func WithWindowsNodeGroup() Option {
	return func(p *Params) error {
		p.WindowsNodeGroup = true
		return nil
	}
}

func WithManagedInstanceNodeGroup() Option {
	return func(p *Params) error {
		p.ManagedInstanceNodeGroup = true
		return nil
	}
}

func buildClusterOptionsFromConfigMap(e aws.Environment) []Option {
	clusterOptions := []Option{}
	// Add the cluster options from the config map
	if e.ECSFargateCapacityProvider() {
		clusterOptions = append(clusterOptions, WithFargateCapacityProvider())
	}
	if e.ECSWindowsNodeGroup() {
		clusterOptions = append(clusterOptions, WithWindowsNodeGroup())
	}
	if e.ECSLinuxECSOptimizedARMNodeGroup() {
		clusterOptions = append(clusterOptions, WithLinuxARMNodeGroup())
	}
	if e.ECSLinuxECSOptimizedNodeGroup() {
		clusterOptions = append(clusterOptions, WithLinuxNodeGroup())
	}
	if e.ECSLinuxBottlerocketNodeGroup() {
		clusterOptions = append(clusterOptions, WithLinuxBottleRocketNodeGroup())
	}
	return clusterOptions
}
