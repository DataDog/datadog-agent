// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package defaultforwarder

import (
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// Params contains the parameters to create a forwarder.
type Params struct {
	useNoopForwarder bool
	withResolver     bool

	// Use optional to override Options.DisableAPIKeyChecking only if WithFeatures was called
	disableAPIKeyCheckingOverride optional.Option[bool]
	features                      []Features
}

type option = func(*Params)

// NewParams initializes a new Params struct
func NewParams(options ...option) Params {
	p := Params{}
	for _, option := range options {
		option(&p)
	}
	return p
}

// WithResolvers enables the forwarder to use resolvers
func WithResolvers() option {
	return func(p *Params) {
		p.withResolver = true
	}
}

// WithDisableAPIKeyChecking disables the API key checking
func WithDisableAPIKeyChecking() option {
	return func(p *Params) {
		p.disableAPIKeyCheckingOverride.Set(true)
	}
}

// WithFeatures sets a features to the forwarder
func WithFeatures(features ...Features) option {
	return func(p *Params) {
		p.features = features
	}
}

// WithNoopForwarder sets the forwarder to use the noop forwarder
func WithNoopForwarder() option {
	return func(p *Params) {
		p.useNoopForwarder = true
	}
}
