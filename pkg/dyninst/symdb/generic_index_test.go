// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package symdb

import (
	"cmp"
	"debug/dwarf"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
)

type indexResult struct {
	name   string
	offset dwarf.Offset
}

// builderFactory creates a genericFuncIndexBuilder for testing.
type builderFactory struct {
	name   string
	create func(t *testing.T) genericFuncIndexBuilder
}

func getBuilderFactories(t *testing.T) []builderFactory {
	t.Helper()
	return []builderFactory{
		{
			name:   "in_memory",
			create: func(_ *testing.T) genericFuncIndexBuilder { return &inMemGenericFuncIndexBuilder{} },
		},
		{
			name: "on_disk",
			create: func(t *testing.T) genericFuncIndexBuilder {
				dc := newTestDiskCache(t)
				b, err := newOnDiskGenericFuncIndexBuilder(dc, "test")
				require.NoError(t, err)
				t.Cleanup(func() { _ = b.Close() })
				return b
			},
		},
	}
}

func newTestDiskCache(t *testing.T) *object.DiskCache {
	t.Helper()
	dc, err := object.NewDiskCache(object.DiskCacheConfig{
		DirPath:       t.TempDir(),
		MaxTotalBytes: 512 << 20,
	})
	require.NoError(t, err)
	return dc
}

func collectForPackage(idx genericFuncIndex, pkgName string) []indexResult {
	var results []indexResult
	for name, offset := range idx.forPackage(pkgName) {
		results = append(results, indexResult{name, offset})
	}
	slices.SortFunc(results, func(a, b indexResult) int {
		return cmp.Or(cmp.Compare(a.name, b.name), cmp.Compare(a.offset, b.offset))
	})
	return results
}

func TestGenericFuncIndex(t *testing.T) {
	for _, factory := range getBuilderFactories(t) {
		t.Run(factory.name, func(t *testing.T) {
			t.Run("empty", func(t *testing.T) {
				builder := factory.create(t)
				idx, err := builder.build()
				require.NoError(t, err)
				defer idx.Close()

				assert.Empty(t, collectForPackage(idx, "foo"))
			})

			t.Run("single_entry", func(t *testing.T) {
				builder := factory.create(t)
				require.NoError(t, builder.add("main.Foo[...]", 100))
				idx, err := builder.build()
				require.NoError(t, err)
				defer idx.Close()

				assert.Equal(t, []indexResult{{"main.Foo[...]", 100}}, collectForPackage(idx, "main"))
			})

			t.Run("multiple_packages", func(t *testing.T) {
				builder := factory.create(t)
				require.NoError(t, builder.add("main.Foo[...]", 100))
				require.NoError(t, builder.add("lib.Bar[...]", 200))
				require.NoError(t, builder.add("lib.Baz[...]", 300))
				require.NoError(t, builder.add("other.Qux[...]", 400))
				idx, err := builder.build()
				require.NoError(t, err)
				defer idx.Close()

				assert.Equal(t, []indexResult{
					{"lib.Bar[...]", 200},
					{"lib.Baz[...]", 300},
				}, collectForPackage(idx, "lib"))

				assert.Equal(t, []indexResult{{"main.Foo[...]", 100}}, collectForPackage(idx, "main"))
				assert.Empty(t, collectForPackage(idx, "unknown"))
			})

			t.Run("no_prefix_confusion", func(t *testing.T) {
				builder := factory.create(t)
				require.NoError(t, builder.add("lib.Foo[...]", 100))
				require.NoError(t, builder.add("library.Bar[...]", 200))
				idx, err := builder.build()
				require.NoError(t, err)
				defer idx.Close()

				assert.Equal(t, []indexResult{{"lib.Foo[...]", 100}}, collectForPackage(idx, "lib"))
			})

			t.Run("duplicates_preserved", func(t *testing.T) {
				builder := factory.create(t)
				require.NoError(t, builder.add("main.Foo[...]", 300))
				require.NoError(t, builder.add("main.Foo[...]", 100))
				require.NoError(t, builder.add("main.Foo[...]", 200))
				idx, err := builder.build()
				require.NoError(t, err)
				defer idx.Close()

				assert.Equal(t, []indexResult{
					{"main.Foo[...]", 100},
					{"main.Foo[...]", 200},
					{"main.Foo[...]", 300},
				}, collectForPackage(idx, "main"))
			})

			t.Run("methods_and_functions", func(t *testing.T) {
				builder := factory.create(t)
				require.NoError(t, builder.add("main.genericFunc[...]", 100))
				require.NoError(t, builder.add("main.GenericType[...].Method", 200))
				require.NoError(t, builder.add("main.(*GenericType[...]).PtrMethod", 300))
				idx, err := builder.build()
				require.NoError(t, err)
				defer idx.Close()

				results := collectForPackage(idx, "main")
				assert.Len(t, results, 3)
			})

			t.Run("long_package_paths", func(t *testing.T) {
				builder := factory.create(t)
				require.NoError(t, builder.add("github.com/org/repo/pkg/sub.Filter[...]", 100))
				require.NoError(t, builder.add("github.com/org/repo/pkg/sub.Map[...]", 200))
				require.NoError(t, builder.add("github.com/org/repo/pkg.Other[...]", 300))
				idx, err := builder.build()
				require.NoError(t, err)
				defer idx.Close()

				assert.Equal(t, []indexResult{
					{"github.com/org/repo/pkg/sub.Filter[...]", 100},
					{"github.com/org/repo/pkg/sub.Map[...]", 200},
				}, collectForPackage(idx, "github.com/org/repo/pkg/sub"))

				assert.Equal(t, []indexResult{
					{"github.com/org/repo/pkg.Other[...]", 300},
				}, collectForPackage(idx, "github.com/org/repo/pkg"))
			})
		})
	}
}
