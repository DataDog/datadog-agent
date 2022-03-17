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

// Loader defines an eBPF Loader
type Loader struct {
	config            *config.Config
	bytecodeReader    bytecode.AssetReader
	useSyscallWrapper bool
}

// NewLoader returns a new Loader
func NewLoader(config *config.Config, useSyscallWrapper bool) *Loader {
	return &Loader{
		config:            config,
		useSyscallWrapper: useSyscallWrapper,
	}
}

// Close the Loader
func (l *Loader) Close() error {
	return l.bytecodeReader.Close()
}

// Load eBPF programs
func (l *Loader) Load() (bytecode.AssetReader, error) {
	var err error
	if l.config.EnableRuntimeCompiler {
		l.bytecodeReader, err = getRuntimeCompiledPrograms(l.config, l.useSyscallWrapper)
		if err != nil {
			log.Warnf("error compiling runtime-security probe, falling back to pre-compiled: %s", err)
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
			return nil, err
		}
	}

	return l.bytecodeReader, nil
}
