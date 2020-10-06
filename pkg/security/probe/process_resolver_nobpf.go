// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux,!linux_bpf

package probe

// ProcessResolver resolved process context
type ProcessResolver struct {
	probe     *Probe
	resolvers *Resolvers
}

// Resolve returns a cache entry for the given pid
func (p *ProcessResolver) Resolve(pid uint32) *ProcessCacheEntry {
	return nil
}

// NewProcessResolver returns a new process resolver
func NewProcessResolver(probe *Probe, resolvers *Resolvers) (*ProcessResolver, error) {
	return &ProcessResolver{
		probe:     probe,
		resolvers: resolvers,
	}, nil
}
