// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package defaultforwarder

import "github.com/DataDog/datadog-agent/pkg/util/option"

// Params contains the parameters to create a forwarder.
type Params struct {
	allowOPW bool

	// Use optional to override Options.DisableAPIKeyChecking only if WithFeatures was called
	disableAPIKeyCheckingOverride option.Option[bool]
	features                      []Features
}

type optionParams = func(*Params)

// NewParams initializes a new Params struct
func NewParams(options ...optionParams) Params {
	p := Params{
		allowOPW: true,
	}
	for _, option := range options {
		option(&p)
	}
	return p
}

// WithAllowOPW sets whether observability_pipelines and vector config keys need to be honored.
func WithAllowOPW(allow bool) optionParams {
	return func(p *Params) {
		p.allowOPW = allow
	}
}

// WithDisableAPIKeyChecking disables the API key checking
func WithDisableAPIKeyChecking() optionParams {
	return func(p *Params) {
		p.disableAPIKeyCheckingOverride.Set(true)
	}
}

// WithFeatures sets a features to the forwarder
func WithFeatures(features ...Features) optionParams {
	return func(p *Params) {
		p.features = features
	}
}
