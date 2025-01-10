// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package telemetry

import (
	"fmt"
	"hash"
	"slices"

	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/maps"
	"github.com/DataDog/datadog-agent/pkg/ebpf/names"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// MapErrTelemetryMapName is the map storing the map error telemetry
	MapErrTelemetryMapName string = "map_err_telemetry_map"
	// HelperErrTelemetryMapName is the map storing the helper error telemetry
	HelperErrTelemetryMapName string = "helper_err_telemetry_map"
)

// ErrorsTelemetryModifier is a modifier that sets up the manager to handle eBPF telemetry.
type ErrorsTelemetryModifier struct{}

// Ensure it implements the required interfaces
var _ ddebpf.ModifierBeforeInit = &ErrorsTelemetryModifier{}
var _ ddebpf.ModifierAfterInit = &ErrorsTelemetryModifier{}
var _ ddebpf.ModifierBeforeStop = &ErrorsTelemetryModifier{}

// String returns the name of the modifier.
func (t *ErrorsTelemetryModifier) String() string {
	return "ErrorsTelemetryModifier"
}

// getMapNames returns the names of the maps in the manager.
func getMapNames(m *manager.Manager) ([]names.MapName, error) {
	var mapNames []names.MapName

	// we use map specs instead of iterating over the user defined `manager.Maps`
	// because the user defined list may not contain shared maps passed to the manager
	// via `manager.Options.MapEditors`. On the other hand, MapSpecs will include all maps
	// referenced in the ELF file associated with the manager
	specs, err := m.GetMapSpecs()
	if err != nil {
		return nil, err
	}

	for _, spec := range specs {
		mapNames = append(mapNames, names.NewMapNameFromMapSpec(spec))
	}

	return mapNames, nil
}

// MapTelemetryKeyName builds the name of the parameterized constant used in bpf code
func MapTelemetryKeyName(mapName names.MapName) string {
	return mapName.Name() + "_telemetry_key"
}

// MapTelemetryErrorKey returns the key for map errors
func MapTelemetryErrorKey(h hash.Hash64, mapName names.MapName, module names.ModuleName) uint64 {
	return eBPFMapErrorKey(h, mapTelemetryKey(mapName, module))
}

// BeforeInit sets up the manager to handle eBPF telemetry.
// It will patch the instructions of all the manager probes and `undefinedProbes` provided.
// Constants are replaced for map error and helper error keys with their respective values.
func (t *ErrorsTelemetryModifier) BeforeInit(m *manager.Manager, module names.ModuleName, opts *manager.Options) error {
	activateBPFTelemetry, err := EBPFTelemetrySupported()
	if err != nil {
		return err
	}
	if opts.MapSpecEditors == nil {
		opts.MapSpecEditors = make(map[string]manager.MapSpecEditor)
	}

	// add telemetry maps to list of maps, if not present
	if !slices.ContainsFunc(m.Maps, func(x *manager.Map) bool { return x.Name == MapErrTelemetryMapName }) {
		m.Maps = append(m.Maps, &manager.Map{Name: MapErrTelemetryMapName})
	}
	if !slices.ContainsFunc(m.Maps, func(x *manager.Map) bool { return x.Name == HelperErrTelemetryMapName }) {
		m.Maps = append(m.Maps, &manager.Map{Name: HelperErrTelemetryMapName})
	}

	// set a small max entries value if telemetry is not supported. We have to load the maps because the eBPF code
	// references them even when we cannot track the telemetry.
	opts.MapSpecEditors[MapErrTelemetryMapName] = manager.MapSpecEditor{
		MaxEntries: uint32(1),
		EditorFlag: manager.EditMaxEntries,
	}
	opts.MapSpecEditors[HelperErrTelemetryMapName] = manager.MapSpecEditor{
		MaxEntries: uint32(1),
		EditorFlag: manager.EditMaxEntries,
	}

	if activateBPFTelemetry {
		ebpfMaps, err := m.GetMapSpecs()
		if err != nil {
			return fmt.Errorf("failed to get map specs from manager: %w", err)
		}

		ebpfPrograms, err := m.GetProgramSpecs()
		if err != nil {
			return fmt.Errorf("failed to get program specs from manager: %w", err)
		}

		opts.MapSpecEditors[MapErrTelemetryMapName] = manager.MapSpecEditor{
			MaxEntries: uint32(len(ebpfMaps)),
			EditorFlag: manager.EditMaxEntries,
		}
		log.Tracef("module %s maps %d", module.Name(), opts.MapSpecEditors[MapErrTelemetryMapName].MaxEntries)

		opts.MapSpecEditors[HelperErrTelemetryMapName] = manager.MapSpecEditor{
			MaxEntries: uint32(len(ebpfPrograms)),
			EditorFlag: manager.EditMaxEntries,
		}
		log.Tracef("module %s probes %d", module.Name(), opts.MapSpecEditors[HelperErrTelemetryMapName].MaxEntries)

		mapNames, err := getMapNames(m)
		if err != nil {
			return err
		}

		h := keyHash()
		for _, mapName := range mapNames {
			opts.ConstantEditors = append(opts.ConstantEditors, manager.ConstantEditor{
				Name:  MapTelemetryKeyName(mapName),
				Value: MapTelemetryErrorKey(h, mapName, module),
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

// getErrMaps returns the mapErrMap and helperErrMap from the manager.
func getErrMaps(m *manager.Manager) (mapErrMap *maps.GenericMap[uint64, mapErrTelemetry], helperErrMap *maps.GenericMap[uint64, helperErrTelemetry], err error) {
	mapErrMap, err = maps.GetMap[uint64, mapErrTelemetry](m, MapErrTelemetryMapName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get generic map %s: %w", MapErrTelemetryMapName, err)
	}

	helperErrMap, err = maps.GetMap[uint64, helperErrTelemetry](m, HelperErrTelemetryMapName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get generic map %s: %w", HelperErrTelemetryMapName, err)
	}

	return mapErrMap, helperErrMap, nil
}

// AfterInit pre-populates the telemetry maps with entries corresponding to the ebpf program of the manager.
func (t *ErrorsTelemetryModifier) AfterInit(m *manager.Manager, module names.ModuleName, _ *manager.Options) error {
	if errorsTelemetry == nil {
		return nil
	}

	genericMapErrMap, genericHelperErrMap, err := getErrMaps(m)
	if err != nil {
		return err
	}

	mapNames, err := getMapNames(m)
	if err != nil {
		return err
	}

	if err := errorsTelemetry.fill(mapNames, module, genericMapErrMap, genericHelperErrMap); err != nil {
		return err
	}

	return nil
}

// BeforeStop stops the perf collector from telemetry and removes the modules from the telemetry maps.
func (t *ErrorsTelemetryModifier) BeforeStop(m *manager.Manager, module names.ModuleName, _ manager.MapCleanupType) error {
	if errorsTelemetry == nil {
		return nil
	}

	genericMapErrMap, genericHelperErrMap, err := getErrMaps(m)
	if err != nil {
		return err
	}

	mapNames, err := getMapNames(m)
	if err != nil {
		return err
	}

	if err := errorsTelemetry.cleanup(mapNames, module, genericMapErrMap, genericHelperErrMap); err != nil {
		return err
	}

	return nil
}
