// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package demultiplexerendpointimpl

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/zstd"
)

type contextDumperFunc func(io.Writer) error

func (f contextDumperFunc) DumpDogstatsdContexts(w io.Writer) error {
	return f(w)
}

func TestDumpDogstatsdContextsRejectsWhenDataPlaneOwnsDogstatsd(t *testing.T) {
	endpoint := demultiplexerEndpoint{dogstatsdOnDataPlane: true}
	recorder := httptest.NewRecorder()

	endpoint.dumpDogstatsdContexts(recorder, httptest.NewRequest(http.MethodPost, "/dogstatsd-contexts-dump", nil))

	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Agent Data Plane")
}

func TestWriteDogstatsdContextsCoalescesConcurrentDumps(t *testing.T) {
	dumpStarted := make(chan struct{})
	releaseDump := make(chan struct{})
	var dumpCalls atomic.Int32

	endpoint := demultiplexerEndpoint{
		runPath: t.TempDir(),
		demux: contextDumperFunc(func(w io.Writer) error {
			if dumpCalls.Add(1) == 1 {
				close(dumpStarted)
			}
			<-releaseDump
			_, err := io.WriteString(w, "{}\n")
			return err
		}),
	}

	type result struct {
		path string
		err  error
	}
	firstResultCh := make(chan result, 1)
	go func() {
		path, err := endpoint.writeDogstatsdContexts()
		firstResultCh <- result{path: path, err: err}
	}()
	<-dumpStarted

	secondCallStarted := make(chan struct{})
	secondResultCh := make(chan result, 1)
	go func() {
		close(secondCallStarted)
		path, err := endpoint.writeDogstatsdContexts()
		secondResultCh <- result{path: path, err: err}
	}()
	<-secondCallStarted
	close(releaseDump)

	firstResult := <-firstResultCh
	secondResult := <-secondResultCh
	require.NoError(t, firstResult.err)
	require.NoError(t, secondResult.err)
	require.Equal(t, firstResult.path, secondResult.path)
	require.Equal(t, int32(1), dumpCalls.Load())
}

func TestWriteDogstatsdContextsPublishesAtomically(t *testing.T) {
	runPath := t.TempDir()
	finalPath := filepath.Join(runPath, "dogstatsd_contexts.json.zstd")
	require.NoError(t, os.WriteFile(finalPath, []byte("previous dump"), 0o644))

	started := make(chan struct{})
	finish := make(chan struct{})
	var finishOnce sync.Once
	releaseDump := func() {
		finishOnce.Do(func() { close(finish) })
	}
	t.Cleanup(releaseDump)

	endpoint := demultiplexerEndpoint{
		runPath: runPath,
		demux: contextDumperFunc(func(w io.Writer) error {
			_, err := io.WriteString(w, `{"name":"new dump"}`)
			close(started)
			<-finish
			return err
		}),
	}

	type result struct {
		path string
		err  error
	}
	resultCh := make(chan result, 1)
	go func() {
		path, err := endpoint.writeDogstatsdContexts()
		resultCh <- result{path: path, err: err}
	}()

	<-started
	contents, err := os.ReadFile(finalPath)
	require.NoError(t, err)
	require.Equal(t, []byte("previous dump"), contents)

	releaseDump()
	writeResult := <-resultCh
	require.NoError(t, writeResult.err)
	require.Equal(t, finalPath, writeResult.path)

	contents, err = os.ReadFile(finalPath)
	require.NoError(t, err)
	decoder, err := zstd.NewReader(bytes.NewReader(contents))
	require.NoError(t, err)
	decompressed, err := io.ReadAll(decoder)
	require.NoError(t, err)
	decoder.Close()
	require.JSONEq(t, `{"name":"new dump"}`, string(decompressed))

	tempFiles, err := filepath.Glob(filepath.Join(runPath, "dogstatsd_contexts-*.tmp"))
	require.NoError(t, err)
	require.Empty(t, tempFiles)
}

func TestWriteDogstatsdContextsFailurePreservesExistingDump(t *testing.T) {
	runPath := t.TempDir()
	finalPath := filepath.Join(runPath, "dogstatsd_contexts.json.zstd")
	require.NoError(t, os.WriteFile(finalPath, []byte("previous dump"), 0o644))

	dumpErr := errors.New("dump failed")
	endpoint := demultiplexerEndpoint{
		runPath: runPath,
		demux: contextDumperFunc(func(w io.Writer) error {
			_, err := io.WriteString(w, "partial dump")
			require.NoError(t, err)
			return dumpErr
		}),
	}

	_, err := endpoint.writeDogstatsdContexts()
	require.ErrorIs(t, err, dumpErr)

	contents, err := os.ReadFile(finalPath)
	require.NoError(t, err)
	require.Equal(t, []byte("previous dump"), contents)

	tempFiles, err := filepath.Glob(filepath.Join(runPath, "dogstatsd_contexts-*.tmp"))
	require.NoError(t, err)
	require.Empty(t, tempFiles)
}
