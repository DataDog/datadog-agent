// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen

import (
	"maps"
	"math"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/gotype"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

var expectedImplementors = map[string][]string{
	"context.Context": {
		"*context.backgroundCtx",
		"*context.cancelCtx",
		"*context.emptyCtx",
		"*context.stopCtx",
		"*context.timerCtx",
		"*context.todoCtx",
		"*context.valueCtx",
		"*context.withoutCancelCtx",
		"*net.onlyValuesCtx",
		"*orchestrion.glsContext",
		"context.backgroundCtx",
		"context.emptyCtx",
		"context.stopCtx",
		"context.todoCtx",
		"context.withoutCancelCtx",
	},
	"net.Conn": {
		"*net.IPConn",
		"*net.TCPConn",
		"*net.UDPConn",
		"*net.UnixConn",
		"*net.conn",
		"*net.dialResult",
		"*net.tcpConnWithoutReadFrom",
		"*net.tcpConnWithoutWriteTo",
		"*tls.Conn",
		"*transport.bufConn",
		"net.dialResult",
		"net.tcpConnWithoutReadFrom",
		"net.tcpConnWithoutWriteTo",
	},
}
var interestingInterfaceNames = makeSet(expectedImplementors)

func makeSet[K comparable, V any](m map[K]V) map[K]struct{} {
	s := make(map[K]struct{}, len(m))
	for k := range m {
		s[k] = struct{}{}
	}
	return s
}

func TestImplementorIterator(t *testing.T) {
	cfgs := testprogs.MustGetCommonConfigs(t)
	t.Run("in-memory", func(t *testing.T) {
		factory := &inMemoryGoTypeIndexFactory{}
		testImplementorIterator(t, cfgs[0], factory)
	})
	t.Run("on-disk", func(t *testing.T) {
		diskCache, err := object.NewDiskCache(object.DiskCacheConfig{
			DirPath:       t.TempDir(),
			MaxTotalBytes: math.MaxUint64,
		})
		require.NoError(t, err)
		factory := &onDiskGoTypeIndexFactory{diskCache: diskCache}
		testImplementorIterator(t, cfgs[0], factory)
	})

}

func testImplementorIterator(t *testing.T, cfg testprogs.Config, factory goTypeIndexFactory) {
	typeTab, interestingInterfaces, methodIndex := buildMethodIndex(
		t, cfg, "sample", factory, interestingInterfaceNames,
	)
	defer func() { require.NoError(t, typeTab.Close()) }()
	defer func() { require.NoError(t, methodIndex.Close()) }()

	interfaceNames := slices.Sorted(maps.Keys(expectedImplementors))
	require.Equal(t, interfaceNames, slices.Sorted(maps.Keys(interestingInterfaces)))
	ii := makeImplementorIterator(methodIndex)
	var methodsBuf []gotype.IMethod
	for _, name := range interfaceNames {
		goType, ok := interestingInterfaces[name]
		require.True(t, ok)
		iface, ok := goType.Interface()
		require.True(t, ok)
		var err error
		methodsBuf, err = iface.Methods(methodsBuf[:0])
		require.NoError(t, err)
		var impls []string
		for ii.seek(methodsBuf); ii.valid(); ii.next() {
			tt, err := typeTab.ParseGoType(ii.cur())
			require.NoError(t, err)
			impls = append(impls, tt.Name().Name())
		}
		slices.Sort(impls)
		require.Equal(t, expectedImplementors[name], impls)
	}
}

// Build a method index from the sample binary and find the interesting interfaces.
func buildMethodIndex(
	t testing.TB, cfg testprogs.Config, progName string,
	factory goTypeIndexFactory,
	interfaceNames map[string]struct{},
) (*gotype.Table, map[string]gotype.Type, methodToGoTypeIndex) {
	prog := testprogs.MustGetBinary(t, progName, cfg)
	obj, err := object.OpenElfFileWithDwarf(prog)
	require.NoError(t, err)
	defer func() { require.NoError(t, obj.Close()) }()
	// Prepare the main DWARF visitor that will gather all the information we
	// need from the binary.
	arch := obj.Architecture()
	d := obj.DwarfData()
	typeTab, err := gotype.NewTable(obj)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, typeTab.Close()) })

	typeIndexBuilder, err := factory.newGoTypeToOffsetIndexBuilder(0, typeTab.DataByteSize())
	require.NoError(t, err)
	interests, _ := makeInterests(nil)
	{
		_, err := processDwarf(interests, d, arch, typeIndexBuilder)
		require.NoError(t, err)
	}
	typeIndex, err := typeIndexBuilder.build()
	require.NoError(t, err)
	defer func() { require.NoError(t, typeIndex.Close()) }()

	interestingInterfaces := make(map[string]gotype.Type)

	methodIndexBuilder, err := factory.newMethodToGoTypeIndexBuilder(0, typeTab.DataByteSize())
	require.NoError(t, err)
	var methodBuf []gotype.Method
	for tid := range typeIndex.allGoTypes() {
		goType, err := typeTab.ParseGoType(tid)
		require.NoError(t, err, "failed to parse go type %q", tid)
		methodBuf, err := goType.Methods(methodBuf[:0])
		require.NoError(t, err, "failed to get methods for go type %q", tid)
		for _, m := range methodBuf {
			methodIndexBuilder.addMethod(m, tid)
		}
		name := goType.Name().Name()
		if _, ok := interfaceNames[name]; ok {
			interestingInterfaces[name] = goType
		}
	}
	methodIndex, err := methodIndexBuilder.build()
	require.NoError(t, err)
	return typeTab, interestingInterfaces, methodIndex
}

func BenchmarkImplementorIterator(b *testing.B) {
	cfgs := testprogs.MustGetCommonConfigs(b)
	factory := &inMemoryGoTypeIndexFactory{}
	_, interestingInterfaces, methodIndex := buildMethodIndex(
		b, cfgs[0], "sample", factory, interestingInterfaceNames,
	)
	names := slices.Sorted(maps.Keys(interestingInterfaces))
	for _, name := range names {
		b.Run(name, func(b *testing.B) {
			goType, ok := interestingInterfaces[name]
			require.True(b, ok)
			iface, ok := goType.Interface()
			require.True(b, ok)
			imethods, err := iface.Methods(nil)
			require.NoError(b, err)
			ii := makeImplementorIterator(methodIndex)
			b.ResetTimer()
			for b.Loop() {
				for ii.seek(imethods); ii.valid(); ii.next() {
					_ = ii.cur()
				}
			}
		})
	}
}
