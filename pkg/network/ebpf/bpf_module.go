// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package ebpf

import (
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
)

// prebuiltModulesInUse is a global object which is responsible for keeping a list of all the prebuilt ebpf assets in use.
// This is used to report ebpf asset telemetry
var prebuiltModulesInUse = []string{}
var telemetrymu sync.Mutex

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

	telemetrymu.Lock()
	defer telemetrymu.Unlock()
	prebuiltModulesInUse = append(prebuiltModulesInUse, moduleName)

	return ebpfReader, nil
}

// ReadBPFModule from the asset file
func ReadBPFModule(bpfDir string, debug bool) (bytecode.AssetReader, error) {
	return readModule(bpfDir, "tracer", debug)
}

// ReadHTTPModule from the asset file
func ReadHTTPModule(bpfDir string, debug bool) (bytecode.AssetReader, error) {
	return readModule(bpfDir, "usm", debug)
}

// ReadDNSModule from the asset file
func ReadDNSModule(bpfDir string, debug bool) (bytecode.AssetReader, error) {
	return readModule(bpfDir, "dns", debug)
}

// ReadOffsetBPFModule from the asset file
func ReadOffsetBPFModule(bpfDir string, debug bool) (bytecode.AssetReader, error) {
	return readModule(bpfDir, "offset-guess", debug)
}

func ReadFentryTracerModule(bpfDir string, debug bool) (bytecode.AssetReader, error) {
	return readModule(bpfDir, "tracer-fentry", debug)
}

// ReadConntrackBPFModule from the asset file
func ReadConntrackBPFModule(bpfDir string, debug bool) (bytecode.AssetReader, error) {
	file := "conntrack.o"
	if debug {
		file = "conntrack-debug.o"
	}

	ebpfReader, err := bytecode.GetReader(bpfDir, file)
	if err != nil {
		return nil, fmt.Errorf("couldn't find asset: %s", err)
	}

	return ebpfReader, nil
}

func GetModulesInUse() []string {
	telemetrymu.Lock()
	defer telemetrymu.Unlock()

	result := make([]string, len(prebuiltModulesInUse))
	copy(result, prebuiltModulesInUse)
	return result
}
