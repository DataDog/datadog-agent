// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package fakeintake

import "github.com/DataDog/datadog-agent/test/e2e-framework/common"

type Params struct {
	LoadBalancerEnabled bool
	ImageURL            string
	CPU                 int
	Memory              int
	DDDevForwarding     bool
	RetentionPeriod     string
}

type Option = func(*Params) error

// NewParams returns a new instance of Fakeintake Params
func NewParams(options ...Option) (*Params, error) {
	params := &Params{
		LoadBalancerEnabled: false,
		ImageURL:            "public.ecr.aws/datadog/fakeintake:latest",
		CPU:                 512,
		Memory:              1024,
		DDDevForwarding:     true,
		RetentionPeriod:     "15m",
	}
	return common.ApplyOption(params, options)
}

// WithLoadBalancer enable load balancer in front of the fakeintake
// Default is false
func WithLoadBalancer() Option {
	return func(p *Params) error {
		p.LoadBalancerEnabled = true
		return nil
	}
}

// WithImageURL sets the URL of the image to use to define the fakeintake
func WithImageURL(imageURL string) Option {
	return func(p *Params) error {
		p.ImageURL = imageURL
		return nil
	}
}

// WithCPU sets the number of CPU units to allocate to the fakeintake
// Default is 512 CPU units
func WithCPU(cpu int) Option {
	return func(p *Params) error {
		p.CPU = cpu
		return nil
	}
}

// WithMemory sets the amount (in MiB) of memory to allocate to the fakeintake
// Default is 1024 MiB
func WithMemory(memory int) Option {
	return func(p *Params) error {
		p.Memory = memory
		return nil
	}
}

// WithoutDDDevForwarding disables payload forwarding to dddev account.
// dddev forwarding is enabled by default
func WithoutDDDevForwarding() Option {
	return func(p *Params) error {
		p.DDDevForwarding = false
		return nil
	}
}

// WithRetentionPeriod set the retention period for the fakeintake
// Default is 15 minutes
// Possible values are: 1m, 10s, 1h
func WithRetentionPeriod(retentionPeriod string) Option {
	return func(p *Params) error {
		p.RetentionPeriod = retentionPeriod
		return nil
	}
}
