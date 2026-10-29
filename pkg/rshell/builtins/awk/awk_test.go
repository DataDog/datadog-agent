// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package awk

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

type blockingProgramReader struct {
	started   chan struct{}
	closed    chan struct{}
	closeOnce sync.Once
}

type blockingNonClosingProgramReader struct {
	started  chan struct{}
	release  chan struct{}
	finished chan struct{}
}

func newBlockingProgramReader() *blockingProgramReader {
	return &blockingProgramReader{
		started: make(chan struct{}),
		closed:  make(chan struct{}),
	}
}

func (r *blockingProgramReader) Read([]byte) (int, error) {
	close(r.started)
	<-r.closed
	return 0, os.ErrClosed
}

func (r *blockingProgramReader) Write([]byte) (int, error) {
	return 0, os.ErrInvalid
}

func (r *blockingProgramReader) Close() error {
	r.closeOnce.Do(func() { close(r.closed) })
	return nil
}

func (r *blockingNonClosingProgramReader) Read([]byte) (int, error) {
	close(r.started)
	<-r.release
	close(r.finished)
	return 0, io.EOF
}

func TestLoadProgramRejectsTooManyFilesBeforeOpening(t *testing.T) {
	opened := 0
	callCtx := &builtins.CallContext{
		OpenFile: func(context.Context, string, int, os.FileMode) (io.ReadWriteCloser, error) {
			opened++
			return &closeTrackedFile{Reader: strings.NewReader("")}, nil
		},
	}
	programFiles := make([]string, MaxProgramFiles+1)

	_, _, err := loadProgram(context.Background(), callCtx, nil, programFiles)

	require.EqualError(t, err, fmt.Sprintf("too many program files (maximum %d)", MaxProgramFiles))
	assert.Zero(t, opened)
}

func TestLoadProgramCountsFileSeparatorsTowardSizeLimit(t *testing.T) {
	tests := []struct {
		name        string
		firstSize   int
		wantErr     bool
		wantOpened  int
		wantProgram int
	}{
		{name: "at limit", firstSize: MaxProgramBytes - 1, wantOpened: 2, wantProgram: MaxProgramBytes},
		{name: "over limit", firstSize: MaxProgramBytes, wantErr: true, wantOpened: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opened := 0
			contents := map[string]string{
				"first.awk": strings.Repeat(" ", tt.firstSize),
				"empty.awk": "",
			}
			callCtx := &builtins.CallContext{
				OpenFile: func(_ context.Context, path string, _ int, _ os.FileMode) (io.ReadWriteCloser, error) {
					opened++
					return &closeTrackedFile{Reader: strings.NewReader(contents[path])}, nil
				},
			}

			program, _, err := loadProgram(context.Background(), callCtx, nil, []string{"first.awk", "empty.awk"})

			if tt.wantErr {
				require.EqualError(t, err, fmt.Sprintf("program exceeds %d bytes", MaxProgramBytes))
			} else {
				require.NoError(t, err)
				assert.Len(t, program, tt.wantProgram)
			}
			assert.Equal(t, tt.wantOpened, opened)
		})
	}
}

func TestReadProgramStdinCancellationInterruptsReaderWithoutDeadline(t *testing.T) {
	reader := newBlockingProgramReader()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		total := 0
		_, err := readProgramCancellable(ctx, reader, &total)
		done <- err
	}()

	<-reader.started
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("program read did not observe cancellation")
	}
}

func TestReadProgramFileCancellationInterruptsReader(t *testing.T) {
	reader := newBlockingProgramReader()
	callCtx := &builtins.CallContext{
		OpenFile: func(context.Context, string, int, os.FileMode) (io.ReadWriteCloser, error) {
			return reader, nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		total := 0
		_, err := readProgramFile(ctx, callCtx, "program.fifo", &total)
		done <- err
	}()

	<-reader.started
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		_ = reader.Close()
		t.Fatal("program file read did not observe cancellation")
	}
}

func TestReadProgramStdinCancellableContextReadsNonCloser(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	total := 0

	text, err := readProgramCancellable(ctx, strings.NewReader(`BEGIN { print "ok" }`), &total)

	require.NoError(t, err)
	assert.Equal(t, `BEGIN { print "ok" }`, text)
	assert.Equal(t, len(text), total)
}

func TestReadProgramStdinCancellationInterruptsNonCloser(t *testing.T) {
	reader := &blockingNonClosingProgramReader{
		started:  make(chan struct{}),
		release:  make(chan struct{}),
		finished: make(chan struct{}),
	}
	t.Cleanup(func() {
		close(reader.release)
		<-reader.finished
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		total := 0
		_, err := readProgramCancellable(ctx, reader, &total)
		done <- err
	}()

	<-reader.started
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("program read did not observe cancellation")
	}
}
