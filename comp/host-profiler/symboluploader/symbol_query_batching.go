// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package symboluploader

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"sync"

	ddhttp "github.com/DataDog/datadog-agent/pkg/util/http"
	"go.opentelemetry.io/ebpf-profiler/libpf"

	"github.com/DataDog/datadog-agent/comp/host-profiler/symboluploader/symbol"
)

type SymbolQueryResult struct {
	Upload       bool
	StoredSource symbol.Source
	Err          error
}

type ElfWithQueryResults struct {
	*symbol.Elf
	// CandidateSource is the symbol sent to the backend for this binary.
	CandidateSource symbol.Source
	// QueryResults has one slot per querier (endpoint), in querier order.
	QueryResults []SymbolQueryResult
}

// GetSize returns the size of the underlying elf.
// It will return 0 if the size can't be retrieved.
func (e *ElfWithQueryResults) GetSize() int64 {
	return e.Elf.GetSize()
}

func fileHash(fileID libpf.FileID) string {
	if fileID == (libpf.FileID{}) {
		return ""
	}
	return fileID.StringNoQuotes()
}

// getBuildID returns the most appropriate build ID for an executable.
// It prioritizes GNU build ID, then Go build ID, then falls back to file hash.
// Special handling for Bazel builds where Go build ID is "redacted".
func getBuildID(gnuBuildID, goBuildID string, fileID libpf.FileID) string {
	// When building Go binaries, Bazel will set the Go build ID to "redacted" to
	// achieve deterministic builds. Since Go 1.24, the Gnu Build ID is inherited
	// from the Go build ID - if the Go build ID is "redacted", the Gnu Build ID will
	// be a hash of "redacted". In this case, we should use the file hash instead of build IDs.
	if goBuildID == "redacted" {
		return fileHash(fileID)
	}
	if gnuBuildID != "" {
		return gnuBuildID
	}
	if goBuildID != "" {
		return goBuildID
	}

	return fileHash(fileID)
}

func invokeQuerier(ctx context.Context, candidates []SymbolCandidate, arch string, querier SymbolQuerier, ind int,
	reporter *uploadReporter, buildIDToResult map[string][]*ElfWithQueryResults) {
	elfCount := 0
	for _, results := range buildIDToResult {
		elfCount += len(results)
	}
	symbolFiles, err := querier.QuerySymbols(ctx, candidates, arch)
	if err != nil {
		reason := reasonRequestError
		if sce, ok := errors.AsType[*ddhttp.StatusCodeError](err); ok && sce.StatusCode == http.StatusTooManyRequests {
			reason = reasonRateLimited
		}
		queryErr := fmt.Errorf("failed to query symbols: %w", err)
		if reason != reasonRateLimited {
			slog.Error("Failed to query symbols",
				slog.Int("endpoint", ind),
				slog.String("arch", arch),
				slog.Int("executables", elfCount),
				slog.String("error", err.Error()))
		}
		paths := make([]string, 0, elfCount)
		for _, results := range buildIDToResult {
			for _, result := range results {
				paths = append(paths, result.Path())
				result.QueryResults[ind].Err = queryErr
			}
		}
		reporter.Error(reason, elfCount, paths...)
		return
	}

	reporter.Info(reasonQueried, elfCount)

	// Among binaries sharing the same build ID, QueryResults.Upload defaults to true
	// for the one with the highest-priority source. The backend can only turn it off,
	// with a no_upload decision for the candidate.
	for _, symbolFile := range symbolFiles {
		storedSource, err := symbol.NewSource(symbolFile.SymbolSource)
		if err != nil {
			storedSource = symbol.SourceNone
		}
		for _, result := range buildIDToResult[symbolFile.BuildId] {
			if symbolFile.Decision == decisionNoUpload {
				result.QueryResults[ind].Upload = false
			}
			result.QueryResults[ind].StoredSource = storedSource
		}
	}
}

func (e *ElfWithQueryResults) fillWithError(err error) {
	for i := range e.QueryResults {
		e.QueryResults[i].Err = err
	}
}

func ExecuteSymbolQueryBatch(ctx context.Context, batch []ElfWithQueryResults, queriers []SymbolQuerier,
	reporter *uploadReporter) []ElfWithQueryResults {
	if len(batch) == 0 {
		return nil
	}

	slog.Debug("Querying symbols for executables", slog.Int("count", len(batch)))
	buildIDToResult := make(map[string][]*ElfWithQueryResults)

	// All the elfs in the batch are expected to have the same arch
	arch := batch[0].Elf.Arch()

	invalidBuildIDs := 0

	for i := range batch {
		result := &batch[i]
		if result.Elf.Arch() != arch {
			result.fillWithError(fmt.Errorf("arch mismatch: expected %s, got %s", arch, result.Elf.Arch()))
			invalidBuildIDs++
			continue
		}

		buildID := getBuildID(result.Elf.GnuBuildID(), result.Elf.GoBuildID(), result.Elf.FileID())
		if buildID == "" {
			result.fillWithError(errors.New("empty buildID"))
			invalidBuildIDs++
			continue
		}
		buildIDToResult[buildID] = append(buildIDToResult[buildID], result)
	}
	reporter.Error(reasonInvalidBuildID, invalidBuildIDs)

	// One candidate per build ID, declaring the highest-priority source found among the binaries
	// sharing that build ID. By default, upload is set to true for that candidate.
	candidates := make([]SymbolCandidate, 0, len(buildIDToResult))
	for buildID, results := range buildIDToResult {
		maxSource := results[0].CandidateSource
		fileName := ""
		for _, result := range results {
			if result.CandidateSource > maxSource {
				maxSource = result.CandidateSource
				fileName = filepath.Base(result.Path())
			}
		}
		if fileName == "" {
			fileName = filepath.Base(results[0].Path())
		}
		for _, result := range results {
			for ind := range result.QueryResults {
				result.QueryResults[ind].Upload = result.CandidateSource == maxSource
			}
		}
		candidates = append(candidates, SymbolCandidate{
			BuildId:      buildID,
			FileName:     fileName,
			SymbolSource: maxSource.String(),
		})
	}

	if len(candidates) == 0 {
		// The backend rejects empty candidates.
		return batch
	}

	if len(queriers) == 1 {
		invokeQuerier(ctx, candidates, arch, queriers[0], 0, reporter, buildIDToResult)
	} else {
		var wg sync.WaitGroup
		for i, querier := range queriers {
			wg.Add(1)
			go func(i int, querier SymbolQuerier) {
				defer wg.Done()
				invokeQuerier(ctx, candidates, arch, querier, i, reporter, buildIDToResult)
			}(i, querier)
		}
		wg.Wait()
	}

	return batch
}
