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
	infoReporter = newSymbolReporter(slog.LevelInfo)
	infoReporter.clock = fc
	infoReporter.current.start = fc.Now()

	errorReporter = newSymbolReporter(slog.LevelError)
	errorReporter.clock = fc
	errorReporter.current.start = fc.Now()
	return buf, fc
}

// decodeReports returns the symbol upload reports logged to buf.
func decodeReports(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	return decodeLogs(t, buf, reportMessage)
}

// decodeLogs returns the JSON log records of buf whose message is msg.
func decodeLogs(t *testing.T, buf *bytes.Buffer, msg string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		if record["msg"] == msg {
			out = append(out, record)
		}
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

// windowBounds returns the decoded window_start/window_end of a report.
func windowBounds(t *testing.T, report map[string]any) (start, end time.Time) {
	t.Helper()
	startRaw, ok := report["window_start"].(string)
	require.True(t, ok, "missing window_start")
	endRaw, ok := report["window_end"].(string)
	require.True(t, ok, "missing window_end")
	start, err := time.Parse(time.RFC3339Nano, startRaw)
	require.NoError(t, err)
	end, err = time.Parse(time.RFC3339Nano, endRaw)
	require.NoError(t, err)
	return start, end
}

// requireWindow asserts the exact window_start/window_end of a report.
func requireWindow(t *testing.T, report map[string]any, wantStart, wantEnd time.Time) {
	t.Helper()
	start, end := windowBounds(t, report)
	require.True(t, start.Equal(wantStart), "window_start = %v, want %v", start, wantStart)
	require.True(t, end.Equal(wantEnd), "window_end = %v, want %v", end, wantEnd)
}

func TestQuerySymbolsRateLimited(t *testing.T) {
	httpmock.Activate()
	t.Cleanup(httpmock.DeactivateAndReset)

	querier, err := NewDatadogSymbolQuerier("datad0g.com", "api-key")
	require.NoError(t, err)

	httpmock.RegisterResponder(
		http.MethodPost,
		buildSymbolQueryURL("datad0g.com"),
		httpmock.NewStringResponder(http.StatusTooManyRequests, `{"errors":[{"status":"429","title":"Too Many Requests"}]}`),
	)

	symbolFiles, err := querier.QuerySymbols(t.Context(), []string{"buildid1"}, "arch1")
	require.Nil(t, symbolFiles)
	var sce *ddhttp.StatusCodeError
	require.ErrorAs(t, err, &sce)
	require.Equal(t, http.StatusTooManyRequests, sce.StatusCode)
}

func TestSymbolReportSamePathAcrossReasonsLevelsAndPeriods(t *testing.T) {
	buf, fc := setupReportFixture(t)
	ref := fc.Now()

	reportError(reasonRateLimited, 2, "/usr/bin/baz")
	reportError(reasonRequestError, 1, "/usr/bin/baz")
	fc.Advance(time.Minute + time.Nanosecond)
	end1 := fc.Now()
	infoReporter.flush()
	errorReporter.flush()

	reports := decodeReports(t, buf)
	require.Len(t, reports, 1)
	requireWindow(t, reports[0], ref, ref.Add(time.Minute+time.Nanosecond))
	require.Equal(t, 2, reasonCount(t, reports[0], reasonRateLimited))
	require.Equal(t, []string{"/usr/bin/baz"}, reasonPaths(t, reports[0], reasonRateLimited))
	require.Equal(t, 1, reasonCount(t, reports[0], reasonRequestError))
	require.Equal(t, []string{"/usr/bin/baz"}, reasonPaths(t, reports[0], reasonRequestError))

	reportInfo(reasonUploaded, 1, "/usr/bin/cross")
	reportError(reasonRateLimited, 1, "/usr/bin/cross")
	fc.Advance(2 * time.Minute)
	infoReporter.flush()
	errorReporter.flush()

	reports = decodeReports(t, buf)
	require.Len(t, reports, 3)
	requireWindow(t, reports[1], end1, end1.Add(2*time.Minute))
	requireWindow(t, reports[2], end1, end1.Add(2*time.Minute))
	require.Equal(t, 1, reasonCount(t, reports[1], reasonUploaded))
	require.Equal(t, []string{"/usr/bin/cross"}, reasonPaths(t, reports[1], reasonUploaded))
	require.Equal(t, 1, reasonCount(t, reports[2], reasonRateLimited))
	require.Equal(t, []string{"/usr/bin/cross"}, reasonPaths(t, reports[2], reasonRateLimited))
	require.NotContains(t, reports[2], reasonPrefix(reasonRequestError)+".count")
}

func TestSymbolReportPeriodAggregatesByLevel(t *testing.T) {
	buf, fc := setupReportFixture(t)
	ref := fc.Now()

	reportInfo(reasonAlreadyUploaded, 420)
	reportInfo(reasonQueried, 42)
	reportInfo(reasonNoDebugSymbols, 33, "/usr/bin/stripped")
	reportInfo(reasonExistingSymbols, 35, "/usr/bin/baz")
	reportInfo(reasonUploaded, 6, "/usr/bin/foo", "/usr/bin/bar")
	reportError(reasonRateLimited, 5, "/usr/bin/baz", "/usr/bin/foo")
	reportError(reasonQueueFull, 1, "/usr/lib/ld.so")
	reportError(reasonInvalidBuildID, 2)
	fc.Advance(time.Minute)
	infoReporter.flush()
	errorReporter.flush()

	reports := decodeReports(t, buf)
	require.Len(t, reports, 2)
	info, failure := reports[0], reports[1]

	require.Equal(t, "INFO", info["level"])
	requireWindow(t, info, ref, ref.Add(time.Minute))
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
	requireWindow(t, failure, ref, ref.Add(time.Minute))
	require.Equal(t, 5, reasonCount(t, failure, reasonRateLimited))
	require.Equal(t, 1, reasonCount(t, failure, reasonQueueFull))
	require.Equal(t, 2, reasonCount(t, failure, reasonInvalidBuildID))
	require.Equal(t, []string{"/usr/bin/baz", "/usr/bin/foo"}, reasonPaths(t, failure, reasonRateLimited))
	require.Equal(t, []string{"/usr/lib/ld.so"}, reasonPaths(t, failure, reasonQueueFull))
	require.NotContains(t, failure, reasonPrefix(reasonInvalidBuildID)+".paths")

	fc.Advance(2 * reportInterval)
	infoReporter.flush()
	errorReporter.flush()
	require.Len(t, decodeReports(t, buf), 2)
}

func TestSymbolReportPartialOnCap(t *testing.T) {
	buf, fc := setupReportFixture(t)
	ref := fc.Now()

	paths := make([]string, 0, maxReportPaths)
	for i := range maxReportPaths {
		paths = append(paths, fmt.Sprintf("/usr/bin/bin-%03d", i))
	}
	reportError(reasonRateLimited, maxReportPaths, paths...)
	require.Zero(t, buf.Len())

	fc.Advance(time.Minute)
	burst := make([]string, 0, 150)
	for i := range 150 {
		burst = append(burst, fmt.Sprintf("/usr/bin/ovf-%03d", i))
	}
	reportError(reasonRateLimited, 150, burst...)
	reports := decodeReports(t, buf)
	require.Len(t, reports, 2)
	require.Equal(t, maxReportPaths, reasonCount(t, reports[0], reasonRateLimited))
	require.Len(t, reasonPaths(t, reports[0], reasonRateLimited), maxReportPaths)
	require.Equal(t, maxReportPaths, reasonCount(t, reports[1], reasonRateLimited))
	require.Len(t, reasonPaths(t, reports[1], reasonRateLimited), maxReportPaths)

	fc.Advance(time.Minute)
	errorReporter.flush()
	reports = decodeReports(t, buf)
	require.Len(t, reports, 3)
	require.Equal(t, 50, reasonCount(t, reports[2], reasonRateLimited))
	require.Len(t, reasonPaths(t, reports[2], reasonRateLimited), 50)

	requireWindow(t, reports[0], ref, ref.Add(time.Minute))
	requireWindow(t, reports[1], ref.Add(time.Minute), ref.Add(time.Minute))
	requireWindow(t, reports[2], ref.Add(time.Minute), ref.Add(2*time.Minute))
}

func TestSymbolReportCapTriggeredByAnotherReason(t *testing.T) {
	buf, _ := setupReportFixture(t)
	paths := make([]string, 0, maxReportPaths)
	for i := range maxReportPaths {
		paths = append(paths, fmt.Sprintf("/usr/bin/bin-%03d", i))
	}
	reportError(reasonRateLimited, maxReportPaths, paths...)
	reportError(reasonQueueFull, 1, "/usr/lib/ld.so")

	reports := decodeReports(t, buf)
	require.Len(t, reports, 1)
	require.NotContains(t, reports[0], reasonPrefix(reasonQueueFull)+".count")
}

func TestFlushReportsEmitsPending(t *testing.T) {
	buf, fc := setupReportFixture(t)
	ref := fc.Now()

	reportError(reasonRequestError, 3, "/usr/bin/baz")
	require.Zero(t, buf.Len())

	fc.Advance(time.Minute)
	flushReports()
	reports := decodeReports(t, buf)
	require.Len(t, reports, 1)
	require.Equal(t, 3, reasonCount(t, reports[0], reasonRequestError))
	requireWindow(t, reports[0], ref, ref.Add(time.Minute))
}

func TestSymbolReportRepeatedPathOccurrences(t *testing.T) {
	buf, _ := setupReportFixture(t)

	reportError(reasonRateLimited, 1, "/usr/bin/baz")
	reportError(reasonRateLimited, 1, "/usr/bin/baz")
	errorReporter.flush()

	reports := decodeReports(t, buf)
	require.Len(t, reports, 1)
	require.Equal(t, 2, reasonCount(t, reports[0], reasonRateLimited))
	require.Equal(t, []string{"/usr/bin/baz", "/usr/bin/baz"}, reasonPaths(t, reports[0], reasonRateLimited))
}

func TestSymbolReportTickerFlushesPeriods(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		buf, fc := setupReportFixture(t)
		ref := fc.Now()
		stop := startReportFlushTicker(fc)
		t.Cleanup(stop)
		require.NoError(t, fc.BlockUntilContext(t.Context(), 1))
		reportInfo(reasonQueried, 1)
		reportError(reasonRateLimited, 1)

		fc.Advance(reportInterval - time.Nanosecond)
		synctest.Wait()
		require.Zero(t, buf.Len())

		fc.Advance(time.Nanosecond)
		synctest.Wait()
		reports := decodeReports(t, buf)
		require.Len(t, reports, 2)
		require.Equal(t, 1, reasonCount(t, reports[0], reasonQueried))
		require.Equal(t, 1, reasonCount(t, reports[1], reasonRateLimited))
		requireWindow(t, reports[0], ref, ref.Add(reportInterval))
		requireWindow(t, reports[1], ref, ref.Add(reportInterval))

		stop()
		reportInfo(reasonQueried, 1)
		fc.Advance(reportInterval)
		synctest.Wait()
		require.Len(t, decodeReports(t, buf), 2)
	})
}

