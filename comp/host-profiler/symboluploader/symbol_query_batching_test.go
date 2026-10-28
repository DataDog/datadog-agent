// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package symboluploader

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/ebpf-profiler/libpf"

	"github.com/DataDog/datadog-agent/comp/host-profiler/symboluploader/symbol"
)

// newTestReporter returns a reporter whose output goes to slog.Default.
func newTestReporter() *uploadReporter {
	return newUploadReporter()
}

func newQueryBatch(elfs []*symbol.Elf, candidateSource symbol.Source, querierCount int) []ElfWithQueryResults {
	batch := make([]ElfWithQueryResults, len(elfs))
	for i, e := range elfs {
		batch[i] = ElfWithQueryResults{
			Elf:             e,
			CandidateSource: candidateSource,
			QueryResults:    make([]SymbolQueryResult, querierCount),
		}
	}
	return batch
}

// Simulate response from symbols query.
type entry struct {
	decision     SymbolFileDecision
	symbolSource string
	// emit this entry twice, to exercise "first entry for a buildID wins"
	duplicate bool
}

type mockSymbolQuerier struct {
	stored         map[string]entry
	err            error
	ncalls         int
	lastCandidates []SymbolCandidate
}

func (m *mockSymbolQuerier) QuerySymbols(_ context.Context, candidates []SymbolCandidate,
	_ string) ([]SymbolFile, error) {
	m.ncalls++
	m.lastCandidates = candidates
	if m.err != nil {
		return nil, m.err
	}

	var symbolFiles []SymbolFile
	for i, c := range candidates {
		e, ok := m.stored[c.BuildId]
		if !ok {
			continue
		}
		file := SymbolFile{
			ID:           strconv.Itoa(i),
			BuildId:      c.BuildId,
			SymbolSource: e.symbolSource,
			Decision:     e.decision,
		}
		symbolFiles = append(symbolFiles, file)
		if e.duplicate {
			symbolFiles = append(symbolFiles, file)
		}
	}
	return symbolFiles, nil
}

func candidateBuildIDs(candidates []SymbolCandidate) []string {
	ids := make([]string, len(candidates))
	for i, c := range candidates {
		ids[i] = c.BuildId
	}
	return ids
}

