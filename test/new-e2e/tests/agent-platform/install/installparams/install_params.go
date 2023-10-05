// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installparams implements function parameters for agent install functions
package installparams

import "os"

// Params defines the parameters for the Agent client.
// The Params configuration uses the [Functional options pattern].
//
// The available options are:
//   - [WithMajorVersion]

// Params struct containing install params
type Params struct {
	PipelineID   string
	MajorVersion string
}

// Option alias to a functional option changing a given Params instance
type Option func(*Params)

// NewParams creates a new instance of Install params
func NewParams(options ...Option) *Params {
	p := &Params{
		PipelineID:   os.Getenv("CI_PIPELINE_ID"),
		MajorVersion: "7",
	}
	return applyOption(p, options...)
}

func applyOption(instance *Params, options ...Option) *Params {
	for _, o := range options {
		o(instance)
	}
	return instance
}

// WithMajorVersion set the major version of the agent to install
func WithMajorVersion(majorVersion string) Option {
	return func(p *Params) {
		p.MajorVersion = majorVersion
	}
}
