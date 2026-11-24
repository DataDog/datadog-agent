// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package irgen

import (
	"debug/dwarf"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompilerSplitLocals(t *testing.T) {
	for _, name := range []string{"err.itab", "err.data", "s.len", "&escaped", "for"} {
		entry := &dwarf.Entry{Tag: dwarf.TagVariable, Field: []dwarf.Field{{Attr: dwarf.AttrName, Val: name}}}
		variable, err := processVariable(nil, entry, false, nil, nil, 8, nil)
		require.NoError(t, err)
		require.Nil(t, variable)
		entry.Tag = dwarf.TagFormalParameter
		_, err = processVariable(nil, entry, false, nil, nil, 8, nil)
		require.Error(t, err, "formal parameters must still be processed")
	}
	for _, name := range []string{"~r0", "~r1"} {
		entry := &dwarf.Entry{Tag: dwarf.TagFormalParameter, Field: []dwarf.Field{{Attr: dwarf.AttrName, Val: name}}}
		_, err := processVariable(nil, entry, false, nil, nil, 8, nil)
		require.Error(t, err, "synthetic result %s must still be processed", name)
	}
	for _, name := range []string{"err", "data", "_", "日本語", "value2"} {
		entry := &dwarf.Entry{Tag: dwarf.TagVariable, Field: []dwarf.Field{{Attr: dwarf.AttrName, Val: name}}}
		_, err := processVariable(nil, entry, false, nil, nil, 8, nil)
		require.Error(t, err, "source local %s must still be processed", name)
	}
}
