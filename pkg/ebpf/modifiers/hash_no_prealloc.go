// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && bpf

package modifiers

import (
	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/features"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/names"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// HashMapNoPreallocModifier modifies all hash map to not be pre-allocated
type HashMapNoPreallocModifier struct {
}

func (h *HashMapNoPreallocModifier) String() string {
	return "HashMapNoPreallocModifier"
}

func NoPreallocSupportedForMapType(spec *ebpf.MapSpec) bool {
	return spec.Type == ebpf.Hash || spec.Type == ebpf.PerCPUHash
}

func NoPreallocOverrideSupported() bool {
	kv, err := kernel.HostVersion()
	if err != nil {
		log.Warnf("Failed to get kernel version, disallowing NO_PREALLOC override: %v", err)
		return false
	}

	// set minimum supported kernel version for no prealloc override as 6.8, since this feature received a number
	// of bug fixes in previous versions which may effect stability
	return kv >= kernel.VersionCode(6, 8, 0) && features.HaveMapFlag(features.BPF_F_NO_PREALLOC) == nil
}

// BeforeInit modifies hash maps to not be pre-allocated (if available)
func (h *HashMapNoPreallocModifier) BeforeInit(mgr *manager.Manager, _ names.ModuleName, options *manager.Options) error {
	if !NoPreallocOverrideSupported() {
		return nil
	}

	specs, err := mgr.GetMapSpecs()
	if err != nil {
		return err
	}

	if options.MapSpecEditors == nil {
		options.MapSpecEditors = make(map[string]manager.MapSpecEditor)
	}

	for mapName, spec := range specs {
		if !NoPreallocSupportedForMapType(spec) {
			continue
		}

		editor := options.MapSpecEditors[mapName]
		if editor.EditorFlag&manager.EditType != 0 && editor.Type != ebpf.Hash && editor.Type != ebpf.PerCPUHash {
			// changing type to something other than supported types
			continue
		}

		// do not adjust 1 entry maps
		if editor.EditorFlag&manager.EditMaxEntries != 0 {
			log.Infof("map %s has edit flags for max entries: %d\n", mapName, editor.MaxEntries)
			if editor.MaxEntries <= 1 {
				continue
			}
		} else if spec.MaxEntries <= 1 {
			continue
		}

		editor.Flags |= features.BPF_F_NO_PREALLOC
		editor.EditorFlag |= manager.EditFlags
		options.MapSpecEditors[mapName] = editor
	}
	return nil
}

// ensure HashMapNoPreallocModifier implements the ModifierBeforeInit interface
var _ ddebpf.ModifierBeforeInit = &HashMapNoPreallocModifier{}
