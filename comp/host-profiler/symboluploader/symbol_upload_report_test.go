// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package symboluploader

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	ddhttp "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/jarcoal/httpmock"
	"github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/host-profiler/symboluploader/symbol"
)

func setupReportFixture(t *testing.T) (*bytes.Buffer, *clockwork.FakeClock) {
	t.Helper()
	buf := &bytes.Buffer{}
	fc := clockwork.NewFakeClock()
	slog.SetDefault(slog.New(slog.NewJSONHandler(buf, nil)))
	symbolUploadInfo = newSymbolReporter(slog.LevelInfo)
	symbolUploadInfo.clock = fc
	symbolUploadInfo.start = fc.Now()

	symbolUploadError = newSymbolReporter(slog.LevelError)
	symbolUploadError.clock = fc
	symbolUploadError.start = fc.Now()
	return buf, fc
}

func decodeReports(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		out = append(out, record)
	}
	return out
}

func reasonPrefix(reason string) string {
	return "status_by_reason." + reason
}

func reasonCount(t *testing.T, report map[string]any, reason string) int {
	t.Helper()
	count, ok := report[reasonPrefix(reason)+".count"].(float64)
	require.True(t, ok, "missing count for %s", reason)
	return int(count)
}

func reasonPaths(t *testing.T, report map[string]any, reason string) []string {
	t.Helper()
	raw, ok := report[reasonPrefix(reason)+".paths"].([]any)
	require.True(t, ok, "missing paths for %s", reason)
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		s, ok := v.(string)
		require.True(t, ok, "non-string path %v", v)
		out = append(out, s)
	}
	slices.Sort(out)
	return out
}

func TestQuerySymbolsRateLimited(t *testing.T) {
	httpmock.Activate()
	t.Cleanup(httpmock.DeactivateAndReset)

	querier, err := NewDatadogSymbolQuerier("datad0g.com", "api-key")
	require.NoError(t, err)

	httpmock.RegisterResponder(
		http.MethodPost,
		"https://api.datad0g.com/api/v2/profiles/symbols/query",
		httpmock.NewStringResponder(429, `{"errors":[{"status":"429","title":"Too Many Requests"}]}`).
			HeaderSet(http.Header{"Retry-After": []string{"30"}}),
	)

	symbolFiles, err := querier.QuerySymbols(t.Context(), []string{"buildid1"}, "arch1")
	require.Nil(t, symbolFiles)
	var sce *ddhttp.StatusCodeError
	require.ErrorAs(t, err, &sce)
	require.Equal(t, http.StatusTooManyRequests, sce.StatusCode)
	require.Contains(t, err.Error(), "429")
}

func TestSymbolReportSamePathAcrossReasonsLevelsAndPeriods(t *testing.T) {
	buf, fc := setupReportFixture(t)

	SymbolUpload.Error(reasonRateLimited, 2, "/usr/bin/baz")
	SymbolUpload.Error(reasonRequestError, 1, "/usr/bin/baz")
	symbolUploadInfo.flush(fc.Now())
	symbolUploadError.flush(fc.Now())

	reports := decodeReports(t, buf)
	require.Len(t, reports, 1)
	require.Equal(t, 2, reasonCount(t, reports[0], reasonRateLimited))
	require.Equal(t, []string{"/usr/bin/baz"}, reasonPaths(t, reports[0], reasonRateLimited))
	require.Equal(t, 1, reasonCount(t, reports[0], reasonRequestError))
	require.Equal(t, []string{"/usr/bin/baz"}, reasonPaths(t, reports[0], reasonRequestError))

	SymbolUpload.Info(reasonUploaded, 1, "/usr/bin/cross")
	SymbolUpload.Error(reasonRateLimited, 1, "/usr/bin/cross")
	symbolUploadInfo.flush(fc.Now())
	symbolUploadError.flush(fc.Now())

	reports = decodeReports(t, buf)
	require.Len(t, reports, 3)
	require.Equal(t, 1, reasonCount(t, reports[1], reasonUploaded))
	require.Equal(t, []string{"/usr/bin/cross"}, reasonPaths(t, reports[1], reasonUploaded))
	require.Equal(t, 1, reasonCount(t, reports[2], reasonRateLimited))
	require.Equal(t, []string{"/usr/bin/cross"}, reasonPaths(t, reports[2], reasonRateLimited))
	require.NotContains(t, reports[2], reasonPrefix(reasonRequestError)+".count")
}

