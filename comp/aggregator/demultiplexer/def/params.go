// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package demultiplexer

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Params contains the parameters for the demultiplexer
type Params struct {
	ContinueOnMissingHostname bool

	// This is an optional field to override the default flush interval only if it is set
	FlushInterval option.Option[time.Duration]

	UseDogstatsdNoAggregationPipelineConfig bool
}

// Option is a function that sets a parameter in the Params struct
type Option func(*Params)

// NewDefaultParams returns the default parameters for the demultiplexer
func NewDefaultParams(options ...Option) Params {
	p := Params{}
	for _, o := range options {
		o(&p)
	}
	return p
}

// WithContinueOnMissingHostname sets the ContinueOnMissingHostname field to true
func WithContinueOnMissingHostname() Option {
	return func(p *Params) {
		p.ContinueOnMissingHostname = true
	}
}

// WithFlushInterval sets the FlushInterval field to the provided duration
func WithFlushInterval(duration time.Duration) Option {
	return func(p *Params) {
		p.FlushInterval = option.New(duration)
	}
}

// WithDogstatsdNoAggregationPipelineConfig uses the config dogstatsd_no_aggregation_pipeline
func WithDogstatsdNoAggregationPipelineConfig() Option {
	return func(p *Params) {
		p.UseDogstatsdNoAggregationPipelineConfig = true
	}
}
