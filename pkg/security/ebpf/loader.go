// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ebpf

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-go/v5/statsd"
	manager "github.com/DataDog/ebpf-manager"
)

// ProbeLoader defines an eBPF ProbeLoader
type ProbeLoader struct {
	config            *config.Config
	bytecodeReader    bytecode.AssetReader
	useSyscallWrapper bool
	useRingBuffer     bool
	statsdClient      statsd.ClientInterface
}

// NewProbeLoader returns a new Loader
func NewProbeLoader(config *config.Config, useSyscallWrapper, useRingBuffer bool, statsdClient statsd.ClientInterface) *ProbeLoader {
	return &ProbeLoader{
		config:            config,
		useSyscallWrapper: useSyscallWrapper,
		useRingBuffer:     useRingBuffer,
		statsdClient:      statsdClient,
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
		l.bytecodeReader, err = getRuntimeCompiledPrograms(l.config, l.useSyscallWrapper, l.useRingBuffer, l.statsdClient)
		if err != nil {
			seclog.Warnf("error compiling runtime-security probe, falling back to pre-compiled: %s", err)
		} else {
			seclog.Debugf("successfully compiled runtime-security probe")
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

// IsSyscallWrapperRequired checks whether the wrapper is required
func IsSyscallWrapperRequired() (bool, error) {
	openSyscall, err := manager.GetSyscallFnName("open")
	if err != nil {
		return false, err
	}

	return !strings.HasPrefix(openSyscall, "SyS_") && !strings.HasPrefix(openSyscall, "sys_"), nil
}
