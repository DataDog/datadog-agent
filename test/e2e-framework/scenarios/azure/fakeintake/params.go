// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package fakeintake

import "github.com/DataDog/datadog-agent/test/e2e-framework/common"

type Params struct {
	DDDevForwarding bool
	ImageURL        string
	StoreStype      string
	RetentionPeriod string
}

type Option = func(*Params) error

// NewParams returns a new instance of Fakeintake Params
func NewParams(options ...Option) (*Params, error) {
	params := &Params{
		ImageURL:        "public.ecr.aws/datadog/fakeintake:latest",
		DDDevForwarding: true,
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

// WithDDDevForwarding sets the flag to enable DD Dev forwarding
func WithoutDDDevForwarding() Option {
	return func(p *Params) error {
		p.DDDevForwarding = false
		return nil
	}
}

// WithRetentionPeriod set the retention period for the fakeintake
func WithRetentionPeriod(retentionPeriod string) Option {
	return func(p *Params) error {
		p.RetentionPeriod = retentionPeriod
		return nil
	}
}

// WithStoreType set the store type for the fakeintake
func WithStoreType(storeType string) Option {
	return func(p *Params) error {
		p.StoreStype = storeType
		return nil
	}
}
