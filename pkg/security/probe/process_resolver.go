// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

type ProcessResolverEntry struct {
	Filename string
}

// ProcessResolver resolved process context
type ProcessResolver struct {
	probe     *Probe
	cache map[uint32]*ProcessResolverEntry
}

func (p *ProcessResolver) AddEntry(pid uint32, entry *ProcessResolverEntry) {
	p.cache[pid] = entry
}

func (p *ProcessResolver) DelEntry(pid uint32) {
	delete(p.cache, pid)
}

func (p *ProcessResolver) Resolve(pid uint32) *ProcessResolverEntry {
	return p.cache[pid]
}

// NewProcessResolver returns a new process resolver
func NewProcessResolver(probe *Probe) (*ProcessResolver, error) {
	return &ProcessResolver{
		probe: probe,
		cache: make(map[uint32]*ProcessResolverEntry),
	}, nil
}
