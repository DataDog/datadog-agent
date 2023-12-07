// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"fmt"
	"sync"

	"golang.org/x/exp/maps"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
)

// prebuiltModulesInUse is a global object which is responsible for keeping a list of all the prebuilt ebpf assets in use.
// This is used to report ebpf asset telemetry
var prebuiltModulesInUse = map[string]struct{}{}
var telemetrymu sync.Mutex

//nolint:revive // TODO(NET) Fix revive linter
func ModuleFileName(moduleName string, debug bool) string {
	if debug {
		return fmt.Sprintf("%s-debug.o", moduleName)
	}

	return fmt.Sprintf("%s.o", moduleName)
}

func readModule(bpfDir, moduleName string, debug bool) (bytecode.AssetReader, error) {
	ebpfReader, err := bytecode.GetReader(bpfDir, ModuleFileName(moduleName, debug))
	if err != nil {
		return nil, fmt.Errorf("couldn't find asset: %s", err)
	}

	telemetrymu.Lock()
	defer telemetrymu.Unlock()
	prebuiltModulesInUse[moduleName] = struct{}{}
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

// ReadSharedLibrariesModule from the asset file
func ReadSharedLibrariesModule(bpfDir string, debug bool) (bytecode.AssetReader, error) {
	return readModule(bpfDir, "shared-libraries", debug)
}

// ReadDNSModule from the asset file
func ReadDNSModule(bpfDir string, debug bool) (bytecode.AssetReader, error) {
	return readModule(bpfDir, "dns", debug)
}

// ReadOffsetBPFModule from the asset file
func ReadOffsetBPFModule(bpfDir string, debug bool) (bytecode.AssetReader, error) {
	return readModule(bpfDir, "offset-guess", debug)
}

//nolint:revive // TODO(NET) Fix revive linter
func ReadFentryTracerModule(bpfDir string, debug bool) (bytecode.AssetReader, error) {
	return readModule(bpfDir, "tracer-fentry", debug)
}

// ReadConntrackBPFModule from the asset file
func ReadConntrackBPFModule(bpfDir string, debug bool) (bytecode.AssetReader, error) {
	return readModule(bpfDir, "conntrack", debug)
}

//nolint:revive // TODO(NET) Fix revive linter
func GetModulesInUse() []string {
	telemetrymu.Lock()
	defer telemetrymu.Unlock()

	return maps.Keys(prebuiltModulesInUse)
}
