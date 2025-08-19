// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package names provide types for eBPF resource names
package names

import (
	manager "github.com/DataDog/ebpf-manager"

	"github.com/cilium/ebpf"
)

// MapName represents a name of a map derived from an object representing
// an eBPF map. It should be used in places where we want guarantees that
// we are working with a valid map name.
type MapName struct {
	n string
}

// Name returns the map name as a string
func (m *MapName) Name() string {
	return m.n
}

// NewMapNameFromManagerMap creates a MapName object from a *manager.Map
func NewMapNameFromManagerMap(m *manager.Map) MapName {
	return MapName{n: m.Name}
}

// NewMapNameFromMapSpec creates a MapName object from an *ebpf.MapSpec
func NewMapNameFromMapSpec(m *ebpf.MapSpec) MapName {
	return MapName{n: m.Name}
}

// ProgramName represents a name of a map derived from an object representing
// an eBPF program. It should be used in places where we want guarantees that
// we are working with a valid program name.
type ProgramName struct {
	n string
}

// Name returns the program name as a string
func (p *ProgramName) Name() string {
	return p.n
}

// NewProgramNameFromProgramSpec creates a ProgramName from a *ebpf.ProgramSpec
func NewProgramNameFromProgramSpec(spec *ebpf.ProgramSpec) ProgramName {
	return ProgramName{n: spec.Name}
}

// ModuleName represents a module name. It should be used in places where
// we want guarantees that we are working with a string which was intended
// by the programmer to be treated as a module name
type ModuleName struct {
	n string
}

// Name returns the module name as a string
func (mn *ModuleName) Name() string {
	return mn.n
}

// NewModuleName creates a ModuleName from a string
func NewModuleName(mn string) ModuleName {
	return ModuleName{n: mn}
}

// ResourceName represents the name of any eBPF resources.
type ResourceName interface {
	Name() string
}
