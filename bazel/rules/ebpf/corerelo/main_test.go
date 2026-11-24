// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/cilium/ebpf/btf"
	"github.com/stretchr/testify/require"
)

// objects returns the CO-RE objects passed as test arguments.
func objects(t *testing.T) []string {
	objs := flag.Args()
	if len(objs) == 0 {
		t.Skip("no CO-RE objects given")
	}
	return objs
}

// reloSet returns the deduplicated relocations of the given ELF files.
func reloSet(t *testing.T, paths ...string) []string {
	set := map[string]bool{}
	for _, p := range paths {
		relos, _, err := readRelos(p)
		require.NoError(t, err, p)
		for _, r := range relos {
			tk, err := typeKey(r.typ)
			require.NoError(t, err)
			set[fmt.Sprintf("%d %s %s", r.kind, tk, r.access)] = true
		}
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func stub(t *testing.T, objs ...string) string {
	out := filepath.Join(t.TempDir(), "stub.o")
	require.NoError(t, run(objs, out))
	return out
}

func TestStubKeepsRelocations(t *testing.T) {
	for _, obj := range objects(t) {
		t.Run(filepath.Base(obj), func(t *testing.T) {
			want := reloSet(t, obj)
			s := stub(t, obj)
			if len(want) == 0 {
				info, err := os.Stat(s)
				require.NoError(t, err)
				require.Zero(t, info.Size(), "objects without relocations get an empty stub")
				return
			}
			require.Equal(t, want, reloSet(t, s))
		})
	}
}

func TestMergedStubIsUnion(t *testing.T) {
	objs := objects(t)
	require.Equal(t, reloSet(t, objs...), reloSet(t, stub(t, objs...)))
}

func TestStubIsDeterministic(t *testing.T) {
	objs := objects(t)
	reversed := slices.Clone(objs)
	slices.Reverse(reversed)
	a, err := os.ReadFile(stub(t, objs...))
	require.NoError(t, err)
	b, err := os.ReadFile(stub(t, reversed...))
	require.NoError(t, err)
	require.Equal(t, a, b)
}

func TestSameNamedTypesStayDistinct(t *testing.T) {
	u32 := &btf.Int{Name: "u32", Size: 4}
	newTask := func(members ...string) *btf.Struct {
		s := &btf.Struct{Name: "task", Size: uint32(4 * len(members))}
		for i, m := range members {
			s.Members = append(s.Members, btf.Member{Name: m, Type: u32, Offset: btf.Bits(32 * i)})
		}
		return s
	}
	const fieldByteOffset = 0
	relos := []relo{
		{kind: fieldByteOffset, typ: newTask("pid", "tgid"), access: "0:1"},
		{kind: fieldByteOffset, typ: newTask("pid", "tgid"), access: "0:1"},
		{kind: fieldByteOffset, typ: newTask("tgid", "pid"), access: "0:1"},
	}
	raw, err := buildStub(relos, binary.LittleEndian)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "stub.o")
	require.NoError(t, os.WriteFile(path, raw, 0o644))

	got, _, err := readRelos(path)
	require.NoError(t, err)
	require.Len(t, got, 2, "identical types collapse, different ones with the same name do not")
	require.NotEqual(t, got[0].typ, got[1].typ)
}

func TestRootsKeepTheirRelocationOrder(t *testing.T) {
	u32 := &btf.Int{Name: "u32", Size: 4}
	newTask := func() *btf.Struct {
		return &btf.Struct{Name: "task", Size: 8, Members: []btf.Member{
			{Name: "pid", Type: u32},
			{Name: "tgid", Type: u32, Offset: 32},
		}}
	}
	const fieldByteOffset = 0
	a, b := newTask(), newTask()
	relos := []relo{
		{kind: fieldByteOffset, typ: a, access: "0:1"},
		{kind: fieldByteOffset, typ: b, access: "0:0"},
		{kind: fieldByteOffset, typ: a, access: "0:1"},
		{kind: fieldByteOffset, typ: a, access: "0:0"},
		{kind: fieldByteOffset, typ: a, access: "0:1"},
	}
	raw, err := buildStub(relos, binary.LittleEndian)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "stub.o")
	require.NoError(t, os.WriteFile(path, raw, 0o644))

	got, _, err := readRelos(path)
	require.NoError(t, err)
	// a and b are identical but pruned separately by libbpf, so they keep separate
	// type IDs; repeats within a root are only dropped when consecutive.
	require.Len(t, got, 4)
	require.NotSame(t, got[0].typ, got[1].typ)
	require.Equal(t, []string{"0:0"}, accesses(got, got[0].typ))
	require.Equal(t, []string{"0:1", "0:0", "0:1"}, accesses(got, got[1].typ))
}

func TestOversizedRecordLength(t *testing.T) {
	le := binary.LittleEndian
	// The record size wraps the uint32 cursor back inside the section.
	sec := le.AppendUint32(nil, 0xfffffff8)
	sec = le.AppendUint32(sec, 0) // sec_name_off
	sec = le.AppendUint32(sec, 1) // num_info
	sec = le.AppendUint32(sec, 0) // truncated record
	raw := make([]byte, btfExtHeaderLen)
	le.PutUint16(raw, btfExtMagic)
	le.PutUint32(raw[4:], btfExtHeaderLen)
	le.PutUint32(raw[28:], uint32(len(sec)))
	_, err := coreRelos(append(raw, sec...), nil, nil, le)
	require.ErrorContains(t, err, "core_relo record truncated")
}

func accesses(relos []relo, typ btf.Type) []string {
	var out []string
	for _, r := range relos {
		if r.typ == typ {
			out = append(out, r.access)
		}
	}
	return out
}
