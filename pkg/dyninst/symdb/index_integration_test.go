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

// TestForPackageEscapeRoundTrip exercises forPackage on the prepass
// indexes built from the sample testprog. The sample contains a
// package "lib.v2" whose name has a dot in its last path segment —
// DWARF emits the linker symbol with the segment-dot escaped as %2e,
// while the compile unit's DW_AT_name keeps the unescaped form.
// forPackage takes the unescaped form and must internally escape it
// before binary-searching.
//
// Concretely we assert:
//  1. typesByPackage.forPackage("…/lib.v2") yields type DIE offsets
//     that the typeInfoByOffset reverse index reports under the
//     escaped name form (lib%2ev2.…).
//  2. typesByPackage.forPackage("…/lib") yields no entries whose
//     names contain "lib%2ev2." — the escape boundary keeps sibling
//     packages with shared dot-prefixed segments separate.
//  3. genericFuncs.forPackage works on unescaped names too.
//  4. typesByPackage reaches non-generic types like lib.v2's V2Type.
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

			// (1) lib.v2's type DIEs must be reachable, and their
			// names — fetched via the reverse index — must use the
			// escaped package form.
			var libV2Names []string
			for offset := range indexes.typesByPackage.forPackage(libV2Pkg) {
				name, _, ok := indexes.typeInfoByOffset.infoAt(offset)
				require.True(t, ok, "typeInfoByOffset has no entry for offset 0x%x", offset)
				libV2Names = append(libV2Names, name)
			}
			require.NotEmpty(t, libV2Names,
				"forPackage(%q) returned no type entries — escape mismatch suspected", libV2Pkg)
			for _, name := range libV2Names {
				require.Contains(t, name, "lib%2ev2.",
					"expected entry %q to use the escaped package form", name)
			}

			// (2) forPackage("lib") must not yield lib.v2 entries.
			for offset := range indexes.typesByPackage.forPackage(libPkg) {
				name, _, ok := indexes.typeInfoByOffset.infoAt(offset)
				require.True(t, ok)
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

			// (4) Non-generic types in lib.v2 are now reachable via
			// typesByPackage. V2Type is declared as `type V2Type
			// struct{}` in lib.v2 — its DIE name is
			// "…/lib%2ev2.V2Type" with no [go.shape.…] suffix, which
			// the previous genericTypes index would have rejected.
			// This is the new-coverage signal: the index-driven
			// emission walk picks up types defined elsewhere as
			// well as types whose only mentions are in foreign CUs.
			foundV2Type := false
			for _, name := range libV2Names {
				if strings.HasSuffix(name, "lib%2ev2.V2Type") {
					foundV2Type = true
					break
				}
			}
			require.True(t, foundV2Type,
				"expected V2Type non-generic struct to appear in typesByPackage(%q); got %v",
				libV2Pkg, libV2Names)
		})
	}
}
