// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	"github.com/DataDog/datadog-go/v5/statsd"
)

// Opts defines some probe options
type Opts struct {
	// DontDiscardRuntime do not discard the runtime. Mostly used by functional tests
	DontDiscardRuntime bool
	// StatsdClient to be used for probe stats
	StatsdClient statsd.ClientInterface
	// PathResolutionEnabled defines if the path resolution is enabled
	PathResolutionEnabled bool
	// TagsResolver will override the default one. Mainly here for tests.
	TagsResolver tags.Resolver
	// SyscallsMapMonitorEnabled enable syscalls map monitor
	SyscallsMapMonitorEnabled bool
	// TTYFallbackEnabled enable the tty procfs fallback
	TTYFallbackEnabled bool
}

func (o *Opts) normalize() {
	if o.StatsdClient == nil {
		o.StatsdClient = &statsd.NoOpClient{}
	}
}
