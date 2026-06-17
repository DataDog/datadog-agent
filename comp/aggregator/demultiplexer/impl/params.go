// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package demultiplexerimpl

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// LookbackRetentionFactory creates the optional metric lookback retention
// backend for binaries that support lookback.
type LookbackRetentionFactory func(config.Component, string) aggregator.LookbackRetention

// LookbackTriggerFactory creates the optional DogStatsD metric lookback trigger
// for binaries that support trigger evaluation.
type LookbackTriggerFactory func(config.Component, log.Component, aggregator.LookbackDumper) aggregator.LookbackTrigger

// Params contains the parameters for the demultiplexer
type Params struct {
	continueOnMissingHostname bool

	// This is an optional field to override the default flush interval only if it is set
	flushInterval option.Option[time.Duration]

	lookbackRetentionFactory LookbackRetentionFactory
	lookbackTriggerFactory   LookbackTriggerFactory

	useDogstatsdNoAggregationPipelineConfig bool
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
		p.flushInterval = option.New(duration)
	}
}

// WithDogstatsdNoAggregationPipelineConfig uses the config dogstatsd_no_aggregation_pipeline
func WithDogstatsdNoAggregationPipelineConfig() Option {
	return func(p *Params) {
		p.useDogstatsdNoAggregationPipelineConfig = true
	}
}

// WithLookbackRetentionFactory wires the concrete metric lookback retention
// backend into demux instances created by this module. Binaries that do not
// support lookback should not set this option, which keeps the concrete
// lookback packages out of their link graph.
func WithLookbackRetentionFactory(factory LookbackRetentionFactory) Option {
	return func(p *Params) {
		p.lookbackRetentionFactory = factory
	}
}

// WithLookbackTriggerFactory wires the concrete metric lookback DogStatsD
// trigger into demux instances created by this module. Binaries that do not
// support lookback triggers should not set this option, which keeps the
// concrete trigger package out of their link graph.
func WithLookbackTriggerFactory(factory LookbackTriggerFactory) Option {
	return func(p *Params) {
		p.lookbackTriggerFactory = factory
	}
}
