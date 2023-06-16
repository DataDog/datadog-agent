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

func (mm *MountMap) GetChecked(id uint32) (*model.Mount, bool) {
	if id < LOWER_SIZE {
		mm.fastPathCounter++
		ptr := mm.lower[id]
		return ptr, ptr != nil
	}
	mm.slowPathCounter++
	ptr, exists := mm.upper[id]
	return ptr, exists
}

func (mm *MountMap) Get(id uint32) *model.Mount {
	ptr, _ := mm.GetChecked(id)
	return ptr
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
	if mount == nil {
		delete(mm.upper, id)
	} else {
		mm.upper[id] = mount
	}
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

func (mm *MountMap) RealLen() int {
	count := 0
	for _, mount := range mm.lower {
		if mount != nil {
			count += 1
		}
	}

	count += len(mm.upper)
	return count
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
