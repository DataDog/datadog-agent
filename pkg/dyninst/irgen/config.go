// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen

import "github.com/DataDog/datadog-agent/pkg/dyninst/object"

type config struct {
	objectLoader     object.Loader
	typeIndexFactory goTypeIndexFactory
	skipReturnEvents bool
	additionalTypes  []string
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

type optionFunc func(c *config)

func (o optionFunc) apply(c *config) { o(c) }
