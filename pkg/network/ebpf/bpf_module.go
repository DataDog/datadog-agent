// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package ebpf

import (
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
)

// prebuiltModulesInUse is a global object which is responsible for keeping a list of all the prebuilt ebpf assets in use.
// This is used to report ebpf asset telemetry
var prebuiltModulesInUse = map[string]struct{}{}
var telemetryMu sync.Mutex

func readModule(bpfDir, moduleName string) (bytecode.AssetReader, error) {
	ebpfReader, err := bytecode.GetReader(bpfDir, moduleName+".o")
	if err != nil {
		return nil, fmt.Errorf("couldn't find asset: %s", err)
	}

	telemetryMu.Lock()
	defer telemetryMu.Unlock()
	prebuiltModulesInUse[moduleName] = struct{}{}
	return ebpfReader, nil
}

// ReadBPFModule from the asset file
func ReadBPFModule(bpfDir string) (bytecode.AssetReader, error) {
	return readModule(bpfDir, "tracer")
}

// ReadHTTPModule from the asset file
func ReadHTTPModule(bpfDir string) (bytecode.AssetReader, error) {
	return readModule(bpfDir, "usm")
}

// ReadSharedLibrariesModule from the asset file
func ReadSharedLibrariesModule(bpfDir string) (bytecode.AssetReader, error) {
	return readModule(bpfDir, "shared-libraries")
}

// ReadDNSModule from the asset file
func ReadDNSModule(bpfDir string) (bytecode.AssetReader, error) {
	return readModule(bpfDir, "dns")
}

// ReadOffsetBPFModule from the asset file
func ReadOffsetBPFModule(bpfDir string) (bytecode.AssetReader, error) {
	return readModule(bpfDir, "offset-guess")
}

// ReadFentryTracerModule from the asset file
func ReadFentryTracerModule(bpfDir string) (bytecode.AssetReader, error) {
	return readModule(bpfDir, "tracer-fentry")
}

// ReadConntrackBPFModule from the asset file
func ReadConntrackBPFModule(bpfDir string) (bytecode.AssetReader, error) {
	return readModule(bpfDir, "conntrack")
}

// GetModulesInUse returns a list of modules in use
func GetModulesInUse() []string {
	telemetryMu.Lock()
	defer telemetryMu.Unlock()

	return slices.Collect(maps.Keys(prebuiltModulesInUse))
}
