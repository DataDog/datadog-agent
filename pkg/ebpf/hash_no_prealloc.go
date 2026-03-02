// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/features"

	"github.com/DataDog/datadog-agent/pkg/ebpf/names"
)

// HashMapNoPreallocModifier modifies all hash map to not be pre-allocated
type HashMapNoPreallocModifier struct {
}

func (h *HashMapNoPreallocModifier) String() string {
	return "HashMapNoPreallocModifier"
}

// BeforeInit modifies hash maps to not be pre-allocated (if available)
func (h *HashMapNoPreallocModifier) BeforeInit(mgr *manager.Manager, _ names.ModuleName, options *manager.Options) error {
	hasNoPrealloc := features.HaveMapFlag(features.BPF_F_NO_PREALLOC) == nil
	if !hasNoPrealloc {
		return nil
	}

	specs, err := mgr.GetMapSpecs()
	if err != nil {
		return err
	}

	for mapName, spec := range specs {
		if spec.Type != ebpf.Hash && spec.Type != ebpf.PerCPUHash {
			continue
		}
		editor := options.MapSpecEditors[mapName]
		// do not adjust 1 entry maps
		if editor.EditorFlag&manager.EditMaxEntries != 0 {
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

// ensure PrintkPatcherModifier implements the ModifierBeforeInit interface
var _ ModifierBeforeInit = &HashMapNoPreallocModifier{}
