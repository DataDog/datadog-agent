// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package process holds process related files
package process

// ResolverOpts options of resolver
type ResolverOpts struct {
	ttyFallbackEnabled    bool
	envsResolutionEnabled bool
	envsWithValue         map[string]bool
}

// WithEnvsValue specifies envs with value
func (o *ResolverOpts) WithEnvsValue(envsWithValue []string) *ResolverOpts {
	for _, envVar := range envsWithValue {
		o.envsWithValue[envVar] = true
	}
	return o
}

// WithTTYFallbackEnabled enables the TTY fallback
func (o *ResolverOpts) WithTTYFallbackEnabled() *ResolverOpts {
	o.ttyFallbackEnabled = true
	return o
}

// WithEnvsResolutionEnabled enables the envs resolution
func (o *ResolverOpts) WithEnvsResolutionEnabled() *ResolverOpts {
	o.envsResolutionEnabled = true
	return o
}

// NewResolverOpts returns a new set of process resolver options
func NewResolverOpts() *ResolverOpts {
	return &ResolverOpts{
		envsWithValue: make(map[string]bool),
	}
}
