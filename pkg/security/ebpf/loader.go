// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package ebpf

import (
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/log"
)

// ProbeLoader defines an eBPF ProbeLoader
type ProbeLoader struct {
	config            *config.Config
	bytecodeReader    bytecode.AssetReader
	useSyscallWrapper bool
}

// NewProbeLoader returns a new Loader
func NewProbeLoader(config *config.Config, useSyscallWrapper bool) *ProbeLoader {
	return &ProbeLoader{
		config:            config,
		useSyscallWrapper: useSyscallWrapper,
	}
}

// Close the ProbeLoader
func (l *ProbeLoader) Close() error {
	if l.bytecodeReader != nil {
		return l.bytecodeReader.Close()
	}
	return nil
}

// Load eBPF programs
func (l *ProbeLoader) Load() (bytecode.AssetReader, bool, error) {
	var err error
	var runtimeCompiled bool
	if l.config.RuntimeCompilationEnabled {
		l.bytecodeReader, err = getRuntimeCompiledPrograms(l.config, l.useSyscallWrapper)
		if err != nil {
			log.Warnf("error compiling runtime-security probe, falling back to pre-compiled: %s", err)
		} else {
			runtimeCompiled = true
		}
	}

	// fallback to pre-compiled version
	if l.bytecodeReader == nil {
		asset := "runtime-security"
		if l.useSyscallWrapper {
			asset += "-syscall-wrapper"
		}

		l.bytecodeReader, err = bytecode.GetReader(l.config.BPFDir, asset+".o")
		if err != nil {
			return nil, false, err
		}
	}

	return l.bytecodeReader, runtimeCompiled, nil
}

// OffsetGuesserLoader defines an eBPF Loader
type OffsetGuesserLoader struct {
	config         *config.Config
	bytecodeReader bytecode.AssetReader
}

// NewOffsetGuesserLoader returns a new OffsetGuesserLoader
func NewOffsetGuesserLoader(config *config.Config) *OffsetGuesserLoader {
	return &OffsetGuesserLoader{
		config: config,
	}
}

// Close the OffsetGuesserLoader
func (l *OffsetGuesserLoader) Close() error {
	if l.bytecodeReader != nil {
		return l.bytecodeReader.Close()
	}
	return nil
}

// Load eBPF programs
func (l *OffsetGuesserLoader) Load() (bytecode.AssetReader, error) {
	return bytecode.GetReader(l.config.BPFDir, "runtime-security-offset-guesser.o")
}
