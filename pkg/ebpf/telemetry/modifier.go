// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package telemetry

import (
	"fmt"
	"slices"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/ebpf/maps"
	"github.com/DataDog/datadog-agent/pkg/ebpf/names"
)

const (
	// MapErrTelemetryMap is the map storing the map error telemetry
	mapErrTelemetryMapName string = "map_err_telemetry_map"
	// HelperErrTelemetryMap is the map storing the helper error telemetry
	helperErrTelemetryMapName string = "helper_err_telemetry_map"
)

// ErrorsTelemetryModifier is a modifier that sets up the manager to handle eBPF telemetry.
type ErrorsTelemetryModifier struct{}

// String returns the name of the modifier.
func (t *ErrorsTelemetryModifier) String() string {
	return "ErrorsTelemetryModifier"
}

// BeforeInit sets up the manager to handle eBPF telemetry.
// It will patch the instructions of all the manager probes and `undefinedProbes` provided.
// Constants are replaced for map error and helper error keys with their respective values.
func (t *ErrorsTelemetryModifier) BeforeInit(m *manager.Manager, module names.ModuleName, opts *manager.Options) error {
	activateBPFTelemetry, err := ebpfTelemetrySupported()
	if err != nil {
		return err
	}

	if activateBPFTelemetry {
		// add telemetry maps to list of maps, if not present
		if !slices.ContainsFunc(m.Maps, func(x *manager.Map) bool { return x.Name == mapErrTelemetryMapName }) {
			m.Maps = append(m.Maps, &manager.Map{Name: mapErrTelemetryMapName})
		}
		if !slices.ContainsFunc(m.Maps, func(x *manager.Map) bool { return x.Name == helperErrTelemetryMapName }) {
			m.Maps = append(m.Maps, &manager.Map{Name: helperErrTelemetryMapName})
		}

		h := keyHash()
		for _, ebpfMap := range m.Maps {
			opts.ConstantEditors = append(opts.ConstantEditors, manager.ConstantEditor{
				Name:  ebpfMap.Name + "_telemetry_key",
				Value: eBPFMapErrorKey(h, mapTelemetryKey(names.NewMapNameFromManagerMap(ebpfMap), module)),
			})
		}
	}

	m.InstructionPatchers = append(m.InstructionPatchers, func(m *manager.Manager) error {
		specs, err := m.GetProgramSpecs()
		if err != nil {
			return err
		}
		return patchEBPFTelemetry(specs, activateBPFTelemetry, module, errorsTelemetry)
	})

	return nil
}

// AfterInit pre-populates the telemetry maps with entries corresponding to the ebpf program of the manager.
func (t *ErrorsTelemetryModifier) AfterInit(m *manager.Manager, module names.ModuleName, _ *manager.Options) error {
	if errorsTelemetry == nil {
		return nil
	}

	mapNames := make([]names.MapName, len(m.Maps))
	for _, mp := range m.Maps {
		mapNames = append(mapNames, names.NewMapNameFromManagerMap(mp))
	}

	mapErrMap, _, err := m.GetMapSpec(mapErrTelemetryMapName)
	if err != nil {
		return fmt.Errorf("failed to find map %s: %w", mapErrTelemetryMapName, err)
	}
	genericMapErrMap, err := maps.NewGenericMap[uint64, mapErrTelemetry](mapErrMap)
	if err != nil {
		return fmt.Errorf("failed to create generic map from map spec for %s: %w", mapErrTelemetryMapName, err)
	}

	helperErrMap, _, err := m.GetMapSpec(helperErrTelemetryMapName)
	if err != nil {
		return fmt.Errorf("failed to find map %s: %w", helperErrTelemetryMapName, err)
	}
	genericHelperErrMap, err := maps.NewGenericMap[uint64, helperErrTelemetry](helperErrMap)
	if err != nil {
		return fmt.Errorf("failed to create generic map from map spec for %s: %w", helperErrTelemetryMapName, err)
	}

	if err := errorsTelemetry.fill(mapNames, module, genericMapErrMap, genericHelperErrMap); err != nil {
		return err
	}

	return nil
}
