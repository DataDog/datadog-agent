// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package fakeintake

import "github.com/DataDog/datadog-agent/test/e2e-framework/common"

type Params struct {
	DDDevForwarding bool
	ImageURL        string
	Memory          int
	LoadBalancer    bool
	RetentionPeriod string
}

type Option = func(*Params) error

// NewParams returns a new instance of Fakeintake Params
func NewParams(options ...Option) (*Params, error) {
	params := &Params{
		ImageURL:        "gcr.io/datadoghq/fakeintake:latest",
		DDDevForwarding: true,
		Memory:          1024,
		LoadBalancer:    false,
		RetentionPeriod: "15m",
	}
	return common.ApplyOption(params, options)
}

// WithImageURL sets the URL of the image to use to define the fakeintake
func WithImageURL(imageURL string) Option {
	return func(p *Params) error {
		p.ImageURL = imageURL
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

// WithMemory sets the amount (in MiB) of memory to allocate to the fakeintake
// Default is 1024 MiB
func WithMemory(memory int) Option {
	return func(p *Params) error {
		p.Memory = memory
		return nil
	}
}

// WithLoadBalancer enable load balancer in front of the fakeintake
func WithLoadBalancer() Option {
	return func(p *Params) error {
		p.LoadBalancer = true
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