func TestSymbolReportPeriodAggregatesByLevel(t *testing.T) {
	buf, fc := setupReportFixture(t)
	ref := fc.Now()

	SymbolUpload.Info(reasonAlreadyUploaded, 420)
	SymbolUpload.Info(reasonQueried, 42)
	SymbolUpload.Info(reasonNoDebugSymbols, 33, "/usr/bin/stripped")
	SymbolUpload.Info(reasonExistingSymbols, 35, "/usr/bin/baz")
	SymbolUpload.Info(reasonUploaded, 6, "/usr/bin/foo", "/usr/bin/bar")
	SymbolUpload.Error(reasonRateLimited, 5, "/usr/bin/baz", "/usr/bin/foo")
	SymbolUpload.Error(reasonQueueFull, 1, "/usr/lib/ld.so")
	SymbolUpload.Error(reasonInvalidBuildID, 2)
	symbolUploadInfo.flush(fc.Now())
	symbolUploadError.flush(fc.Now())

	reports := decodeReports(t, buf)
	require.Len(t, reports, 2)
	info, failure := reports[0], reports[1]

	require.Equal(t, "INFO", info["level"])
	start, end := windowBounds(t, info)
	require.Equal(t, ref.Unix(), start.Unix())
	require.Equal(t, fc.Now().Unix(), end.Unix())
	require.Equal(t, 420, reasonCount(t, info, reasonAlreadyUploaded))
	require.Equal(t, 42, reasonCount(t, info, reasonQueried))
	require.Equal(t, 33, reasonCount(t, info, reasonNoDebugSymbols))
	require.Equal(t, 35, reasonCount(t, info, reasonExistingSymbols))
	require.Equal(t, 6, reasonCount(t, info, reasonUploaded))
	require.Equal(t, []string{"/usr/bin/stripped"}, reasonPaths(t, info, reasonNoDebugSymbols))
	require.Equal(t, []string{"/usr/bin/baz"}, reasonPaths(t, info, reasonExistingSymbols))
	require.Equal(t, []string{"/usr/bin/bar", "/usr/bin/foo"}, reasonPaths(t, info, reasonUploaded))
	require.NotContains(t, info, reasonPrefix(reasonAlreadyUploaded)+".paths")
	require.NotContains(t, info, reasonPrefix(reasonRateLimited)+".count")

	require.Equal(t, "ERROR", failure["level"])
	start, end = windowBounds(t, failure)
	require.Equal(t, ref.Unix(), start.Unix())
	require.Equal(t, fc.Now().Unix(), end.Unix())
	require.Equal(t, 5, reasonCount(t, failure, reasonRateLimited))
	require.Equal(t, 1, reasonCount(t, failure, reasonQueueFull))
	require.Equal(t, 2, reasonCount(t, failure, reasonInvalidBuildID))
	require.Equal(t, []string{"/usr/bin/baz", "/usr/bin/foo"}, reasonPaths(t, failure, reasonRateLimited))
	require.Equal(t, []string{"/usr/lib/ld.so"}, reasonPaths(t, failure, reasonQueueFull))
	require.NotContains(t, failure, reasonPrefix(reasonInvalidBuildID)+".paths")

	fc.Advance(2 * reportInterval)
	symbolUploadInfo.flush(fc.Now())
	symbolUploadError.flush(fc.Now())
	require.Len(t, decodeReports(t, buf), 2)
}

func TestSymbolReportPartialOnCap(t *testing.T) {
	buf, fc := setupReportFixture(t)
	ref := fc.Now()

	paths := make([]string, 0, maxReportSymbols)
	for i := 0; i < maxReportSymbols; i++ {
		paths = append(paths, fmt.Sprintf("/usr/bin/bin-%03d", i))
	}
	SymbolUpload.Error(reasonRateLimited, maxReportSymbols, paths...)
	require.Zero(t, buf.Len())

	burst := make([]string, 0, 150)
	for i := 0; i < 150; i++ {
		burst = append(burst, fmt.Sprintf("/usr/bin/ovf-%03d", i))
	}
	SymbolUpload.Error(reasonRateLimited, 150, burst...)
	reports := decodeReports(t, buf)
	require.Len(t, reports, 2)
	require.Equal(t, maxReportSymbols, reasonCount(t, reports[0], reasonRateLimited))
	require.Len(t, reasonPaths(t, reports[0], reasonRateLimited), maxReportSymbols)
	require.Equal(t, maxReportSymbols, reasonCount(t, reports[1], reasonRateLimited))
	require.Len(t, reasonPaths(t, reports[1], reasonRateLimited), maxReportSymbols)

	symbolUploadError.flush(fc.Now())
	reports = decodeReports(t, buf)
	require.Len(t, reports, 3)
	require.Equal(t, 50, reasonCount(t, reports[2], reasonRateLimited))
	require.Len(t, reasonPaths(t, reports[2], reasonRateLimited), 50)
	start, end := windowBounds(t, reports[2])
	require.Equal(t, ref.Unix(), start.Unix())
	require.Equal(t, fc.Now().Unix(), end.Unix())
}

