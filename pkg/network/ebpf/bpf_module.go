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

func getBPFModule(bpfDir string, debug bool, name string) (bytecode.AssetReader, error) {
	file := name + ".o"
	if debug {
		file = name + "-debug.o"
	}
	ebpfReader, err := bytecode.GetReader(bpfDir, file)
	if err != nil {
		return nil, fmt.Errorf("couldn't find asset: %s", err)
	}

	return ebpfReader, nil
}

// ReadBPFModule from the asset file
func ReadBPFModule(bpfDir string, debug bool) (bytecode.AssetReader, error) {
	return getBPFModule(bpfDir, debug, "tracer")
}

// ReadHTTPModule from the asset file
func ReadHTTPModule(bpfDir string, debug bool) (bytecode.AssetReader, error) {
	return getBPFModule(bpfDir, debug, "http")
}

// ReadDNSModule from the asset file
func ReadDNSModule(bpfDir string, debug bool) (bytecode.AssetReader, error) {
	return getBPFModule(bpfDir, debug, "dns")
}

// ReadClassifierModule from the asset file
func ReadClassifierModule(bpfDir string, debug bool) (bytecode.AssetReader, error) {
	return getBPFModule(bpfDir, debug, "classifier")
}

// ReadOffsetBPFModule from the asset file
func ReadOffsetBPFModule(bpfDir string, debug bool) (bytecode.AssetReader, error) {
	return getBPFModule(bpfDir, debug, "offset-guess")
}
