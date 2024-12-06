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
	Arch         string
	Flavor       string
	Upgrade      bool
	APIKey       string
}

// Option alias to a functional option changing a given Params instance
type Option func(*Params)

const defaultMajorVersion = "7"

// NewParams creates a new instance of Install params
func NewParams(options ...Option) *Params {
	majorVersion, found := os.LookupEnv("E2E_MAJOR_VERSION")

	if !found {
		majorVersion = defaultMajorVersion
	}

	p := &Params{
		PipelineID:   os.Getenv("E2E_PIPELINE_ID"),
		MajorVersion: majorVersion,
		Arch:         "x86_64",
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

// WithArch specify the arch to use when installing the agent, needed to determine repo for deb repository
func WithArch(arch string) Option {
	return func(p *Params) {
		p.Arch = arch
	}
}

// WithFlavor specify the flavor to use when installing the agent
func WithFlavor(flavor string) Option {
	return func(p *Params) {
		p.Flavor = flavor
	}
}

// WithUpgrade specify if the upgrade environment variable is used when installing the agent
func WithUpgrade(upgrade bool) Option {
	return func(p *Params) {
		p.Upgrade = upgrade
	}
}

// WithAPIKey specify a custom api key to use when installing the agent
func WithAPIKey(apiKey string) Option {
	return func(p *Params) {
		p.APIKey = apiKey
	}
}

// WithPipelineID specify a custom pipeline ID to use when installing the agent
func WithPipelineID(id string) Option {
	return func(p *Params) {
		p.PipelineID = id
	}
}
