// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/cilium/ebpf/btf"
	"github.com/stretchr/testify/require"
)

func newSpec(t *testing.T, types ...btf.Type) *btf.Spec {
	var b btf.Builder
	for _, typ := range types {
		_, err := b.Add(typ)
		require.NoError(t, err)
	}
	raw, err := b.Marshal(nil, &btf.MarshalOptions{Order: binary.LittleEndian})
	require.NoError(t, err)
	spec, err := btf.LoadSpecFromReader(bytes.NewReader(raw))
	require.NoError(t, err)
	return spec
}

func typeID(t *testing.T, spec *btf.Spec, name string) btf.TypeID {
	typ, err := spec.AnyTypeByName(name)
	require.NoError(t, err)
	id, err := spec.TypeID(typ)
	require.NoError(t, err)
	return id
}

func TestSameFixup(t *testing.T) {
	u32 := &btf.Int{Name: "u32", Size: 4}
	task := &btf.Struct{Name: "task", Size: 4, Members: []btf.Member{{Name: "pid", Type: u32}}}
	file := &btf.Struct{Name: "file", Size: 4, Members: []btf.Member{{Name: "f_mode", Type: u32}}}
	full := newSpec(t, file, task)
	minimized := newSpec(t, task)
	fullTask, fullFile := typeID(t, full, "task"), typeID(t, full, "file")
	minTask := typeID(t, minimized, "task")
	require.NotEqual(t, fullTask, minTask, "minimizing renumbers types")

	typeFixup := func(local int, target btf.TypeID) string {
		return fmt.Sprintf("target_type_id=%d->%d", local, target)
	}
	for _, tc := range []struct {
		want, got string
		same      bool
	}{
		{"byte_off=8->16", "byte_off=8->16", true},
		{"byte_off=8->16", "byte_off=8->24", false},
		{"field_exists=1->1", "field_exists=poison", false},
		{typeFixup(12, fullTask), typeFixup(12, minTask), true},
		{typeFixup(12, fullFile), typeFixup(12, minTask), false},
		{typeFixup(12, fullTask), typeFixup(13, minTask), false},
		{typeFixup(12, fullTask), typeFixup(12, 99), false},
		{typeFixup(12, fullTask), "target_type_id=poison", false},
		{"target_type_id=poison", "target_type_id=poison", true},
	} {
		require.Equal(t, tc.same, sameFixup(tc.want, tc.got, full, minimized), "%s vs %s", tc.want, tc.got)
	}
}
