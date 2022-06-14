// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package ebpf

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
)

// ReadBPFModule from the asset file
func ReadBPFModule(bpfDir string, debug bool) (bytecode.AssetReader, error) {
	file := "tracer.o"
	if debug {
		file = "tracer-debug.o"
	}

	return readAsset(bpfDir, file)
}

// ReadHTTPModule from the asset file
func ReadHTTPModule(bpfDir string, debug bool) (bytecode.AssetReader, error) {
	file := "http.o"
	if debug {
		file = "http-debug.o"
	}

	return readAsset(bpfDir, file)
}

// ReadDNSModule from the asset file
func ReadDNSModule(bpfDir string, debug bool) (bytecode.AssetReader, error) {
	file := "dns.o"
	if debug {
		file = "dns-debug.o"
	}

	return readAsset(bpfDir, file)
}

// ReadOffsetBPFModule from the asset file
func ReadOffsetBPFModule(bpfDir string, debug bool) (bytecode.AssetReader, error) {
	file := "offset-guess.o"
	if debug {
		file = "offset-guess-debug.o"
	}

	return readAsset(bpfDir, file)
}

func ReadProcessModule(bpfDir string, debug bool) (bytecode.AssetReader, error) {
	file := "process.o"
	if debug {
		file = "process-debug.o"
	}

	return readAsset(bpfDir, file)
}

func readAsset(bpfDir, name string) (bytecode.AssetReader, error) {
	ebpfReader, err := bytecode.GetReader(bpfDir, name)
	if err != nil {
		return nil, fmt.Errorf("couldn't find asset: %w", err)
	}

	return ebpfReader, nil
}
