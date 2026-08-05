// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen

import (
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/redaction"
)

type config struct {
	objectLoader             object.Loader
	typeIndexFactory         goTypeIndexFactory
	skipReturnEvents         bool
	additionalTypes          []string
	skipRuntimeRecoveryProbe bool
	redaction                *redaction.Config
}

var defaultConfig = config{
	objectLoader:     object.NewInMemoryLoader(),
	typeIndexFactory: &inMemoryGoTypeIndexFactory{},
}

// Option configures ir generation.
type Option interface {
	apply(c *config)
}

// WithObjectLoader sets the object loader to use for loading object files.
func WithObjectLoader(loader object.Loader) Option {
	return optionFunc(func(c *config) { c.objectLoader = loader })
}

// WithOnDiskGoTypeIndexFactory make irgen store the go type indexes on disk.
func WithOnDiskGoTypeIndexFactory(diskCache *object.DiskCache) Option {
	return optionFunc(func(c *config) {
		c.typeIndexFactory = &onDiskGoTypeIndexFactory{diskCache: diskCache}
	})
}

// WithSkipReturnEvents skips the generation of return events.
func WithSkipReturnEvents(skip bool) Option {
	return optionFunc(func(c *config) { c.skipReturnEvents = skip })
}

// WithAdditionalTypes provides a list of Go type names (as reported by
// gotype) that should be resolved against the binary's Go runtime type
// table and added to the IR type registry. This is used to include types
// discovered at runtime through interface decoding.
func WithAdditionalTypes(typeNames []string) Option {
	return optionFunc(func(c *config) { c.additionalTypes = typeNames })
}

// WithSkipRuntimeRecoveryProbe suppresses the synthetic runtime.recovery
// probe that otherwise gets spliced into any program containing a
// function-targeted user probe. Used to honor a circuit-breaker trip on
// the recovery probe: the program then drops back to the pre-recovery
// behavior (probed frames unwound by panic+recover leak their
// in_progress_calls slot until the goid is reused) until the user
// probes change.
func WithSkipRuntimeRecoveryProbe(skip bool) Option {
	return optionFunc(func(c *config) { c.skipRuntimeRecoveryProbe = skip })
}

// WithRedaction sets the policy used to scrub sensitive captured values. When
// set, irgen attaches it to the generated program so the decoder can enforce
// it, rejects probe conditions that reference a redacted identifier, and marks
// capture expressions that reference one.
func WithRedaction(cfg *redaction.Config) Option {
	return optionFunc(func(c *config) { c.redaction = cfg })
}

type optionFunc func(c *config)

func (o optionFunc) apply(c *config) { o(c) }
