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

type ResourceType int

const (
	MapType ResourceType = iota
	ProgramType
)

type MapName struct {
	n string
}

func (m *MapName) String() string {
	return m.n
}

func (m *MapName) Type() ResourceType {
	return MapType
}

func NewMapNameFromManagerMap(m *manager.Map) MapName {
	return MapName{n: m.Name}
}

type ProgramName struct {
	n string
}

func (p *ProgramName) String() string {
	return p.n
}

func (p *ProgramName) Type() ResourceType {
	return ProgramType
}

func NewProgramNameFromProgramSpec(spec *ebpf.ProgramSpec) ProgramName {
	return ProgramName{n: spec.Name}
}

type ModuleName struct {
	n string
}

func (mn *ModuleName) String() string {
	return mn.n
}

func NewModuleName(mn string) ModuleName {
	return ModuleName{n: mn}
}

type ResourceName interface {
	String() string
	Type() ResourceType
}
