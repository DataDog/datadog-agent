// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package symdb

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

// TestForPackageEscapeRoundTrip exercises funcOffsetByNameIndex.forPackage
// against the indexes built from the sample testprog. The sample contains a
// package "lib.v2" whose name has a dot in its last path segment — DWARF
// emits the linker symbol with the segment-dot escaped as %2e, while the
// compile unit's DW_AT_name keeps the unescaped form. forPackage takes the
// unescaped form and must internally escape it before binary-searching.
//
// Concretely we assert:
//  1. inlineDefs.forPackage("…/lib.v2") returns the inline definitions for
//     lib.v2's symbols (otherwise cross-CU inline replay misses them).
//  2. inlineDefs.forPackage("…/lib") yields no entries from lib.v2 — the
//     escape boundary keeps sibling packages with shared dot-prefixed
//     segments separate (the issue Andrei flagged).
//  3. genericFuncs.forPackage works on unescaped names too.
//  4. genericTypes.forPackage("…/lib.v2") returns lib.v2's generic types
//     in DWARF (escaped) form — the index keys must match the lookup
//     prefix's escape form, otherwise displaced generic types in
//     dotted-segment packages are silently dropped.
func TestForPackageEscapeRoundTrip(t *testing.T) {
	cfgs, err := testprogs.GetCommonConfigs()
	require.NoError(t, err)
	for _, cfg := range cfgs {
		t.Run(cfg.String(), func(t *testing.T) {
			binaryPath, err := testprogs.GetBinary("sample", cfg)
			require.NoError(t, err)

			bin, err := openBinary(binaryPath, object.NewInMemoryLoader(), ExtractOptions{
				Scope: ExtractScopeMainModuleOnly,
			})
			require.NoError(t, err)
			b := newPackagesIterator(bin, ExtractOptions{
				Scope: ExtractScopeMainModuleOnly,
			})
			t.Cleanup(b.close)

			indexes, err := b.buildPrePassIndexes()
			require.NoError(t, err)
			b.indexes = indexes
			t.Cleanup(b.indexes.close)

			const (
				libPkg   = "github.com/DataDog/datadog-agent/pkg/dyninst/testprogs/progs/sample/lib"
				libV2Pkg = "github.com/DataDog/datadog-agent/pkg/dyninst/testprogs/progs/sample/lib.v2"
			)

			// (1) lib.v2's inline definitions must be reachable.
			var libV2Names []string
			for name := range indexes.inlineDefs.forPackage(libV2Pkg) {
				libV2Names = append(libV2Names, name)
			}
			require.NotEmpty(t, libV2Names,
				"forPackage(%q) returned no inline-def entries — escape mismatch suspected", libV2Pkg)
			for _, name := range libV2Names {
				// Stored names are in DWARF (escaped) form.
				require.Contains(t, name, "lib%2ev2.",
					"expected entry %q to use the escaped package form", name)
			}

			// (2) forPackage("lib") must not yield lib.v2 entries.
			for name := range indexes.inlineDefs.forPackage(libPkg) {
				require.False(t, strings.Contains(name, "lib%2ev2."),
					"forPackage(%q) leaked a lib.v2 entry: %s", libPkg, name)
			}

			// (3) genericFuncs uses the same forPackage mechanism. lib has
			// generic functions (Filter, Map, Reduce, NewImmutableSet, …),
			// so a lookup with the unescaped pkg name must yield entries.
			var libGenericNames []string
			for name := range indexes.genericFuncs.forPackage(libPkg) {
				libGenericNames = append(libGenericNames, name)
			}
			require.NotEmpty(t, libGenericNames,
				"forPackage(%q) returned no generic-func entries", libPkg)
			for _, name := range libGenericNames {
				require.True(t, strings.HasPrefix(name, libPkg+"."),
					"genericFuncs entry %q does not belong to %q", name, libPkg)
			}

			// (4) genericTypes for a dotted-segment package. lib.v2
			// defines V2GenericBox[T]; if its key were stored unescaped
			// the escaped-prefix lookup would miss it.
			var libV2GenericTypeNames []string
			for name := range indexes.genericTypes.forPackage(libV2Pkg) {
				libV2GenericTypeNames = append(libV2GenericTypeNames, name)
			}
			require.NotEmpty(t, libV2GenericTypeNames,
				"forPackage(%q) returned no generic-type entries — escape mismatch in genericTypes keys?", libV2Pkg)
			for _, name := range libV2GenericTypeNames {
				require.Contains(t, name, "lib%2ev2.",
					"expected genericTypes entry %q to use the escaped package form", name)
			}
		})
	}
}
