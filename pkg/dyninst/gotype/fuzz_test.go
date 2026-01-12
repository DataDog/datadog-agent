// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package gotype

import (
	"encoding/binary"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

// FuzzParseTypeAndCallMethods constructs a table from random bytes, parses a
// type from it, and, if successful, calls methods on the parsed value and on
// wrapper values returned by its accessors.
func FuzzParseTypeAndCallMethods(f *testing.F) {
	// Seed with a minimally sized buffer.

	if !testing.Short() {
		cfgs := testprogs.MustGetCommonConfigs(f)
		bin := testprogs.MustGetBinary(f, "simple", cfgs[0])

		mef, err := object.OpenMMappingElfFile(bin)
		require.NoError(f, err)
		defer func() { err = errors.Join(err, mef.Close()); require.NoError(f, err) }()

		lt, err := NewTable(mef)
		require.NoError(f, err)
		defer func() { err = errors.Join(err, lt.Close()); require.NoError(f, err) }()

		tlSection := mef.Section(".typelink")
		require.NotNil(f, tlSection)
		tlMap, err := mef.SectionData(tlSection)
		require.NoError(f, err)
		defer func() { err = errors.Join(err, tlMap.Close()); require.NoError(f, err) }()

		tl := ParseTypeLinks(tlMap.Data())
		for id := range tl.TypeIDs() {
			f.Add(lt.data, lt.dataAddress, id)
		}
	}

	f.Fuzz(func(t *testing.T, data []byte, dataAddress uint64, offset uint32) {
		// Need at least the type header size to parse.
		if len(data) < hardCodedLayout._type.size || offset > uint32(len(data)) {
			t.Skip()
		}

		// Choose an offset within bounds.
		var id TypeID
		limit := len(data) - hardCodedLayout._type.size
		if len(data) >= hardCodedLayout._type.size+4 {
			raw := binary.LittleEndian.Uint32(
				data[len(data)-4:],
			)
			id = TypeID(int(raw) % (limit + 1))
		} else {
			id = 0
		}

		// Use dataAddress 0 to avoid pointer-below-base panics in accessors.
		tb := &Table{
			abiLayout:   hardCodedLayout,
			data:        data,
			dataAddress: dataAddress,
			closer:      nopCloser{},
		}

		ty, err := tb.ParseGoType(id)
		if err != nil {
			return
		}

		// Basic methods on Type.
		_ = ty.ID()
		_ = ty.Size()
		_ = ty.Kind()

		// Name and related methods.
		tname := ty.Name()
		_ = tname.Name()
		_ = tname.UnsafeName()
		_ = tname.IsExported()
		_ = tname.IsEmbedded()
		_ = tname.HasTag()
		_ = tname.Tag()

		// Pkg path and name methods.
		_ = ty.PkgPathOff()
		pkg := ty.PkgPath()
		_ = pkg.Name()
		_ = pkg.UnsafeName()
		_ = pkg.IsExported()
		_ = pkg.IsEmbedded()
		_ = pkg.HasTag()
		_ = pkg.Tag()
		_ = pkg.IsBlank()
		_ = pkg.IsEmpty()

		// PtrToThis.
		_, _ = ty.PtrToThis()

		// References.
		refs := ty.AppendReferences(nil)
		if len(refs) > 0 {
			rid := refs[0]
			if int(rid) <= limit {
				_, _ = tb.ParseGoType(rid)
			}
		}

		// Struct fields.
		if s, ok := ty.Struct(); ok {
			_, _ = s.Fields()
		}
		// Pointer elem.
		if p, ok := ty.Pointer(); ok {
			_ = p.Elem()
		}
		// Slice elem.
		if sl, ok := ty.Slice(); ok {
			_ = sl.Elem()
		}
		// Map key/value.
		if m, ok := ty.Map(); ok {
			_, _, _ = m.KeyValue()
		}
		// Func signature.
		if fn, ok := ty.Func(); ok {
			_, _, _, _ = fn.Signature()
		}
		// Chan dir/elem.
		if ch, ok := ty.Chan(); ok {
			_, _, _ = ch.DirElem()
		}
		// Array len/elem.
		if ar, ok := ty.Array(); ok {
			_, _, _ = ar.LenElem()
		}
		// Interface methods and their names.
		if in, ok := ty.Interface(); ok {
			ims, _ := in.Methods(nil)
			for _, im := range ims {
				nn := im.Name.Resolve(tb)
				_ = nn.Name()
				_ = nn.UnsafeName()
				_ = nn.IsExported()
				_ = nn.IsEmbedded()
				_ = nn.HasTag()
				_ = nn.Tag()
			}
		}

		// Uncommon methods and their names.
		ms, _ := ty.Methods(nil)
		for _, m := range ms {
			nn := m.Name.Resolve(tb)
			_ = nn.Name()
			_ = nn.UnsafeName()
			_ = nn.IsExported()
			_ = nn.IsEmbedded()
			_ = nn.HasTag()
			_ = nn.Tag()
		}
	})
}

type nopCloser struct{}

func (nopCloser) Close() error { return nil }