type failingQuerier struct {
	err error
}

func (f *failingQuerier) QuerySymbols(_ context.Context, _ []string, _ string) ([]SymbolFile, error) {
	return nil, f.err
}

func TestExecuteSymbolQueryBatchReportsQueryError(t *testing.T) {
	elf1, err := symbol.NewElfFromDisk("/bin/sh")
	require.NoError(t, err)
	t.Cleanup(elf1.Close)
	elf2, err := symbol.NewElfFromDisk("/bin/ls")
	require.NoError(t, err)
	t.Cleanup(elf2.Close)
	elfs := []*symbol.Elf{elf1, elf2}

	rateLimited := &ddhttp.StatusCodeError{StatusCode: http.StatusTooManyRequests}
	tests := []struct {
		name          string
		err           error
		wantReason    string
		wantErrorLogs int
	}{
		{"generic error", errors.New("error unmarshalling symbols query response"), reasonRequestError, 1},
		{"429", rateLimited, reasonRateLimited, 0},
		{"500", &ddhttp.StatusCodeError{StatusCode: http.StatusInternalServerError}, reasonRequestError, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf, _ := setupReportFixture(t)

			ExecuteSymbolQueryBatch(t.Context(), elfs, []SymbolQuerier{&failingQuerier{err: tt.err}})
			flushReports()

			reports := decodeReports(t, buf)
			require.Len(t, reports, 1)
			require.Equal(t, "ERROR", reports[0]["level"])
			require.Equal(t, len(elfs), reasonCount(t, reports[0], tt.wantReason))
			logs := decodeLogs(t, buf, "Failed to query symbols")
			require.Len(t, logs, tt.wantErrorLogs)
			for _, l := range logs {
				require.Equal(t, "ERROR", l["level"])
			}
		})
	}
}