func TestSymbolReportStopClosesPending(t *testing.T) {
	buf, fc := setupReportFixture(t)

	SymbolUpload.Error(reasonRequestError, 3, "/usr/bin/baz")
	require.Zero(t, buf.Len())

	SymbolUpload.Stop()
	reports := decodeReports(t, buf)
	require.Len(t, reports, 1)
	require.Equal(t, 3, reasonCount(t, reports[0], reasonRequestError))
	start, end := windowBounds(t, reports[0])
	require.Equal(t, fc.Now().Unix(), start.Unix())
	require.Equal(t, fc.Now().Unix(), end.Unix())
}

func TestSymbolReportRepeatedPathOccurrences(t *testing.T) {
	buf, fc := setupReportFixture(t)

	SymbolUpload.Error(reasonRateLimited, 1, "/usr/bin/baz")
	SymbolUpload.Error(reasonRateLimited, 1, "/usr/bin/baz")
	symbolUploadError.flush(fc.Now())

	reports := decodeReports(t, buf)
	require.Len(t, reports, 1)
	require.Equal(t, 2, reasonCount(t, reports[0], reasonRateLimited))
	require.Equal(t, []string{"/usr/bin/baz", "/usr/bin/baz"}, reasonPaths(t, reports[0], reasonRateLimited))
}

func TestSymbolReportTickerFlushesPeriods(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		buf, fc := setupReportFixture(t)
		startReportFlushTicker()
		t.Cleanup(func() {
			if tickerDone != nil {
				close(tickerDone)
				tickerDone = nil
			}
		})
		fc.BlockUntilContext(t.Context(), 1)
		SymbolUpload.Info(reasonQueried, 1)
		SymbolUpload.Error(reasonRateLimited, 1)

		fc.Advance(reportInterval - time.Nanosecond)
		synctest.Wait()
		require.Zero(t, buf.Len())

		fc.Advance(time.Nanosecond)
		synctest.Wait()
		reports := decodeReports(t, buf)
		require.Len(t, reports, 2)
		require.Equal(t, 1, reasonCount(t, reports[0], reasonQueried))
		require.Equal(t, 1, reasonCount(t, reports[1], reasonRateLimited))
	})
}

type failingQuerier struct {
	err error
}

func (f *failingQuerier) QuerySymbols(_ context.Context, _ []string, _ string) ([]SymbolFile, error) {
	return nil, f.err
}

func TestExecuteSymbolQueryBatchReportsRequestError(t *testing.T) {
	buf, _ := setupReportFixture(t)
	elf1, err := symbol.NewElfFromDisk("/bin/sh")
	require.NoError(t, err)
	elf2, err := symbol.NewElfFromDisk("/bin/ls")
	require.NoError(t, err)
	elfs := []*symbol.Elf{elf1, elf2}

	queriers := []SymbolQuerier{&failingQuerier{err: errors.New("error unmarshalling symbols query response")}}
	ExecuteSymbolQueryBatch(t.Context(), elfs, queriers)
	SymbolUpload.Stop()
	reports := decodeReports(t, buf)
	require.Len(t, reports, 1)
	require.Equal(t, "Symbol Upload Report", reports[0]["msg"])
	require.Equal(t, 2, reasonCount(t, reports[0], reasonRequestError))
}

// windowBounds returns the decoded window_start/window_end of a report.
func windowBounds(t *testing.T, report map[string]any) (time.Time, time.Time) {
	t.Helper()
	startRaw, ok := report["window_start"].(string)
	require.True(t, ok, "missing window_start")
	endRaw, ok := report["window_end"].(string)
	require.True(t, ok, "missing window_end")
	start, err := time.Parse(time.RFC3339, startRaw)
	require.NoError(t, err)
	end, err := time.Parse(time.RFC3339, endRaw)
	require.NoError(t, err)
	return start.UTC(), end.UTC()
}
