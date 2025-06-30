// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package compiler

import (
	"io"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
)

// Compiler encapsulates the logic for compiling a program into a BPF program.
type Compiler struct {
	config
}

type config struct {
	ebpfConfig *ebpf.Config

	dyninstDebugLevel   uint8
	dyninstDebugEnabled bool
}

type option interface {
	apply(c *config)
}

type dyninstDebugLevelOption uint8

func (o dyninstDebugLevelOption) apply(c *config) {
	c.dyninstDebugLevel = uint8(o)
	c.dyninstDebugEnabled = true
}

// WithDyninstDebugLevel sets the debug level for the compiler.
func WithDyninstDebugLevel(level int) option {
	return dyninstDebugLevelOption(level)
}

type ebpfConfigOption ebpf.Config

// WithEbpfConfig sets the eBPF configuration for the compiler.
func WithEbpfConfig(cfg *ebpf.Config) option {
	return (*ebpfConfigOption)(cfg)
}

func (o *ebpfConfigOption) apply(c *config) {
	c.ebpfConfig = (*ebpf.Config)(o)
}

// NewCompiler creates a new compiler with the given eBPF configuration.
func NewCompiler(opts ...option) *Compiler {
	c := &Compiler{}
	for _, opt := range opts {
		opt.apply(&c.config)
	}
	if c.config.ebpfConfig == nil {
		c.config.ebpfConfig = ebpf.NewConfig()
	}
	return c
}

// Compile compiles the given program into a BPF program.
func (c *Compiler) Compile(
	program *ir.Program, extraCodeSink io.Writer,
) (CompiledBPF, error) {
	return compileBPFProgram(&c.config, program, extraCodeSink)
}
