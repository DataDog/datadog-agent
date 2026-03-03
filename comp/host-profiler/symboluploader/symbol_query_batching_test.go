// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package symboluploader

import (
	"context"
	"errors"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/ebpf-profiler/libpf"

	"github.com/DataDog/datadog-agent/comp/host-profiler/symboluploader/symbol"
)

type key struct {
	buildID string
	arch    string
}

type value struct {
	symbolSource string
	buildIDType  string
}

type symbolMap map[key][]value
type errorMap map[string]error

type mockSymbolQuerier struct {
	m      symbolMap
	e      errorMap
	ncalls int
}

func (m *mockSymbolQuerier) QuerySymbols(_ context.Context, buildIDs []string, arch string) ([]SymbolFile, error) {
	m.ncalls++
	if err, ok := m.e[arch]; ok {
		return nil, err
	}

	buildIDsCopy := make([]string, len(buildIDs))
	copy(buildIDsCopy, buildIDs)

	// randomly shuffle the buildIDs
	for i := range buildIDsCopy {
		j := rand.Intn(i + 1) //nolint:gosec
		buildIDsCopy[i], buildIDsCopy[j] = buildIDsCopy[j], buildIDsCopy[i]
	}

	var symbolFiles []SymbolFile
	for _, buildID := range buildIDsCopy {
		if values, ok := m.m[key{buildID, arch}]; ok {
			for _, s := range values {
				symbolFiles = append(symbolFiles, SymbolFile{
					BuildID:      buildID,
					SymbolSource: s.symbolSource,
					BuildIDType:  s.buildIDType,
				})
			}
		}
	}

	return symbolFiles, nil
}

func newTestQuerier1() *mockSymbolQuerier {
	e := errorMap{"arch0": errors.New("error")}

	m := symbolMap{
		{buildID: "buildid1", arch: "arch1"}: []value{{symbolSource: "symbol_table", buildIDType: "gnu_build_id"}},
		{buildID: "buildid2", arch: "arch1"}: []value{
			{symbolSource: "symbol_table", buildIDType: "gnu_build_id"},
			{symbolSource: "debug_info", buildIDType: "go_build_id"}},
		{buildID: "buildid1", arch: "arch2"}: []value{{symbolSource: "debug_info", buildIDType: "gnu_build_id"}},
	}

	return &mockSymbolQuerier{m: m, e: e}
}

func newTestQuerier2() *mockSymbolQuerier {
	e := errorMap{
		"arch0": errors.New("error"),
		"arch2": errors.New("error")}

	m := symbolMap{
		{buildID: "buildid1", arch: "arch1"}: []value{{symbolSource: "debug_info", buildIDType: "gnu_build_id"}},
	}

	return &mockSymbolQuerier{m: m, e: e}
}

func TestBatchSymbolQuerier_Multiplexing(t *testing.T) {
	querier1 := newTestQuerier1()
	querier2 := newTestQuerier2()

	queriers := []SymbolQuerier{querier1, querier2}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	t.Run("Empty batch", func(t *testing.T) {
		results := ExecuteSymbolQueryBatch(ctx, nil, queriers)
		require.Empty(t, results)
	})

	t.Run("Multiple buildIDs", func(t *testing.T) {
		fileID := libpf.FileID{}
		elfs := []*symbol.Elf{
			symbol.NewElfForTest("arch1", "non_existing_buildid", "", fileID),
			symbol.NewElfForTest("arch1", "buildid1", "buildid2", fileID), // multiple buildIDs, gnu_build_id should be used
			symbol.NewElfForTest("arch2", "buildid1", "", fileID),         // arch mismatch
			symbol.NewElfForTest("arch1", "", "", fileID),                 // empty buildID
			symbol.NewElfForTest("arch1", "buildid2", "", fileID),         // multiple matching buildIDs, first one should be used
		}
		results := ExecuteSymbolQueryBatch(ctx, elfs, queriers)
		require.ElementsMatch(t, []ElfWithBackendSources{
			{
				Elf: elfs[0],
				BackendSymbolSources: []SymbolQueryResult{
					{
						SymbolSource: symbol.SourceNone,
					},
					{
						SymbolSource: symbol.SourceNone,
					},
				},
			},
			{
				Elf: elfs[1],
				BackendSymbolSources: []SymbolQueryResult{
					{
						SymbolSource: symbol.SourceSymbolTable,
					},
					{
						SymbolSource: symbol.SourceDebugInfo,
					},
				},
			},
			{
				Elf: elfs[2],
				BackendSymbolSources: []SymbolQueryResult{
					{
						Err: errors.New("arch mismatch: expected arch1, got arch2"),
					},
					{
						Err: errors.New("arch mismatch: expected arch1, got arch2"),
					},
				},
			},
			{
				Elf: elfs[3],
				BackendSymbolSources: []SymbolQueryResult{
					{
						Err: errors.New("empty buildID"),
					},
					{
						Err: errors.New("empty buildID"),
					},
				},
			},
			{
				Elf: elfs[4],
				BackendSymbolSources: []SymbolQueryResult{
					{ // multiple matching buildIDs, first one should be used
						SymbolSource: symbol.SourceSymbolTable,
					},
					{
						SymbolSource: symbol.SourceNone,
					},
				},
			},
		}, results)
	})

	t.Run("Errors on both endpoints", func(t *testing.T) {
		elfs := []*symbol.Elf{
			symbol.NewElfForTest("arch0", "buildid1", "", libpf.FileID{}),
		}
		results := ExecuteSymbolQueryBatch(ctx, elfs, queriers)
		require.Error(t, results[0].BackendSymbolSources[0].Err)
		require.Error(t, results[0].BackendSymbolSources[1].Err)
	})

	t.Run("Errors on one endpoint", func(t *testing.T) {
		elfs := []*symbol.Elf{
			symbol.NewElfForTest("arch2", "buildid1", "", libpf.FileID{}),
		}
		results := ExecuteSymbolQueryBatch(ctx, elfs, queriers)
		require.Error(t, results[0].BackendSymbolSources[1].Err)
		require.NoError(t, results[0].BackendSymbolSources[0].Err)
		require.Equal(t, symbol.SourceDebugInfo, results[0].BackendSymbolSources[0].SymbolSource)
	})
}
