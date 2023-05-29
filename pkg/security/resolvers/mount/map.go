// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mount

import "github.com/DataDog/datadog-agent/pkg/security/secl/model"

const LOWER_SIZE = 512

type MountMap struct {
	lower []*model.Mount
	upper map[uint32]*model.Mount

	fastPathCounter int
	slowPathCounter int
}

func NewMountMap() *MountMap {
	return &MountMap{
		lower: make([]*model.Mount, LOWER_SIZE),
		upper: make(map[uint32]*model.Mount),
	}
}

func (mm *MountMap) Get(id uint32) *model.Mount {
	if id < LOWER_SIZE {
		mm.fastPathCounter++
		return mm.lower[id]
	}
	mm.slowPathCounter++
	return mm.upper[id]
}

func (mm *MountMap) Delete(id uint32) {
	if id < LOWER_SIZE {
		mm.fastPathCounter++
		mm.lower[id] = nil
	}
	mm.slowPathCounter++
	delete(mm.upper, id)
}

func (mm *MountMap) Insert(id uint32, mount *model.Mount) {
	if id < LOWER_SIZE {
		mm.fastPathCounter++
		mm.lower[id] = mount
	}
	mm.slowPathCounter++
	mm.upper[id] = mount
}

func (mm *MountMap) Contains(id uint32) bool {
	if id < LOWER_SIZE {
		mm.fastPathCounter++
		return mm.lower[id] != nil
	}
	mm.slowPathCounter++
	return mm.upper[id] != nil
}

func (mm *MountMap) OverLen() int {
	return LOWER_SIZE + len(mm.upper)
}

func (mm *MountMap) ForEach(f func(uint32, *model.Mount) bool) {
	for id, mount := range mm.lower {
		if mount != nil {
			cont := f(uint32(id), mount)
			if !cont {
				return
			}
		}
	}

	for id, mount := range mm.upper {
		cont := f(id, mount)
		if !cont {
			return
		}
	}
}
