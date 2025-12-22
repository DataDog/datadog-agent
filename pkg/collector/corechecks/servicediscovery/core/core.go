// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package core

// PidSet represents a set of process IDs.
type PidSet map[int32]struct{}

// Has returns true if the set contains the given pid.
func (s PidSet) Has(pid int32) bool {
	_, present := s[pid]
	return present
}

// Add adds the given pid to the set.
func (s PidSet) Add(pid int32) {
	s[pid] = struct{}{}
}

// Remove removes the given pid from the set.
func (s PidSet) Remove(pid int32) {
	delete(s, pid)
}

// Discovery represents the core service discovery functionality.
type Discovery struct {
	Config *DiscoveryConfig
}

// Close cleans up resources used by the Discovery instance.
func (c *Discovery) Close() {
}
