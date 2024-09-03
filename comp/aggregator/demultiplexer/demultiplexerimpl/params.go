// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package demultiplexerimpl

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// Params contains the parameters for the demultiplexer
type Params struct {
	continueOnMissingHostname bool

	// This is an optional field to override the default flush interval only if it is set
	flushInterval optional.Option[time.Duration]
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

// WithContinueOnMissingHostname sets the continueOnMissingHostname field to true
func WithContinueOnMissingHostname() Option {
	return func(p *Params) {
		p.continueOnMissingHostname = true
	}
}

// WithFlushInterval sets the flushInterval field to the provided duration
func WithFlushInterval(duration time.Duration) Option {
	return func(p *Params) {
		p.flushInterval = optional.NewOption(duration)
	}
}
