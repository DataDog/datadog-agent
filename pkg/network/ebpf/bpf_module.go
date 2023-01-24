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

func readModule(bpfDir, moduleName string, debug bool) (bytecode.AssetReader, error) {
	var file string
	if debug {
		file = fmt.Sprintf("%s-debug.o", moduleName)
	} else {
		file = fmt.Sprintf("%s.o", moduleName)
	}

	ebpfReader, err := bytecode.GetReader(bpfDir, file)
	if err != nil {
		return nil, fmt.Errorf("couldn't find asset: %s", err)
	}

	return ebpfReader, nil
}

func ReadBPFModule(bpfDir string, debug bool) (bytecode.AssetReader, error) {
	return readModule(bpfDir, "tracer", debug)
}

func ReadHTTPModule(bpfDir string, debug bool) (bytecode.AssetReader, error) {
	return readModule(bpfDir, "http", debug)
}

func ReadKafkaModule(bpfDir string, debug bool) (bytecode.AssetReader, error) {
	return readModule(bpfDir, "kafka", debug)
}

func ReadDNSModule(bpfDir string, debug bool) (bytecode.AssetReader, error) {
	return readModule(bpfDir, "dns", debug)
}

func ReadOffsetBPFModule(bpfDir string, debug bool) (bytecode.AssetReader, error) {
	return readModule(bpfDir, "offset-guess", debug)
}