func TestBatchSymbolQuerier_Multiplexing(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	t.Run("Empty batch", func(t *testing.T) {
		q := &mockSymbolQuerier{}
		results := ExecuteSymbolQueryBatch(ctx, nil, []SymbolQuerier{q}, newTestReporter())
		require.Empty(t, results)
		require.Zero(t, q.ncalls)
	})

	t.Run("Multiple buildIDs", func(t *testing.T) {
		fileID := libpf.FileID{}
		elfs := []*symbol.Elf{
			symbol.NewElfForTest("/usr/bin/foo-0", "arch1", "non_existing_buildid", "", fileID), // unknown buildID to the backend
			symbol.NewElfForTest("/usr/bin/foo-1", "arch1", "buildid1", "buildid2", fileID),     // multiple buildIDs, gnu_build_id should be used
			symbol.NewElfForTest("/usr/bin/foo-2", "arch2", "buildid1", "", fileID),             // arch mismatch
			symbol.NewElfForTest("/usr/bin/foo-3", "arch1", "", "", fileID),                     // empty buildID
			symbol.NewElfForTest("/usr/bin/foo-4", "arch1", "buildid2", "", fileID),             // multiple matching buildIDs, first one should be used
		}
		q := &mockSymbolQuerier{stored: map[string]entry{
			"buildid1": {decision: decisionUpload},
			"buildid2": {decision: decisionNoUpload, duplicate: true},
		}}

		results := ExecuteSymbolQueryBatch(ctx, newQueryBatch(elfs, symbol.SourceSymbolTable, 1), []SymbolQuerier{q}, newTestReporter())

		require.Len(t, results, 5)
		require.ElementsMatch(t, []ElfWithQueryResults{
			{
				Elf:             elfs[0],
				CandidateSource: symbol.SourceSymbolTable,
				QueryResults:    []SymbolQueryResult{{Upload: true}},
			},
			{
				Elf:             elfs[1],
				CandidateSource: symbol.SourceSymbolTable,
				QueryResults:    []SymbolQueryResult{{Upload: true}},
			},
			{
				Elf:             elfs[2],
				CandidateSource: symbol.SourceSymbolTable,
				QueryResults:    []SymbolQueryResult{{Err: errors.New("arch mismatch: expected arch1, got arch2")}},
			},
			{
				Elf:             elfs[3],
				CandidateSource: symbol.SourceSymbolTable,
				QueryResults:    []SymbolQueryResult{{Err: errors.New("empty buildID")}},
			},
			{
				Elf:             elfs[4],
				CandidateSource: symbol.SourceSymbolTable,
				QueryResults:    []SymbolQueryResult{{Upload: false}},
			},
		}, results)

		require.Equal(t, 1, q.ncalls)
		require.ElementsMatch(t, []string{"non_existing_buildid", "buildid1", "buildid2"}, candidateBuildIDs(q.lastCandidates))
	})

	t.Run("No valid build ID sends no request", func(t *testing.T) {
		q := &mockSymbolQuerier{}
		elfs := []*symbol.Elf{
			symbol.NewElfForTest("/usr/bin/foo-emptyid", "arch1", "", "", libpf.FileID{}),
			symbol.NewElfForTest("/usr/bin/foo-archmismatch", "arch2", "buildid1", "", libpf.FileID{}),
		}

		results := ExecuteSymbolQueryBatch(ctx, newQueryBatch(elfs, symbol.SourceSymbolTable, 1), []SymbolQuerier{q}, newTestReporter())

		require.Zero(t, q.ncalls)
		require.Len(t, results, 2)
		require.ErrorContains(t, results[0].QueryResults[0].Err, "empty buildID")
		require.ErrorContains(t, results[1].QueryResults[0].Err, "arch mismatch")
	})

	t.Run("Fail-open when the decision is missing, empty, or unrecognized", func(t *testing.T) {
		q := &mockSymbolQuerier{stored: map[string]entry{
			"buildid-empty":   {decision: ""},
			"buildid-unknown": {decision: "lease"},
		}}
		elfs := []*symbol.Elf{
			symbol.NewElfForTest("/usr/bin/foo-empty", "arch1", "buildid-empty", "", libpf.FileID{}),
			symbol.NewElfForTest("/usr/bin/foo-unknown", "arch1", "buildid-unknown", "", libpf.FileID{}),
			symbol.NewElfForTest("/usr/bin/foo-absent", "arch1", "buildid-absent", "", libpf.FileID{}),
		}

		results := ExecuteSymbolQueryBatch(ctx, newQueryBatch(elfs, symbol.SourceSymbolTable, 1), []SymbolQuerier{q}, newTestReporter())

		for i, r := range results {
			require.NoError(t, r.QueryResults[0].Err)
			require.True(t, r.QueryResults[0].Upload, "elf %d should fail open to upload", i)
		}
	})

	t.Run("Same build ID, mixed sources: one candidate, every member uploads", func(t *testing.T) {
		q := &mockSymbolQuerier{stored: map[string]entry{
			"shared-buildid": {decision: decisionUpload},
		}}
		fileID := libpf.FileID{}
		lesser := symbol.NewElfForTest("/usr/bin/lesser", "arch1", "shared-buildid", "", fileID)
		better := symbol.NewElfForTest("/usr/bin/better", "arch1", "shared-buildid", "", fileID)
		batch := []ElfWithQueryResults{
			{Elf: lesser, CandidateSource: symbol.SourceSymbolTable, QueryResults: make([]SymbolQueryResult, 1)},
			{Elf: better, CandidateSource: symbol.SourceDebugInfo, QueryResults: make([]SymbolQueryResult, 1)},
		}

		results := ExecuteSymbolQueryBatch(ctx, batch, []SymbolQuerier{q}, newTestReporter())

		require.Equal(t, 1, q.ncalls)
		require.Len(t, q.lastCandidates, 1)
		require.Equal(t, "shared-buildid", q.lastCandidates[0].BuildId)
		require.Equal(t, symbol.SourceDebugInfo.String(), q.lastCandidates[0].SymbolSource)
		require.Equal(t, "better", q.lastCandidates[0].FileName)

		require.NoError(t, results[0].QueryResults[0].Err)
		require.True(t, results[0].QueryResults[0].Upload)
		require.NoError(t, results[1].QueryResults[0].Err)
		require.True(t, results[1].QueryResults[0].Upload, "best source must upload")
	})

	t.Run("Same build ID, tied sources: every tied binary uploads", func(t *testing.T) {
		q := &mockSymbolQuerier{stored: map[string]entry{
			"tied-buildid": {decision: decisionUpload},
		}}
		fileID := libpf.FileID{}
		first := symbol.NewElfForTest("/usr/bin/first", "arch1", "tied-buildid", "", fileID)
		second := symbol.NewElfForTest("/usr/bin/second", "arch1", "tied-buildid", "", fileID)
		batch := []ElfWithQueryResults{
			{Elf: first, CandidateSource: symbol.SourceDebugInfo, QueryResults: make([]SymbolQueryResult, 1)},
			{Elf: second, CandidateSource: symbol.SourceDebugInfo, QueryResults: make([]SymbolQueryResult, 1)},
		}

		results := ExecuteSymbolQueryBatch(ctx, batch, []SymbolQuerier{q}, newTestReporter())

		require.Equal(t, 1, q.ncalls)
		require.Len(t, q.lastCandidates, 1)
		require.Equal(t, "first", q.lastCandidates[0].FileName)

		require.NoError(t, results[0].QueryResults[0].Err)
		require.True(t, results[0].QueryResults[0].Upload, "every binary tied at the best source must upload, not only one")
		require.NoError(t, results[1].QueryResults[0].Err)
		require.True(t, results[1].QueryResults[0].Upload, "every binary tied at the best source must upload, not only one")
	})

	t.Run("Errors on both endpoints", func(t *testing.T) {
		q1 := &mockSymbolQuerier{err: errors.New("error")}
		q2 := &mockSymbolQuerier{err: errors.New("error")}
		elfs := []*symbol.Elf{
			symbol.NewElfForTest("/usr/bin/foo", "arch0", "buildid1", "", libpf.FileID{}),
		}

		results := ExecuteSymbolQueryBatch(ctx, newQueryBatch(elfs, symbol.SourceSymbolTable, 2), []SymbolQuerier{q1, q2}, newTestReporter())
		require.Error(t, results[0].QueryResults[0].Err)
		require.Error(t, results[0].QueryResults[1].Err)
	})

	t.Run("Errors on one endpoint", func(t *testing.T) {
		q1 := &mockSymbolQuerier{err: errors.New("error")}
		q2 := &mockSymbolQuerier{stored: map[string]entry{"buildid1": {decision: decisionUpload}}}
		elfs := []*symbol.Elf{
			symbol.NewElfForTest("/usr/bin/foo", "arch1", "buildid1", "", libpf.FileID{}),
		}

		results := ExecuteSymbolQueryBatch(ctx, newQueryBatch(elfs, symbol.SourceSymbolTable, 2), []SymbolQuerier{q1, q2}, newTestReporter())
		require.Error(t, results[0].QueryResults[0].Err)
		require.NoError(t, results[0].QueryResults[1].Err)
		require.True(t, results[0].QueryResults[1].Upload)
	})
}
