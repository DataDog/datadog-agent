// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package gotype

import (
	"encoding/binary"
	"fmt"
	"strings"
	"sync/atomic"
)

// getOrSetMapType looks up the map type for the table. If it's already set,
// it returns the current map type. If it's not set, it resolves the map type
// by using the fact that knownMapType is a map type and that for both swiss
// maps and hmaps, the same offset in type metadata points to the buckets/groups
// type and those have specific name prefixes.
func (m *Table) getOrSetMapType(knownMapTypeID TypeID) (mapType, error) {
	if mt := m.mapType(); mt != mapTypeUnknown {
		return mt, nil
	}
	mt, err := resolveMapType(m, knownMapTypeID)
	if err != nil {
		return mapTypeUnknown, err
	}
	if err := m.setMapType(mt); err != nil {
		return mapTypeUnknown, err
	}
	return mt, nil
}

type mapTypeContainer struct {
	v atomic.Uint32
}

func (m *mapTypeContainer) mapType() mapType { return mapType(m.v.Load()) }

func (m *mapTypeContainer) setMapType(t mapType) error {
	if !m.v.CompareAndSwap(uint32(mapTypeUnknown), uint32(t)) && m.mapType() != t {
		return fmt.Errorf("map type already set to %v, cant set to %v", m.mapType(), t)
	}
	return nil
}

// mapType is the type of map implementation used in a given binary.
type mapType uint32

const (
	mapTypeUnknown mapType = iota
	mapTypeSwiss
	mapTypeHmap
)

func (m mapType) String() string {
	switch m {
	case mapTypeSwiss:
		return "swiss"
	case mapTypeHmap:
		return "hmap"
	}
	return "unknown"
}

// resolveMapType heuristically detects the map implementation by reading the
// name of the buckets/groups type from the map type metadata.
func resolveMapType(m *Table, knownMapTypeID TypeID) (mapType, error) {
	tl := &m.abiLayout._type
	ml := &m.abiLayout.hMap
	bucketsOrGroupsTypeOffset := int(knownMapTypeID) + tl.size + ml.bucketPtrOff
	if bucketsOrGroupsTypeOffset < 0 || bucketsOrGroupsTypeOffset+8 > len(m.data) {
		return mapTypeUnknown, fmt.Errorf(
			"bucketsOrGroupsTypeOffset %d out of bounds", bucketsOrGroupsTypeOffset,
		)
	}
	bucketsOrGroupsTypePtr := binary.LittleEndian.Uint64(m.data[bucketsOrGroupsTypeOffset:])
	tid := TypeID(bucketsOrGroupsTypePtr - m.dataAddress)
	if !validateBaseTypeData(m, int(tid)) {
		return mapTypeUnknown, fmt.Errorf(
			"bucketsOrGroupsTypePtr %d out of bounds", bucketsOrGroupsTypePtr,
		)
	}
	t := Type{tbl: m, id: tid}
	name := t.Name()
	decodedName := name.UnsafeName()
	if strings.HasPrefix(decodedName, "map.bucket") {
		return mapTypeHmap, nil
	}
	if strings.HasPrefix(decodedName, "map.group") {
		return mapTypeSwiss, nil
	}
	return mapTypeUnknown, fmt.Errorf("unknown map type: %q", name.Name())
}
