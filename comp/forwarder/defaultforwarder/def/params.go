// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package defaultforwarder

import "github.com/DataDog/datadog-agent/pkg/util/option"

// Params contains the parameters to create a forwarder.
type Params struct {
	UseNoopForwarder bool
	WithResolver     bool

	// Use optional to override Options.DisableAPIKeyChecking only if WithFeatures was called
	DisableAPIKeyCheckingOverride option.Option[bool]
	Features                      []Features
}

type OptionParams = func(*Params)

// NewParams initializes a new Params struct
func NewParams(options ...OptionParams) Params {
	p := Params{}
	for _, option := range options {
		option(&p)
	}
	return p
}

// WithResolvers enables the forwarder to use resolvers
func WithResolvers() OptionParams {
	return func(p *Params) {
		p.WithResolver = true
	}
}

// WithDisableAPIKeyChecking disables the API key checking
func WithDisableAPIKeyChecking() OptionParams {
	return func(p *Params) {
		p.DisableAPIKeyCheckingOverride.Set(true)
	}
}

// WithFeatures sets a features to the forwarder
func WithFeatures(features ...Features) OptionParams {
	return func(p *Params) {
		p.Features = features
	}
}

// WithNoopForwarder sets the forwarder to use the noop forwarder
func WithNoopForwarder() OptionParams {
	return func(p *Params) {
		p.UseNoopForwarder = true
	}
}
