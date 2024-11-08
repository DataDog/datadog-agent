// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package to provide types for eBPF resource names
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

func (m *MapName) Name() string {
	return m.n
}

func NewMapNameFromManagerMap(m *manager.Map) MapName {
	return MapName{n: m.Name}
}

// ProgramName represents a name of a map derived from an object representing
// an eBPF program. It should be used in places where we want guarantees that
// we are working with a valid program name.
type ProgramName struct {
	n string
}

func (p *ProgramName) Name() string {
	return p.n
}

func NewProgramNameFromProgramSpec(spec *ebpf.ProgramSpec) ProgramName {
	return ProgramName{n: spec.Name}
}

type ModuleName struct {
	n string
}

func (mn *ModuleName) Name() string {
	return mn.n
}

func NewModuleName(mn string) ModuleName {
	return ModuleName{n: mn}
}

// ResourceName represents the name of any eBPF resources.
type ResourceName interface {
	Name() string
}
